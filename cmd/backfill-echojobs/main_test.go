package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// fakeStore serves one keyset page of echojobs rows and records every description update.
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
	// echojobs is boardless, so its stored external_id carries the empty-board namespace
	// prefix (":job-handle") — jobHandle strips it before the fetch keys on the raw handle.
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "echojobs", ExternalID: ":a", Title: "A", Description: ""},
		{ID: 2, Source: "echojobs", ExternalID: ":b", Title: "B", Description: "same"},
		{ID: 3, Source: "echojobs", ExternalID: ":c", Title: "C", Description: ""},
	}}
	fetch := func(_ context.Context, jobHandle string) (string, bool) {
		switch jobHandle {
		case "a":
			return "New body A", true // empty → gets filled
		case "b":
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
	want := jobhash.OfRow(store.jobs[0], "New body A")
	if !u.ContentHash.Valid || u.ContentHash.String != want {
		t.Errorf("ContentHash = %+v, want %q", u.ContentHash, want)
	}
}

func TestJobHandleStripsNamespacePrefix(t *testing.T) {
	if got := jobHandle(":acme-swe-abc12"); got != "acme-swe-abc12" {
		t.Errorf("jobHandle(%q) = %q, want %q", ":acme-swe-abc12", got, "acme-swe-abc12")
	}
}
