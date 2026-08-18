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
			return ctx.Err()
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
}
