package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestReapDueOncePerWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	due := reapDue("redis://" + mr.Addr())
	ctx := context.Background()

	if !due(ctx) {
		t.Fatal("first run in a window must reap")
	}
	if due(ctx) {
		t.Fatal("a second run in the same window must skip the reap")
	}
	mr.FastForward(reapEvery)
	if !due(ctx) {
		t.Fatal("the first run after the window must reap again")
	}
}

func TestReapDueSkipsWhenRedisIsUnreachable(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()
	if reapDue("redis://" + addr)(context.Background()) {
		t.Fatal("an unreachable Redis must skip the reap, not run it every time")
	}
}

func TestDBStoreReapSkipsWhenNotDue(t *testing.T) {
	// q is nil: reaching the query would panic, so a skipped reap must return first.
	s := &dbStore{reapDue: func(context.Context) bool { return false }}
	if n, err := s.Reap(context.Background(), 10); n != 0 || err != nil {
		t.Fatalf("Reap = %d, %v; want 0, nil", n, err)
	}
}
