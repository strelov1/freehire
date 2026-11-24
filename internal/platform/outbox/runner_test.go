package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClaimer replays a fixed sequence of waves, then returns empty forever —
// the shape every real Store.Claim has (a lease-bounded batch, empty when drained).
type fakeClaimer struct {
	mu             sync.Mutex
	waves          [][]int
	calls          int
	requestedBatch []int
	// errOnCall, if set, makes the Nth Claim call (1-indexed) return errAfter instead
	// of a wave.
	errOnCall int
	errAfter  error
}

func (f *fakeClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestedBatch = append(f.requestedBatch, batch)
	f.calls++
	if f.errOnCall != 0 && f.calls == f.errOnCall {
		return nil, f.errAfter
	}
	if f.calls > len(f.waves) {
		return nil, nil
	}
	w := f.waves[f.calls-1]
	return w, nil
}

func TestRunPool_ProcessesAllItemsUntilClaimEmpty(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}, {4, 5}}}

	var mu sync.Mutex
	processed := map[int]bool{}
	process := func(ctx context.Context, item int) Outcome {
		mu.Lock()
		processed[item] = true
		mu.Unlock()
		return Succeeded
	}

	stats, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 2}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 5 {
		t.Errorf("Succeeded = %d, want 5", stats.Succeeded)
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !processed[id] {
			t.Errorf("item %d was never processed", id)
		}
	}
}

func TestRunPool_RunsUpToConcurrencyItemsInParallel(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	arrived := make(chan int, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	doRelease := func() { releaseOnce.Do(func() { close(release) }) }
	defer doRelease()

	process := func(ctx context.Context, item int) Outcome {
		arrived <- item
		<-release
		return Succeeded
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 3}, process)
	}()

	// All 3 items must arrive concurrently — proves they are not serialized one
	// at a time behind each other's <-release wait.
	for i := 0; i < 3; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for concurrent arrivals; got %d/3", i)
		}
	}
	doRelease()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunPool did not return after release")
	}
}

func TestRunPool_MaxPerRunStopsAndShrinksNextClaim(t *testing.T) {
	// Wave two carries only 1 item — a real Store.Claim would truncate to the
	// shrunk batch size; this fake just needs the data shaped for it.
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}, {4}}}
	process := func(ctx context.Context, item int) Outcome { return Succeeded }

	stats, err := RunPool(context.Background(), claimer, RunOptions{
		BatchSize:    3,
		LeaseSeconds: 60,
		Concurrency:  1,
		MaxPerRun:    4,
	}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 4 {
		t.Errorf("Succeeded = %d, want 4", stats.Succeeded)
	}
	if claimer.calls != 2 {
		t.Errorf("claim calls = %d, want 2 (must stop before a 3rd claim once budget is spent)", claimer.calls)
	}
	wantRequested := []int{3, 1}
	if len(claimer.requestedBatch) != len(wantRequested) {
		t.Fatalf("requestedBatch = %v, want %v", claimer.requestedBatch, wantRequested)
	}
	for i, want := range wantRequested {
		if claimer.requestedBatch[i] != want {
			t.Errorf("requestedBatch[%d] = %d, want %d (should shrink to the remaining budget)", i, claimer.requestedBatch[i], want)
		}
	}
}

func TestRunPool_ConcurrencyOneProcessesInStrictSubmissionOrder(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	// order is deliberately unsynchronized: any concurrent access here is a data
	// race (catch with `go test -race`), which a true sequential loop can't trigger.
	var order []int
	process := func(ctx context.Context, item int) Outcome {
		if item == 1 {
			// If items ran concurrently, 2 and 3 would very likely finish first.
			time.Sleep(20 * time.Millisecond)
		}
		order = append(order, item)
		return Succeeded
	}

	_, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60, Concurrency: 1}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1, 2, 3}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i, id := range want {
		if order[i] != id {
			t.Errorf("order[%d] = %d, want %d", i, order[i], id)
		}
	}
}

// StopOnCancel is the one cancel-aware Claimer every wave-loop worker that wants a clean stop
// on SIGTERM wraps its store in. The loops themselves stay cancellation-agnostic (see the
// Claimer doc comment), so the contract lives here and is tested here.

// A cancelled run stops at the next wave boundary instead of working through the backlog
// and spending every entry's attempt budget on a shutdown.
func TestStopOnCancel_StopsAtTheNextWaveBoundaryOnceCancelled(t *testing.T) {
	inner := &fakeClaimer{waves: [][]int{{1}, {2}, {3}}}
	ctx, cancel := context.WithCancel(context.Background())
	process := func(_ context.Context, _ int) Outcome {
		cancel() // SIGTERM lands mid-wave
		return Succeeded
	}

	stats, err := RunPool(ctx, &StopOnCancel[int]{Claimer: inner}, RunOptions{BatchSize: 1, LeaseSeconds: 60}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1 (the claimed wave finishes, then the run stops)", stats.Succeeded)
	}
	if inner.calls != 1 {
		t.Errorf("inner Claim calls = %d, want 1 (no claim is made after cancellation)", inner.calls)
	}
}

// The first claim is never guarded: a run that starts already cancelled still makes one
// real attempt, so a shutdown-at-startup shows up as the store's own claim error rather
// than a silent empty run that reads as a drained queue.
func TestStopOnCancel_TheFirstClaimIsNeverGuarded(t *testing.T) {
	inner := &fakeClaimer{waves: [][]int{{1, 2}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stats, err := RunPool(ctx, &StopOnCancel[int]{Claimer: inner}, RunOptions{BatchSize: 2, LeaseSeconds: 60}, func(context.Context, int) Outcome {
		return Succeeded
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("inner Claim calls = %d, want 1 (the unguarded first claim)", inner.calls)
	}
	if stats.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2 (the first wave is fully processed)", stats.Succeeded)
	}
}

// RunBatch takes the same wrapper; a cancelled removal or index wave must not be claimed.
func TestStopOnCancel_RunBatchStopsToo(t *testing.T) {
	inner := &fakeClaimer{waves: [][]int{{1}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	processBatch := func(_ context.Context, items []int) (Stats, error) {
		calls++
		return Stats{Succeeded: len(items)}, nil
	}

	stats, err := RunBatch(ctx, &StopOnCancel[int]{Claimer: inner}, RunOptions{BatchSize: 1, LeaseSeconds: 60}, processBatch, nil)
	if err != nil || calls != 1 || stats.Succeeded != 1 || inner.calls != 1 {
		t.Fatalf("RunBatch = (%+v, %v) with %d batch calls and %d claims, want one unguarded wave then a stop", stats, err, calls, inner.calls)
	}
}

// An uncancelled run is untouched: every wave is claimed and passed through verbatim.
func TestStopOnCancel_IsTransparentWhileCtxIsLive(t *testing.T) {
	inner := &fakeClaimer{waves: [][]int{{1, 2}, {3}}}
	claimer := &StopOnCancel[int]{Claimer: inner}

	stats, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 2, LeaseSeconds: 60}, func(context.Context, int) Outcome {
		return Succeeded
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 3 || inner.calls != 3 {
		t.Errorf("Succeeded = %d, inner Claim calls = %d, want 3 and 3 (two waves plus the empty one)", stats.Succeeded, inner.calls)
	}
	if len(inner.requestedBatch) != 3 || inner.requestedBatch[0] != 2 {
		t.Errorf("requestedBatch = %v, want the batch size passed through unchanged", inner.requestedBatch)
	}
}

// A claim error is the queue being unusable and must still abort the run, not read as a stop.
func TestStopOnCancel_PassesAClaimErrorThrough(t *testing.T) {
	wantErr := errors.New("pool unreachable")
	inner := &fakeClaimer{errOnCall: 1, errAfter: wantErr}

	_, err := RunPool(context.Background(), &StopOnCancel[int]{Claimer: inner}, RunOptions{BatchSize: 1, LeaseSeconds: 60}, func(context.Context, int) Outcome {
		return Succeeded
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestClaimFunc_AdaptsABareClaimFunction(t *testing.T) {
	var gotBatch, gotLease int
	var f ClaimFunc[int] = func(_ context.Context, batch, leaseSeconds int) ([]int, error) {
		gotBatch, gotLease = batch, leaseSeconds
		return nil, nil
	}

	if _, err := f.Claim(context.Background(), 7, 42); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if gotBatch != 7 || gotLease != 42 {
		t.Errorf("Claim passed (batch, lease) = (%d, %d), want (7, 42)", gotBatch, gotLease)
	}
}

func TestRunPool_AbortsRunOnClaimError(t *testing.T) {
	wantErr := errors.New("pool unreachable")
	claimer := &fakeClaimer{waves: [][]int{{1, 2}}, errOnCall: 2, errAfter: wantErr}

	var itemCalls int
	process := func(ctx context.Context, item int) Outcome {
		itemCalls++
		return Succeeded
	}

	stats, err := RunPool(context.Background(), claimer, RunOptions{BatchSize: 2, LeaseSeconds: 60, Concurrency: 1}, process)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if stats.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2 (the first wave, processed before the failing claim)", stats.Succeeded)
	}
	if itemCalls != 2 {
		t.Errorf("itemCalls = %d, want 2 (no processing should be attempted after a claim error)", itemCalls)
	}
}

func TestRunBatch_AbortsRunOnClaimError(t *testing.T) {
	wantErr := errors.New("pool unreachable")
	claimer := &fakeClaimer{waves: [][]int{{1, 2}}, errOnCall: 2, errAfter: wantErr}

	processBatch := func(ctx context.Context, items []int) (Stats, error) { return Stats{Succeeded: len(items)}, nil }
	processOne := func(ctx context.Context, item int) Outcome { return Succeeded }

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 2, LeaseSeconds: 60}, processBatch, processOne)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if stats.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2 (the first wave, processed before the failing claim)", stats.Succeeded)
	}
}

func TestRunBatch_MaxPerRunStopsAndShrinksNextClaim(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}, {4}}}
	processBatch := func(ctx context.Context, items []int) (Stats, error) { return Stats{Succeeded: len(items)}, nil }
	processOne := func(ctx context.Context, item int) Outcome { return Succeeded }

	stats, err := RunBatch(context.Background(), claimer, RunOptions{
		BatchSize:    3,
		LeaseSeconds: 60,
		MaxPerRun:    4,
	}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Succeeded != 4 {
		t.Errorf("Succeeded = %d, want 4", stats.Succeeded)
	}
	if claimer.calls != 2 {
		t.Errorf("claim calls = %d, want 2 (must stop before a 3rd claim once budget is spent)", claimer.calls)
	}
	wantRequested := []int{3, 1}
	if len(claimer.requestedBatch) != len(wantRequested) {
		t.Fatalf("requestedBatch = %v, want %v", claimer.requestedBatch, wantRequested)
	}
	for i, want := range wantRequested {
		if claimer.requestedBatch[i] != want {
			t.Errorf("requestedBatch[%d] = %d, want %d (should shrink to the remaining budget)", i, claimer.requestedBatch[i], want)
		}
	}
}

func TestRunPool_CallsOnWaveWithCumulativeStatsAfterEachWave(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2}, {3}}}
	process := func(ctx context.Context, item int) Outcome { return Succeeded }

	var mu sync.Mutex
	var seen []Stats
	onWave := func(s Stats) {
		mu.Lock()
		seen = append(seen, s)
		mu.Unlock()
	}

	_, err := RunPool(context.Background(), claimer, RunOptions{
		BatchSize: 2, LeaseSeconds: 60, Concurrency: 1, OnWave: onWave,
	}, process)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Stats{{Succeeded: 2}, {Succeeded: 3}}
	if len(seen) != len(want) {
		t.Fatalf("OnWave calls = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("OnWave call %d = %+v, want %+v (must be cumulative, not per-wave delta)", i, seen[i], want[i])
		}
	}
}

func TestRunBatch_CallsOnWaveWithCumulativeStatsAfterEachWave(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2}, {3}}}
	processBatch := func(ctx context.Context, items []int) (Stats, error) { return Stats{Succeeded: len(items)}, nil }
	processOne := func(ctx context.Context, item int) Outcome { return Succeeded }

	var seen []Stats
	onWave := func(s Stats) { seen = append(seen, s) }

	_, err := RunBatch(context.Background(), claimer, RunOptions{
		BatchSize: 2, LeaseSeconds: 60, OnWave: onWave,
	}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Stats{{Succeeded: 2}, {Succeeded: 3}}
	if len(seen) != len(want) {
		t.Fatalf("OnWave calls = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("OnWave call %d = %+v, want %+v", i, seen[i], want[i])
		}
	}
}

func TestRunBatch_SucceedsWholeWaveViaOneBatchCall(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}
	var batchCalls, itemCalls int

	processBatch := func(ctx context.Context, items []int) (Stats, error) {
		batchCalls++
		return Stats{Succeeded: len(items)}, nil
	}
	processOne := func(ctx context.Context, item int) Outcome {
		itemCalls++
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batchCalls != 1 {
		t.Errorf("batchCalls = %d, want 1", batchCalls)
	}
	if itemCalls != 0 {
		t.Errorf("itemCalls = %d, want 0 (fallback must not run when the batch call succeeds)", itemCalls)
	}
	if stats.Succeeded != 3 {
		t.Errorf("Succeeded = %d, want 3", stats.Succeeded)
	}
}

func TestRunBatch_FallsBackToPerItemOnBatchFailure(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}

	processBatch := func(ctx context.Context, items []int) (Stats, error) {
		return Stats{}, errors.New("meili down")
	}
	var mu sync.Mutex
	seen := map[int]bool{}
	processOne := func(ctx context.Context, item int) Outcome {
		mu.Lock()
		seen[item] = true
		mu.Unlock()
		if item == 2 {
			return Failed
		}
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seen) != 3 {
		t.Errorf("per-item fallback saw %d items, want 3", len(seen))
	}
	if stats.Succeeded != 2 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want Succeeded=2 Failed=1", stats)
	}
}

// TestRunBatch_ClosureReportsOwnPartialStatsWithoutOuterFallback covers a
// BatchProcessor that internally splits its wave into sub-groups and handles each
// with its own batch-attempt-then-per-item-fallback cycle (internal/ai/embed's actual
// shape: an open-jobs group and a closed-jobs group, each pushed as its own
// Meilisearch call). Returning (Stats, nil) means "I fully handled every item myself,
// including my own fallback" — RunBatch must trust the returned Stats verbatim and
// must NOT also invoke its own processOne, which would re-process (and potentially
// double-complete) items the closure already finished.
func TestRunBatch_ClosureReportsOwnPartialStatsWithoutOuterFallback(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}
	var outerFallbackCalls int

	// Simulates: item 1 succeeded via the group's batch call, items 2 and 3 failed
	// and were already routed to Fail internally by the closure's own fallback.
	processBatch := func(ctx context.Context, items []int) (Stats, error) {
		return Stats{Succeeded: 1, Failed: 2}, nil
	}
	processOne := func(ctx context.Context, item int) Outcome {
		outerFallbackCalls++
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outerFallbackCalls != 0 {
		t.Errorf("outer processOne was called %d times, want 0 (the closure already handled every item itself)", outerFallbackCalls)
	}
	if stats.Succeeded != 1 || stats.Failed != 2 {
		t.Errorf("stats = %+v, want Succeeded=1 Failed=2 (trusted verbatim from the closure)", stats)
	}
}

// TestRunBatch_ClosureCanSkipAWaveWithZeroStatsAndNilError covers the
// skip-on-timeout shape (internal/ai/embed, internal/search/searchdrain): a batch call whose
// context merely expired leaves the wave claimed for a later run's lease-expiry
// retry, rather than falling back per-item (which would turn one slow-but-fine batch
// into many equally slow calls). Reporting (Stats{}, nil) must tally nothing and
// must not trigger the outer per-item fallback either.
func TestRunBatch_ClosureCanSkipAWaveWithZeroStatsAndNilError(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2, 3}}}
	var outerFallbackCalls int

	processBatch := func(ctx context.Context, items []int) (Stats, error) {
		return Stats{}, nil // "handled" (left claimed for lease-expiry retry), no outcome yet
	}
	processOne := func(ctx context.Context, item int) Outcome {
		outerFallbackCalls++
		return Succeeded
	}

	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 3, LeaseSeconds: 60}, processBatch, processOne)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outerFallbackCalls != 0 {
		t.Errorf("outer processOne was called %d times, want 0", outerFallbackCalls)
	}
	if stats != (Stats{}) {
		t.Errorf("stats = %+v, want zero value", stats)
	}
	if claimer.calls < 2 {
		t.Errorf("claim calls = %d, want >=2 (the loop must keep going after a skipped wave, not stop)", claimer.calls)
	}
}

func TestRunBatchNilFallbackContinuesAfterFailure(t *testing.T) {
	claimer := &fakeClaimer{waves: [][]int{{1, 2}, {3}}}
	stats, err := RunBatch(context.Background(), claimer, RunOptions{BatchSize: 2}, func(_ context.Context, items []int) (Stats, error) {
		if items[0] == 1 {
			return Stats{Succeeded: 99}, errors.New("batch failed")
		}
		return Stats{Succeeded: len(items)}, nil
	}, nil)
	if err != nil || stats.Failed != 2 || stats.Succeeded != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
}
