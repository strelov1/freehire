package main

import (
	"context"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// observedStore serves jobs like fakeStore, records which ids were actually WRITTEN, and
// lets a test cancel the run partway through via onWrite. That is the state the resume
// point has to survive and that no single-page test can reach: the reader feeds through a
// channel buffered at backfillBatchSize, so it can accept a whole page — and advance its
// own cursor past it — long before a worker has written any of it.
//
// It records writes itself rather than reading fakeStore.updates because the assertion is
// about what SURVIVED: a write attempted under a cancelled context must not count.
type observedStore struct {
	*fakeStore
	mu      sync.Mutex
	written map[int64]bool
	onWrite func(id int64)
}

func newObservedStore(jobs []db.Job) *observedStore {
	return &observedStore{fakeStore: &fakeStore{jobs: jobs}, written: map[int64]bool{}}
}

func (b *observedStore) UpdateJobDerived(ctx context.Context, arg db.UpdateJobDerivedParams) error {
	if b.onWrite != nil {
		b.onWrite(arg.ID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	b.written[arg.ID] = true
	b.mu.Unlock()
	return b.fakeStore.UpdateJobDerived(ctx, arg)
}

func (b *observedStore) wroteIDs() map[int64]bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[int64]bool, len(b.written))
	for k, v := range b.written {
		out[k] = v
	}
	return out
}

// A cancelled run must resume BEFORE the oldest row it did not finish, not after the
// newest row its reader happened to read. Those are different ids whenever the channel
// holds a backlog, and the gap between them is rows that would keep stale facets for
// good with nothing downstream reporting it.
func TestBackfill_CancelResumesBeforeTheOldestUnfinishedRow(t *testing.T) {
	const total = 1200 // more than backfillBatchSize, so the reader runs ahead
	jobs := make([]db.Job, 0, total)
	for id := int64(1); id <= total; id++ {
		jobs = append(jobs, derivableJob(id))
	}
	store := newObservedStore(jobs)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel once a few rows are written, while the rest of the backlog is still queued.
	var once sync.Once
	var n int
	var mu sync.Mutex
	store.onWrite = func(int64) {
		mu.Lock()
		n++
		hit := n >= 5
		mu.Unlock()
		if hit {
			once.Do(cancel)
		}
	}

	pass, _ := backfillWindow(ctx, store, 1, scanWindow{}, 0, nil)
	if pass.ResumeID == 0 {
		t.Fatal("a cancelled run reported no resume point, so the operator is told it finished")
	}

	// Every id at or below the resume point must actually have been derived. The resume
	// point is what the next run skips, so anything unwritten below it is lost for good.
	wrote := store.wroteIDs()
	for id := int64(1); id < pass.ResumeID; id++ {
		if !wrote[id] {
			t.Fatalf("resume point %d skips id %d, which was never written", pass.ResumeID, id)
		}
	}
}

// The resume point has to survive a store error too: that is the same lost work, and
// run() cannot print an id the pass did not return.
func TestBackfill_StoreErrorStillReportsAResumePoint(t *testing.T) {
	const total = 1200
	jobs := make([]db.Job, 0, total)
	for id := int64(1); id <= total; id++ {
		jobs = append(jobs, derivableJob(id))
	}
	store := newObservedStore(jobs)

	var mu sync.Mutex
	var n int
	store.onWrite = func(id int64) {
		mu.Lock()
		n++
		mu.Unlock()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Fail the run partway by cancelling: the write path turns that into a store error,
	// which is the shape a redeploy or a unit timeout takes in production.
	go func() {
		for {
			mu.Lock()
			hit := n >= 10
			mu.Unlock()
			if hit {
				cancel()
				return
			}
		}
	}()

	pass, err := backfillWindow(ctx, store, 2, scanWindow{}, 0, nil)
	if err == nil && pass.ResumeID == 0 {
		t.Skip("the run finished before the cancel landed; nothing to assert")
	}
	if pass.ResumeID == 0 {
		t.Fatal("a run that stopped on a store error returned no resume point, so run() has none to print")
	}
	wrote := store.wroteIDs()
	for id := int64(1); id < pass.ResumeID; id++ {
		if !wrote[id] {
			t.Fatalf("resume point %d skips id %d, which was never written", pass.ResumeID, id)
		}
	}
}

// A budget that lands exactly on the last row means the table IS fully derived. Saying
// "NOT fully derived" there sends an operator back for a pass with nothing in it, and
// the whole point of the message is that it must not have to be second-guessed.
func TestBackfill_BudgetEndingOnTheLastRowReportsDone(t *testing.T) {
	jobs := []db.Job{derivableJob(1), derivableJob(2), derivableJob(3)}
	store := &fakeStore{jobs: jobs}

	pass, err := backfillWindow(context.Background(), store, 1, scanWindow{maxRows: 3}, 0, nil)
	if err != nil {
		t.Fatalf("backfillWindow: %v", err)
	}
	if pass.Scanned != 3 {
		t.Fatalf("scanned=%d, want 3", pass.Scanned)
	}
	if pass.ResumeID != 0 {
		t.Errorf("ResumeID=%d, want 0 — the budget consumed the whole table", pass.ResumeID)
	}
}

// fromID names the first id a run must DO, matching BACKFILL_REQUIREMENTS_FROM_ID on the
// sibling pass. Two knobs with the same name and opposite senses is how an operator
// chaining them loses exactly one row per hop.
func TestBackfill_FromIDIsInclusive(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		derivableJob(1), derivableJob(2), derivableJob(3), derivableJob(4),
	}}

	pass, err := backfillWindow(context.Background(), store, 1, scanWindow{fromID: 3}, 0, nil)
	if err != nil {
		t.Fatalf("backfillWindow: %v", err)
	}
	if pass.Scanned != 2 {
		t.Fatalf("scanned=%d, want 2 (ids 3 and 4)", pass.Scanned)
	}
	if got := visitedIDs(store); len(got) != 2 || got[0] != 3 {
		t.Errorf("visited %v, want [3 4]", got)
	}
}
