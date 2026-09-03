package atsapply

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

type fakeAtomReader struct {
	atoms []experience.Atom
	err   error
}

func (f fakeAtomReader) ListAtoms(context.Context, int64) ([]experience.Atom, error) {
	return f.atoms, f.err
}

func TestBuildGroundingContext_KeepsOnlyPublishableAtoms(t *testing.T) {
	reader := fakeAtomReader{atoms: []experience.Atom{
		{ID: uuid.New(), Claim: "Shipped the payments migration", Provenance: experience.ProvenanceCVImport},
		{ID: uuid.New(), Claim: "Stated in a chat with the assistant", Provenance: experience.ProvenanceStatedInChat},
		{ID: uuid.New(), Claim: "Entered by hand in the experience editor", Provenance: experience.ProvenanceManual},
		{ID: uuid.New(), Claim: "The model's own guess, never confirmed", Provenance: experience.ProvenanceAgentInferred},
	}}

	got, err := buildGroundingContext(context.Background(), reader, 7)
	if err != nil {
		t.Fatalf("buildGroundingContext: %v", err)
	}

	if len(got.Atoms) != 3 {
		t.Fatalf("got %d atoms, want 3 (agent_inferred excluded)", len(got.Atoms))
	}
	for _, a := range got.Atoms {
		if a.Provenance == experience.ProvenanceAgentInferred {
			t.Errorf("agent_inferred atom %q leaked into the grounding context", a.Claim)
		}
	}
}

func TestBuildGroundingContext_NoAtomsIsNotAnError(t *testing.T) {
	got, err := buildGroundingContext(context.Background(), fakeAtomReader{}, 7)
	if err != nil {
		t.Fatalf("buildGroundingContext: %v", err)
	}
	if len(got.Atoms) != 0 {
		t.Errorf("got %d atoms, want 0", len(got.Atoms))
	}
}

func TestBuildGroundingContext_PropagatesAReadError(t *testing.T) {
	boom := context.DeadlineExceeded
	if _, err := buildGroundingContext(context.Background(), fakeAtomReader{err: boom}, 7); err == nil {
		t.Fatal("want the read error surfaced, not swallowed")
	}
}
