package apiusage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeStore struct {
	fail  bool
	calls []db.AddAPIUsageParams
}

func (f *fakeStore) AddAPIUsage(_ context.Context, arg db.AddAPIUsageParams) error {
	if f.fail {
		return errors.New("down")
	}
	f.calls = append(f.calls, arg)
	return nil
}

// totals sums every flushed row per (user, day) so the test does not depend on map order.
func totals(calls []db.AddAPIUsageParams) map[string][2]int64 {
	out := map[string][2]int64{}
	for _, c := range calls {
		for i := range c.UserIds {
			k := fmt.Sprintf("%s/%d", c.Days[i].Time.Format("2006-01-02"), c.UserIds[i])
			t := out[k]
			out[k] = [2]int64{t[0] + c.Requests[i], t[1] + c.Errors[i]}
		}
	}
	return out
}

func TestFlushGroupsByAccountAndUTCDay(t *testing.T) {
	store := &fakeStore{}
	c := New(store)
	day1 := time.Date(2026, 11, 13, 23, 59, 0, 0, time.UTC)
	c.Record(1, false, day1)
	c.Record(1, true, day1)
	c.Record(2, false, day1)
	c.Record(1, false, day1.Add(2*time.Minute)) // the next UTC day

	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := totals(store.calls)
	want := map[string][2]int64{
		"2026-11-13/1": {2, 1},
		"2026-11-13/2": {1, 0},
		"2026-11-14/1": {1, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %v, want %v", k, got[k], v)
		}
	}
	if err := c.Flush(context.Background()); err != nil || len(store.calls) != 1 {
		t.Fatalf("an empty buffer must not write: calls=%d err=%v", len(store.calls), err)
	}
}

func TestFailedFlushKeepsCountsForTheNext(t *testing.T) {
	store := &fakeStore{fail: true}
	c := New(store)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	c.Record(1, false, now)
	if err := c.Flush(context.Background()); err == nil {
		t.Fatal("want the store's error")
	}
	c.Record(1, true, now)
	store.fail = false
	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := totals(store.calls)["2026-11-13/1"]; got != [2]int64{2, 1} {
		t.Fatalf("got %v, want [2 1]", got)
	}
}

// blockingStore holds a flush until its context ends, then fails it, the way a write
// cut off by shutdown does.
type blockingStore struct {
	started chan struct{}
	fakeStore
}

func (b *blockingStore) AddAPIUsage(ctx context.Context, arg db.AddAPIUsageParams) error {
	if b.started != nil {
		close(b.started)
		b.started = nil
		<-ctx.Done()
		return ctx.Err()
	}
	return b.fakeStore.AddAPIUsage(ctx, arg)
}

// A flush cut off when Run is stopped puts its counts back before Run returns, so the
// final flush after Run has returned writes them.
func TestStoppedRunLeavesInFlightCountsForTheFinalFlush(t *testing.T) {
	started := make(chan struct{})
	store := &blockingStore{started: started}
	c := New(store)
	c.Record(1, false, time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx, time.Millisecond)
	}()
	<-started
	cancel()
	<-done

	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := totals(store.calls)["2026-11-13/1"]; got != [2]int64{1, 0} {
		t.Fatalf("got %v, want [1 0]", got)
	}
}
