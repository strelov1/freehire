package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// fakeStore serves one keyset page of justjoin rows and records every description update.
type fakeStore struct {
	jobs    []db.Job
	updates []db.UpdateJobDescriptionParams
	served  bool
}

func (f *fakeStore) ListJobsBySourceAfter(_ context.Context, _ db.ListJobsBySourceAfterParams) ([]db.Job, error) {
	if f.served {
		return nil, nil
	}
	f.served = true
	return f.jobs, nil
}

func (f *fakeStore) UpdateJobDescription(_ context.Context, arg db.UpdateJobDescriptionParams) (int64, error) {
	f.updates = append(f.updates, arg)
	return 1, nil
}

func TestBackfillUpdatesOnlyChangedDescriptions(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "justjoin", URL: "https://justjoin.it/job-offer/a", Title: "A", Description: ""},
		{ID: 2, Source: "justjoin", URL: "https://justjoin.it/job-offer/b", Title: "B", Description: "same"},
		{ID: 3, Source: "justjoin", URL: "https://justjoin.it/job-offer/c", Title: "C", Description: ""},
	}}
	fetch := func(_ context.Context, jobURL string) (string, bool) {
		switch jobURL {
		case "https://justjoin.it/job-offer/a":
			return "New body A", true // empty → gets filled
		case "https://justjoin.it/job-offer/b":
			return "same", true // unchanged → skipped
		default:
			return "", false // detail failed → counted, skipped
		}
	}

	scanned, updated, failed, err := backfillAll(context.Background(), store, fetch)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 3 || updated != 1 || failed != 1 {
		t.Fatalf("scanned=%d updated=%d failed=%d, want 3/1/1", scanned, updated, failed)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 1 {
		t.Fatalf("updates = %+v, want one update of job 1", store.updates)
	}
	u := store.updates[0]
	if u.Description != "New body A" {
		t.Errorf("Description = %q, want %q", u.Description, "New body A")
	}
	// The content_hash must be the fingerprint of the row's indexed fields WITH the new
	// description, so the row re-indexes (and is stable on a re-run).
	want := jobhash.OfRow(store.jobs[0], "New body A")
	if !u.ContentHash.Valid || u.ContentHash.String != want {
		t.Errorf("ContentHash = %+v, want %q", u.ContentHash, want)
	}
}
