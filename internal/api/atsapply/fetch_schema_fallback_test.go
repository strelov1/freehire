package atsapply

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func formWith(id string) applyform.Form {
	return applyform.Form{Fields: []applyform.Field{{ID: id, Type: applyform.TypeText}}}
}

// Workable answered this host with 429 and a 23-hour Retry-After on 2026-10-25, and every
// approved application failed before reaching the browser. The stored form — the one the
// candidate reviewed — carries the attempt instead.
func TestFetchSchema_AFailedLiveFetchFallsBackToTheStoredForm(t *testing.T) {
	c := &Client{
		fetchers: map[string]applyform.Fetcher{"workable": &fakeFetcher{err: errors.New("status 429")}},
		forms:    &fakeFormReader{found: true, form: formWith("stored")},
	}

	form, err := c.fetchSchema(context.Background(), autoapply.Claimed{Provider: "workable"})
	if err != nil {
		t.Fatalf("fetchSchema: %v", err)
	}
	if form.Fields[0].ID != "stored" {
		t.Errorf("form = %+v, want the stored form", form)
	}
}

// A live fetch that works still wins over storage: a stored row may predate an edit to
// the form.
func TestFetchSchema_ALiveFetchStillWinsOverStorage(t *testing.T) {
	c := &Client{
		fetchers: map[string]applyform.Fetcher{"workable": &fakeFetcher{form: formWith("live")}},
		forms:    &fakeFormReader{found: true, form: formWith("stored")},
	}

	form, err := c.fetchSchema(context.Background(), autoapply.Claimed{Provider: "workable"})
	if err != nil {
		t.Fatalf("fetchSchema: %v", err)
	}
	if form.Fields[0].ID != "live" {
		t.Errorf("form = %+v, want the live form", form)
	}
}

// With nothing stored the live error is returned unchanged, so a transient failure stays
// an ordinary retryable one.
func TestFetchSchema_AFailedLiveFetchWithNothingStoredKeepsItsError(t *testing.T) {
	liveErr := errors.New("status 429")
	c := &Client{
		fetchers: map[string]applyform.Fetcher{"workable": &fakeFetcher{err: liveErr}},
		forms:    &fakeFormReader{found: false},
	}

	if _, err := c.fetchSchema(context.Background(), autoapply.Claimed{Provider: "workable"}); !errors.Is(err, liveErr) {
		t.Errorf("err = %v, want the live fetch's own error", err)
	}
}
