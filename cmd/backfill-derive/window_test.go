package main

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// derivableJob is a job whose stored derived columns are empty, so a pass over it
// always writes. The window tests care about WHICH ids a run visited rather than what
// it derived from them, so this is deliberately the same row at every id.
func derivableJob(id int64) db.Job {
	return db.Job{
		ID: id, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
	}
}

// visitedIDs is the sorted set of ids a run wrote. Sorted because the worker pool
// writes in whatever order it finishes, which is not the order it read.
func visitedIDs(store *fakeStore) []int64 {
	out := make([]int64, 0, len(store.updates))
	for _, u := range store.updates {
		out = append(out, u.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// A run started at an id must not go back before it — that is what makes an interrupted
// pass resumable rather than restarting at the beginning of a 12.7M-row table. What the
// id itself means is pinned separately, by TestBackfill_FromIDIsInclusive.
func TestBackfill_WindowStartsAtFromID(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		derivableJob(1), derivableJob(2), derivableJob(3), derivableJob(4), derivableJob(5),
	}}

	run, err := backfillWindow(context.Background(), store, 1, scanWindow{fromID: 4}, 0, nil)
	if err != nil {
		t.Fatalf("backfillWindow: %v", err)
	}
	if run.Scanned != 2 {
		t.Fatalf("scanned=%d, want 2 (ids 4 and 5)", run.Scanned)
	}
	if got := visitedIDs(store); !reflect.DeepEqual(got, []int64{4, 5}) {
		t.Errorf("visited %v, want [4 5]", got)
	}
}

// A run that exhausts the table reports no resume point, so an operator can tell
// "there is more" from "that was all" without counting rows themselves.
func TestBackfill_ExhaustedWindowReportsNoResume(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{derivableJob(1), derivableJob(2)}}

	run, err := backfillWindow(context.Background(), store, 1, scanWindow{}, 0, nil)
	if err != nil {
		t.Fatalf("backfillWindow: %v", err)
	}
	if run.ResumeID != 0 {
		t.Errorf("ResumeID=%d, want 0 (the pass reached the end)", run.ResumeID)
	}
	if run.Scanned != 2 {
		t.Errorf("scanned=%d, want 2", run.Scanned)
	}
}

// maxRows bounds one run and hands back the id to continue from. Without it the pass
// can only run to completion, which on prod is a ~17h unit a 10h systemd timeout kills
// before it finishes — every night, from the beginning, deriving nothing new.
func TestBackfill_WindowStopsAtMaxAndResumes(t *testing.T) {
	jobs := make([]db.Job, 0, 6)
	for id := int64(1); id <= 6; id++ {
		jobs = append(jobs, derivableJob(id))
	}

	first := &fakeStore{jobs: jobs}
	firstRun, err := backfillWindow(context.Background(), first, 1, scanWindow{maxRows: 4}, 0, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if firstRun.Scanned != 4 {
		t.Fatalf("first run scanned=%d, want 4", firstRun.Scanned)
	}
	if firstRun.ResumeID == 0 {
		t.Fatal("first run reported no resume point, but two rows are left")
	}

	second := &fakeStore{jobs: jobs}
	rest, err := backfillWindow(context.Background(), second, 1, scanWindow{fromID: firstRun.ResumeID}, 0, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if rest.ResumeID != 0 {
		t.Errorf("second run ResumeID=%d, want 0 (it reached the end)", rest.ResumeID)
	}

	// No GAP is the property that matters: a resume point that skipped a row would
	// leave its derived columns stale for good, and nothing downstream would report it.
	covered := map[int64]bool{}
	for _, id := range visitedIDs(first) {
		covered[id] = true
	}
	for _, id := range visitedIDs(second) {
		covered[id] = true
	}
	for id := int64(1); id <= 6; id++ {
		if !covered[id] {
			t.Errorf("id %d was derived by neither run", id)
		}
	}
}
