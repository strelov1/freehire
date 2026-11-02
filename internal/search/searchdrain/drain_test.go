package searchdrain

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type indexBatchFunc func(context.Context, []db.Job) error

func (f indexBatchFunc) IndexBatch(ctx context.Context, jobs []db.Job) error { return f(ctx, jobs) }

type deleteBatchFunc func(context.Context, []int64) error

func (f deleteBatchFunc) DeleteBatch(ctx context.Context, ids []int64) error { return f(ctx, ids) }

func queuedIndexStore(ids ...int64) *fakeStore {
	s := newFakeStore()
	for _, id := range ids {
		s.pending = append(s.pending, Claimed{OutboxID: id, JobID: id})
		s.jobs[id] = db.Job{ID: id}
	}
	return s
}

func TestDrainAlternatesWavesWithIndependentBudgets(t *testing.T) {
	var events []string
	store := queuedIndexStore(1, 2, 3, 4, 5, 6)
	removals := newFakeDeleteStore(11, 12, 13, 14, 15, 16)
	runner := Runner{Store: store, Indexer: indexBatchFunc(func(context.Context, []db.Job) error {
		events = append(events, "index")
		return nil
	})}
	deletions := DeletionRunner{Store: removals, Deleter: deleteBatchFunc(func(context.Context, []int64) error {
		events = append(events, "delete")
		return nil
	})}
	opts := deleteOptions()
	opts.MaxPerRun = 5
	indexed, deleted, err := Drain(context.Background(), runner, deletions, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"delete", "index", "delete", "index", "delete", "index"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("waves = %v, want %v", events, want)
	}
	if indexed.Indexed != 5 || deleted.Deleted != 5 || len(store.pending) != 1 || len(removals.pending) != 1 {
		t.Fatalf("budgets: indexed=%+v deleted=%+v remaining=%d/%d", indexed, deleted, len(store.pending), len(removals.pending))
	}
}

func TestDrainLeavesNewIndexTricklesForNextRun(t *testing.T) {
	store := queuedIndexStore(1)
	removals := newFakeDeleteStore(11, 12, 13, 14, 15)
	calls := 0
	runner := Runner{Store: store, Indexer: indexBatchFunc(func(context.Context, []db.Job) error {
		calls++
		id := int64(100 + calls)
		store.pending = append(store.pending, Claimed{OutboxID: id, JobID: id})
		store.jobs[id] = db.Job{ID: id}
		return nil
	})}
	opts := deleteOptions()
	opts.MaxPerRun = 5
	indexed, deleted, err := Drain(context.Background(), runner, DeletionRunner{Store: removals, Deleter: &fakeDeleter{}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if indexed.Indexed != 1 || calls != 1 || len(store.pending) != 1 || deleted.Deleted != 5 {
		t.Fatalf("trickle: indexed=%+v calls=%d pending=%d deleted=%+v", indexed, calls, len(store.pending), deleted)
	}
}

func TestDrainCountsDeferredWavesAgainstBudget(t *testing.T) {
	store := queuedIndexStore(1, 2, 3, 4, 5, 6)
	opts := deleteOptions()
	opts.MaxPerRun = 2
	indexed, _, err := Drain(context.Background(), Runner{Store: store, Indexer: indexBatchFunc(func(context.Context, []db.Job) error {
		return ErrSkipWave
	})}, DeletionRunner{Store: newFakeDeleteStore(), Deleter: &fakeDeleter{}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.pending) != 4 || len(store.failCalls) != 0 || indexed.Indexed != 0 {
		t.Fatalf("deferred wave exceeded budget or spent attempts: remaining=%d stats=%+v failures=%v", len(store.pending), indexed, store.failCalls)
	}
}

func TestDrainStopsOnClaimError(t *testing.T) {
	want := errors.New("database unavailable")
	store := queuedIndexStore(1)
	_, _, err := Drain(context.Background(), Runner{Store: store, Indexer: newFakeIndexer()}, DeletionRunner{
		Store: &fakeDeleteStore{claimErr: want}, Deleter: &fakeDeleter{},
	}, deleteOptions())
	if !errors.Is(err, want) || len(store.pending) != 1 {
		t.Fatalf("err=%v pending=%d", err, len(store.pending))
	}
}

func TestDrainRejectsInvalidBatchSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		_, _, err := Drain(context.Background(), Runner{}, DeletionRunner{}, RunOptions{BatchSize: size})
		if err == nil {
			t.Fatalf("batch size %d accepted", size)
		}
	}
}

func TestDrainReapsOnceAcrossWaves(t *testing.T) {
	store := queuedIndexStore(1, 2, 3, 4)
	_, _, err := Drain(context.Background(), Runner{Store: store, Indexer: newFakeIndexer()}, DeletionRunner{
		Store: newFakeDeleteStore(), Deleter: &fakeDeleter{},
	}, deleteOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.reapCalls) != 1 || len(store.pending) != 0 {
		t.Fatalf("reap calls=%v, pending=%d", store.reapCalls, len(store.pending))
	}
}
