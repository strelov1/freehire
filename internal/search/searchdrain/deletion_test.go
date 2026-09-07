package searchdrain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeDeleteStore is the deletion queue in memory: entries are handed out once per Claim
// and removed by Complete, so a runner that never completes loops forever and a runner that
// completes twice is visible as a missing entry.
type fakeDeleteStore struct {
	pending   []Claimed
	completed []Claimed
	failed    map[int64]string
	claimErr  error
	claims    int
}

func newFakeDeleteStore(jobIDs ...int64) *fakeDeleteStore {
	s := &fakeDeleteStore{failed: map[int64]string{}}
	for i, id := range jobIDs {
		s.pending = append(s.pending, Claimed{OutboxID: int64(i + 1), JobID: id})
	}
	return s
}

func (s *fakeDeleteStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	s.claims++
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	if len(s.pending) == 0 {
		return nil, nil
	}
	if batch > len(s.pending) {
		batch = len(s.pending)
	}
	out := s.pending[:batch]
	s.pending = s.pending[batch:]
	return out, nil
}

func (s *fakeDeleteStore) Complete(_ context.Context, entries []Claimed) error {
	s.completed = append(s.completed, entries...)
	return nil
}

func (s *fakeDeleteStore) Fail(_ context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	s.failed[outboxID] = errMsg
	return maxAttempts <= 1, nil
}

// fakeDeleter records what it was asked to delete and can be told to fail.
type fakeDeleter struct {
	batches [][]int64
	err     error
	delay   time.Duration
	// failFirstN makes the first N calls fail, so a batch failure followed by a
	// successful per-item fallback is expressible.
	failFirstN int
	calls      int
}

func (d *fakeDeleter) DeleteBatch(ctx context.Context, jobIDs []int64) error {
	d.calls++
	if d.delay > 0 {
		select {
		case <-time.After(d.delay):
		case <-ctx.Done():
			// What the SDK actually hands back, rather than the context's own error:
			// *meilisearch.Error implements no Unwrap (search/client.go says so itself),
			// so a guard that reads the ERROR cannot see the context ending — and the
			// case it misses is the deadline landing inside the SDK's retry window,
			// which engages on 502/503/504, i.e. exactly when it is needed.
			return errors.New("meilisearch: communication error")
		}
	}
	if d.failFirstN >= d.calls {
		return errors.New("index unavailable")
	}
	if d.err != nil {
		return d.err
	}
	ids := append([]int64(nil), jobIDs...)
	d.batches = append(d.batches, ids)
	return nil
}

func deleteOptions() RunOptions {
	return RunOptions{BatchSize: 2, LeaseSeconds: 300, MaxAttempts: 3}
}

func TestDeletionRunnerDeletesEveryQueuedJob(t *testing.T) {
	store := newFakeDeleteStore(10, 11, 12)
	deleter := &fakeDeleter{}

	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(context.Background(), deleteOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Deleted != 3 {
		t.Errorf("Deleted = %d, want 3", stats.Deleted)
	}
	if len(store.completed) != 3 {
		t.Errorf("completed %d entries, want 3 — an entry left behind is deleted again every run",
			len(store.completed))
	}
}

// Deleting a document that was never indexed is a no-op in Meilisearch, so there is nothing
// to distinguish and nothing to retry: the entry must be completed, not left to loop.
func TestDeletionRunnerCompletesEvenWhenNothingWasIndexed(t *testing.T) {
	store := newFakeDeleteStore(99)
	deleter := &fakeDeleter{}

	if _, err := (DeletionRunner{Store: store, Deleter: deleter}).Run(context.Background(), deleteOptions()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.completed) != 1 {
		t.Fatalf("completed %d entries, want 1", len(store.completed))
	}
	if len(store.pending) != 0 {
		t.Errorf("%d entries left pending, want 0", len(store.pending))
	}
}

// A batch failure must not sink the whole wave: fall back to per item, the same way the
// indexing runner does, so one poisonous entry cannot block the rest.
func TestDeletionRunnerFallsBackPerItemOnBatchFailure(t *testing.T) {
	store := newFakeDeleteStore(20, 21)
	deleter := &fakeDeleter{failFirstN: 1}

	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(context.Background(), deleteOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2 — the per-item fallback should have salvaged the wave", stats.Deleted)
	}
	for _, batch := range deleter.batches {
		if len(batch) != 1 {
			t.Errorf("fallback issued a batch of %d, want per-item calls", len(batch))
		}
	}
}

// A call-context timeout is NOT a per-document defect, so it must skip the wave rather than
// fall back — falling back would turn one slow call into BatchSize equally slow ones, which
// is the shape that produced a real outage on the indexing side.
func TestDeletionRunnerSkipsTheWaveOnCallTimeout(t *testing.T) {
	store := newFakeDeleteStore(30, 31)
	deleter := &fakeDeleter{delay: 50 * time.Millisecond}
	opt := deleteOptions()
	opt.CallTimeout = time.Millisecond

	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(context.Background(), opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0 on a timed-out wave", stats.Deleted)
	}
	if deleter.calls != 1 {
		t.Errorf("deleter called %d times, want 1 — a timeout must skip the wave, not fall back per item",
			deleter.calls)
	}
	if len(store.completed) != 0 {
		t.Errorf("completed %d entries on a timed-out wave, want 0 — the lease must retry them",
			len(store.completed))
	}
}

func TestDeletionRunnerDeadLettersARepeatedlyFailingEntry(t *testing.T) {
	store := newFakeDeleteStore(40)
	deleter := &fakeDeleter{err: errors.New("permanently broken")}
	opt := deleteOptions()
	opt.MaxAttempts = 1

	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(context.Background(), opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Errorf("DeadLettered = %d, want 1", stats.DeadLettered)
	}
	// One entry, one outcome — as the indexing runner's failN has always reported it.
	// Counting the same entry as failed AND dead-lettered made a single exhausted entry
	// read as two problems in the run's summary and in the exit code's inputs.
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0 — a dead-lettered entry is not also a failure", stats.Failed)
	}
}

// The run being cancelled is not a per-document defect either, and it is the ordinary case
// rather than an exotic one: freehire-reindexw stops this service before every rebuild.
// Falling back here would ask the index to delete each id separately on a context that is
// already done, and charge every entry an attempt for the privilege.
func TestDeletionRunnerSkipsTheWaveWhenTheRunIsCancelled(t *testing.T) {
	store := newFakeDeleteStore(50, 51)
	deleter := &fakeDeleter{delay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(ctx, deleteOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Deleted != 0 || stats.Failed != 0 || stats.DeadLettered != 0 {
		t.Errorf("stats = %+v, want all zero — a cancelled run learned nothing about these entries", stats)
	}
	if deleter.calls != 1 {
		t.Errorf("deleter called %d times, want 1 — a cancelled wave must not fall back per item",
			deleter.calls)
	}
	if len(store.failed) != 0 {
		t.Errorf("recorded failures %v, want none — an ordinary stop must not spend the attempt budget",
			store.failed)
	}
	if len(store.completed) != 0 {
		t.Errorf("completed %d entries, want 0 — the lease must retry them", len(store.completed))
	}
}

// The removal pass honours the same bound as the indexing one, so a bounded run splits its
// budget between the two rather than letting whichever goes first spend the whole process.
func TestDeletionRunnerStopsAtMaxPerRun(t *testing.T) {
	store := newFakeDeleteStore(10, 11, 12, 13, 14)
	deleter := &fakeDeleter{}

	opts := deleteOptions()
	opts.BatchSize = 2
	opts.MaxPerRun = 4
	stats, err := DeletionRunner{Store: store, Deleter: deleter}.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Deleted != 4 {
		t.Fatalf("Deleted = %d, want 4 — the run took the whole queue", stats.Deleted)
	}
	if len(store.completed) != 4 {
		t.Errorf("completed = %d, want 4", len(store.completed))
	}
}
