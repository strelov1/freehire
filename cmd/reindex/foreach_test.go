package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func noCheckpoint(string) {}

// The fuzzy pass is latency-bound — measured 2026-10-28 it spent under seven minutes of CPU
// in a four-hour run — so the walk has to keep several companies in flight at once. This
// holds every call until `workers` of them are running together; a sequential walk never
// gets there and the calls fail.
func TestForEachCompanyRunsCompaniesConcurrently(t *testing.T) {
	const workers = 4
	var inFlight atomic.Int32
	reached := make(chan struct{})
	var once sync.Once

	_, err := forEachCompany(context.Background(), companyNames(8), workers, func(ctx context.Context, _ string) (int64, error) {
		if inFlight.Add(1) == workers {
			once.Do(func() { close(reached) })
		}
		defer inFlight.Add(-1)
		select {
		case <-reached:
			return 1, nil
		case <-time.After(5 * time.Second):
			return 0, errors.New("never saw the other workers")
		}
	}, noCheckpoint)
	if err != nil {
		t.Fatalf("walk: %v — the companies were not processed %d at a time", err, workers)
	}
}

// One broken company must not starve the rest, and the aggregate error keeps the shape the
// operator reads in the journal: how many failed out of how many, and the last one.
func TestForEachCompanyIsolatesAFailureAndSumsTheRest(t *testing.T) {
	total, err := forEachCompany(context.Background(), []string{"a", "b", "c"}, 2, func(_ context.Context, c string) (int64, error) {
		if c == "b" {
			return 0, errors.New("statement timeout")
		}
		return 10, nil
	}, noCheckpoint)

	if total != 20 {
		t.Errorf("total = %d, want 20 from the two companies that succeeded", total)
	}
	if err == nil || err.Error() != `1/3 companies failed; last: company "b": statement timeout` {
		t.Errorf("err = %v, want the per-company failure summary", err)
	}
}

// The resume point is the oldest company not FINISHED, never the newest one started or
// finished. With several in flight, later companies finish while an earlier one is still
// running; recording the newest would skip the slow one forever (the freehire#2869 trap
// backfill-derive fell into). Here "b" is still running when the run is cancelled, after
// "a", "c", "d" and "e" are done — so the walk may only claim "a".
func TestForEachCompanyCheckpointsTheOldestUnfinishedCompany(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var others sync.WaitGroup
	others.Add(4) // a, c, d, e
	go func() { others.Wait(); cancel() }()

	var saved []string // checkpoint runs on the walk's own goroutine, never concurrently
	_, err := forEachCompany(ctx, []string{"a", "b", "c", "d", "e"}, 3, func(ctx context.Context, c string) (int64, error) {
		if c == "b" {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		defer others.Done()
		return 1, nil
	}, func(after string) { saved = append(saved, after) })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want it to wrap context.Canceled", err)
	}
	if len(saved) == 0 || saved[len(saved)-1] != "a" {
		t.Errorf("checkpoints = %v, want the last one to be %q — %q never finished", saved, "a", "b")
	}
}

// A company cut off by the cancellation is unfinished, not failed: counting it would turn
// one deadline into hundreds of thousands of "failures" (228191 of them on 2026-10-28) and
// say nothing about how far the walk got. Nothing more is dispatched once the context is dead.
func TestForEachCompanyStopsOnCancellationWithoutCountingFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32

	_, err := forEachCompany(ctx, companyNames(1000), 1, func(ctx context.Context, c string) (int64, error) {
		if calls.Add(1) == 3 {
			cancel()
			return 0, ctx.Err()
		}
		return 1, nil
	}, noCheckpoint)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want it to wrap context.Canceled", err)
	}
	if strings.Contains(err.Error(), "failed;") {
		t.Errorf("err = %q, a cancellation must not read as per-company failures", err)
	}
	if n := calls.Load(); n > 4 {
		t.Errorf("fn called %d times after the cancellation at call 3, want the walk to stop", n)
	}
}

// Progress is saved as the walk goes, not only at its end: the run is ended by SIGTERM from
// the unit's timeout, and a process that is then SIGKILLed never reaches its last line.
func TestForEachCompanyCheckpointsBeforeTheEnd(t *testing.T) {
	companies := companyNames(checkpointEvery + 1)
	var saved []string
	_, err := forEachCompany(context.Background(), companies, 1, func(context.Context, string) (int64, error) {
		return 0, nil
	}, func(after string) { saved = append(saved, after) })
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(saved) < 2 || saved[0] != companies[checkpointEvery-1] {
		t.Errorf("first checkpoint = %v, want %q before the final one", saved[:min(len(saved), 1)], companies[checkpointEvery-1])
	}
}

// companyNames returns n distinct names already in the order the walk visits them.
func companyNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("co-%06d", i)
	}
	return out
}
