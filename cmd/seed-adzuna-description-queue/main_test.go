package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

type fakeJobStore struct {
	pages  [][]db.Job
	call   int
	queued []int64
	enqErr error
}

func (f *fakeJobStore) ListJobsBySourceAfter(_ context.Context, _ db.ListJobsBySourceAfterParams) ([]db.Job, error) {
	if f.call >= len(f.pages) {
		return nil, nil
	}
	p := f.pages[f.call]
	f.call++
	return p, nil
}

func (f *fakeJobStore) EnqueueAdzunaDescriptionCapture(_ context.Context, jobID int64) (int64, error) {
	if f.enqErr != nil {
		return 0, f.enqErr
	}
	f.queued = append(f.queued, jobID)
	return 1, nil
}

func TestSeedAllQueuesOnlyEligibleRows(t *testing.T) {
	store := &fakeJobStore{pages: [][]db.Job{
		{
			{ID: 1, Source: "adzuna", URL: "https://www.adzuna.co.uk/jobs/details/1"},
			{ID: 2, Source: "adzuna", URL: "https://www.adzuna.co.uk/jobs/land/ad/2"},
			{ID: 3, Source: "adzuna", URL: "https://www.adzuna.com.au/details/3"},
		},
	}}

	scanned, queued, err := seedAll(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scanned != 3 {
		t.Errorf("scanned = %d, want 3", scanned)
	}
	if queued != 2 {
		t.Errorf("queued = %d, want 2", queued)
	}
	if len(store.queued) != 2 || store.queued[0] != 1 || store.queued[1] != 3 {
		t.Errorf("queued job ids = %v, want [1 3]", store.queued)
	}
}

func TestSeedAllStopsOnAShortPageWithoutAnExtraQuery(t *testing.T) {
	// A page smaller than the batch size is itself the exhaustion signal (mirrors
	// cmd/backfill-echojobs), so a second, empty page is never queried.
	store := &fakeJobStore{pages: [][]db.Job{
		{{ID: 1, Source: "adzuna", URL: "https://www.adzuna.co.uk/jobs/details/1"}},
	}}

	scanned, queued, err := seedAll(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scanned != 1 || queued != 1 {
		t.Errorf("scanned=%d queued=%d, want 1/1", scanned, queued)
	}
	if store.call != 1 {
		t.Errorf("pages read = %d, want 1", store.call)
	}
}
