package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// The worker's cost claim rests entirely here: a user whose structure is current must be
// resolvable with NO extractor at all. Passing nil for both the extractor and the résumé
// store is how the test states "there is no path to the model from here".
func TestResolveStructuredReusesAStoredStructureWithoutTheModel(t *testing.T) {
	stored := &resumeextract.Structured{
		Experience: []resumeextract.Experience{{Company: "RingCentral", Title: "SWE"}},
	}

	got, err := resolveStructured(context.Background(), target{id: 7, structured: stored}, nil, nil)
	if err != nil {
		t.Fatalf("resolveStructured: %v", err)
	}
	if got != stored {
		t.Errorf("resolveStructured returned %+v, want the stored structure verbatim", got)
	}
}

// Without a stored structure and without an extractor there is nothing to do — and that
// is a skip, not a failure. A run with the LLM unconfigured must still complete its free
// pass over everyone else.
func TestResolveStructuredSkipsWhenThereIsNoExtractor(t *testing.T) {
	got, err := resolveStructured(context.Background(), target{id: 7}, nil, nil)
	if err != nil {
		t.Fatalf("resolveStructured: %v", err)
	}
	if got != nil {
		t.Errorf("resolveStructured = %+v, want nil — no structure and no way to make one", got)
	}
}

func TestDecodeStructured(t *testing.T) {
	if got := decodeStructured(nil); got != nil {
		t.Errorf("decodeStructured(nil) = %+v, want nil", got)
	}
	if got := decodeStructured([]byte{}); got != nil {
		t.Errorf("decodeStructured(empty) = %+v, want nil", got)
	}
	// An unreadable payload falls through to the extraction path rather than failing the
	// user: a structure we cannot parse is, for this worker, a structure we do not have.
	if got := decodeStructured([]byte("{not json")); got != nil {
		t.Errorf("decodeStructured(garbage) = %+v, want nil", got)
	}

	got := decodeStructured([]byte(`{"experience":[{"company":"Sber","title":"Team Lead"}]}`))
	if got == nil || len(got.Experience) != 1 || got.Experience[0].Company != "Sber" {
		t.Errorf("decodeStructured = %+v, want the stored structure", got)
	}
}
