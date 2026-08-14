package similarjobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory Store. PendingJobIDs replays a fixed sequence of "waves"
// (mirrors internal/embed's fakeClaimer), then returns empty forever — the shape a
// real live "what's missing" query has: each call reflects the current state, and a
// job that got written no longer matches.
type fakeStore struct {
	mu sync.Mutex

	waves            [][]int64
	pendingCalls     int
	requestedLimit   []int
	pendingErr       error
	pendingErrOnCall int

	genErr   map[int64]error
	genCalls []int64

	nearest      map[int64][]int64
	nearestErr   map[int64]error
	nearestCalls []int64

	setErr    map[int64]error
	staleIDs  map[int64]bool
	written   map[int64][]int64
	setCalls  []int64
	setGenArg map[int64]string

	// arrived/release let a test synchronize on concurrent NearestJobs calls
	// (mirrors internal/embed's blockUntilCtxDone-style fixtures).
	arrived chan int64
	release chan struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		genErr:  map[int64]error{},
		nearest: map[int64][]int64{}, nearestErr: map[int64]error{},
		setErr: map[int64]error{}, staleIDs: map[int64]bool{},
		written: map[int64][]int64{}, setGenArg: map[int64]string{},
	}
}

// JobGeneration returns a deterministic per-job value ("gen-<id>") unless overridden
// via genErr — good enough for tests that don't specifically exercise the generation
// guard, and distinct enough per job for the ones that do.
func (s *fakeStore) JobGeneration(_ context.Context, jobID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.genCalls = append(s.genCalls, jobID)
	if err := s.genErr[jobID]; err != nil {
		return "", err
	}
	return fmt.Sprintf("gen-%d", jobID), nil
}

func (s *fakeStore) PendingJobIDs(_ context.Context, limit int) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestedLimit = append(s.requestedLimit, limit)
	s.pendingCalls++
	if s.pendingErrOnCall != 0 && s.pendingCalls == s.pendingErrOnCall {
		return nil, s.pendingErr
	}
	if s.pendingCalls > len(s.waves) {
		return nil, nil
	}
	return s.waves[s.pendingCalls-1], nil
}

func (s *fakeStore) NearestJobs(_ context.Context, jobID int64, _ int) ([]int64, error) {
	s.mu.Lock()
	arrived := s.arrived
	release := s.release
	s.nearestCalls = append(s.nearestCalls, jobID)
	s.mu.Unlock()

	if arrived != nil {
		arrived <- jobID
	}
	if release != nil {
		<-release
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.nearestErr[jobID]; err != nil {
		return nil, err
	}
	return s.nearest[jobID], nil
}

func (s *fakeStore) SetSimilarJobIDs(_ context.Context, jobID int64, ids []int64, generation string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, jobID)
	s.setGenArg[jobID] = generation
	if err := s.setErr[jobID]; err != nil {
		return false, err
	}
	if s.staleIDs[jobID] {
		return false, nil
	}
	s.written[jobID] = ids
	return true, nil
}

func opt() RunOptions {
	return RunOptions{BatchSize: 500, NeighbourCount: 20, Concurrency: 4}
}

func TestRunnerDrainsMultipleBatches(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}, {4, 5}}
	for _, id := range []int64{1, 2, 3, 4, 5} {
		store.nearest[id] = []int64{id * 10}
	}

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 5 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want processed=5 failed=0", stats)
	}
	for _, id := range []int64{1, 2, 3, 4, 5} {
		got := store.written[id]
		if len(got) != 1 || got[0] != id*10 {
			t.Errorf("written[%d] = %v, want [%d]", id, got, id*10)
		}
	}
	// A third PendingJobIDs call must have happened to discover the queue is
	// drained (waves exhausted → empty result → loop exits).
	if store.pendingCalls != 3 {
		t.Errorf("pendingCalls = %d, want 3", store.pendingCalls)
	}
}

func TestRunnerWritesEmptyResultAsValidOutcome(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1}}
	// job 1 has no entry in store.nearest: NearestJobs returns nil, not an error —
	// a job whose only close matches were same-company must still be stamped so
	// it isn't endlessly re-picked.

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want processed=1 failed=0", stats)
	}
	if got, ok := store.written[1]; !ok {
		t.Fatalf("job 1 was never written")
	} else if len(got) != 0 {
		t.Errorf("written[1] = %v, want empty", got)
	}
}

// TestRunnerFailedJobNotRetriedWithinSameRun_ButCountedAsFailed exercises the
// attempted-set guard: job 2 fails and — because a real PendingJobIDs would keep
// returning it (similar_computed_at stays NULL) — resurfaces in the NEXT wave
// alongside fresh jobs. The runner must skip it rather than loop on it forever, while
// still counting it as Failed.
func TestRunnerFailedJobNotRetriedWithinSameRun_ButCountedAsFailed(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}, {2, 4, 5}} // job 2 reappears — it never got written
	store.nearestErr[2] = errors.New("nearest jobs boom")
	for _, id := range []int64{1, 3, 4, 5} {
		store.nearest[id] = []int64{id * 100}
	}

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 4 || stats.Failed != 1 {
		t.Fatalf("stats = %+v, want processed=4 failed=1", stats)
	}
	if _, ok := store.written[2]; ok {
		t.Errorf("job 2 was written despite failing NearestJobs")
	}
	// job 2's NearestJobs must have been attempted exactly once, not twice —
	// proof the attempted-set guard actually suppressed its second appearance in
	// wave 2 rather than looping on it.
	var nearestCallsFor2 int
	for _, id := range store.nearestCalls {
		if id == 2 {
			nearestCallsFor2++
		}
	}
	if nearestCallsFor2 != 1 {
		t.Errorf("NearestJobs called for job 2 %d times, want exactly 1", nearestCallsFor2)
	}
	// job 2 must never reach SetSimilarJobIDs at all (NearestJobs failed first).
	for _, id := range store.setCalls {
		if id == 2 {
			t.Errorf("SetSimilarJobIDs called for job 2, want it never called (NearestJobs failed first)")
		}
	}
	// Three PendingJobIDs calls: wave1, wave2, then the empty terminal call.
	if store.pendingCalls != 3 {
		t.Errorf("pendingCalls = %d, want 3", store.pendingCalls)
	}
}

// TestRunnerCountsWriteFailureAsFailedAfterSuccessfulCompute distinguishes the
// SetSimilarJobIDs failure branch of processOne from the NearestJobs failure branch
// already covered above: neighbours compute fine, but the write fails. The job must
// still be counted Failed (not Processed) and NOT appear in written — but unlike the
// NearestJobs-failure case, NearestJobs itself DID run (and succeed) first.
func TestRunnerCountsWriteFailureAsFailedAfterSuccessfulCompute(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}}
	store.nearest[1] = []int64{10}
	store.nearest[2] = []int64{20}
	store.nearest[3] = []int64{30}
	store.setErr[2] = errors.New("set similar job ids boom")

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 2 || stats.Failed != 1 {
		t.Fatalf("stats = %+v, want processed=2 failed=1", stats)
	}
	if _, ok := store.written[2]; ok {
		t.Errorf("job 2 was written despite SetSimilarJobIDs failing")
	}
	// Distinguishing evidence from the NearestJobs-failure test: NearestJobs ran
	// (and succeeded) for job 2 exactly once before the write failed.
	var nearestCallsFor2 int
	for _, id := range store.nearestCalls {
		if id == 2 {
			nearestCallsFor2++
		}
	}
	if nearestCallsFor2 != 1 {
		t.Errorf("NearestJobs called for job 2 %d times, want exactly 1 (compute succeeded before the write failed)", nearestCallsFor2)
	}
	// SetSimilarJobIDs must have been attempted for job 2 (it's the call that
	// failed) — this is what distinguishes this test from the NearestJobs-failure
	// case above, where SetSimilarJobIDs is never reached at all.
	var setCallsFor2 int
	for _, id := range store.setCalls {
		if id == 2 {
			setCallsFor2++
		}
	}
	if setCallsFor2 != 1 {
		t.Errorf("SetSimilarJobIDs called for job 2 %d times, want exactly 1", setCallsFor2)
	}
	if got := store.written[1]; len(got) != 1 || got[0] != 10 {
		t.Errorf("written[1] = %v, want [10] (job 1 unaffected by job 2's write failure)", got)
	}
	if got := store.written[3]; len(got) != 1 || got[0] != 30 {
		t.Errorf("written[3] = %v, want [30] (job 3 unaffected by job 2's write failure)", got)
	}
}

func TestRunnerMaxJobsStopsEarlyAndTruncatesFinalBatch(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}, {4, 5, 6}}
	for _, id := range []int64{1, 2, 3, 4, 5, 6} {
		store.nearest[id] = []int64{id}
	}

	o := opt()
	o.MaxJobs = 4
	stats, err := Runner{Store: store}.Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 4 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want processed=4 failed=0", stats)
	}
	// Only job 4 from the second wave should have been processed (truncated to
	// the remaining budget of 1).
	if _, ok := store.written[4]; !ok {
		t.Errorf("job 4 not written, want it processed (first of the second wave)")
	}
	if _, ok := store.written[5]; ok {
		t.Errorf("job 5 written, want it left for a later run (budget exhausted)")
	}
	if _, ok := store.written[6]; ok {
		t.Errorf("job 6 written, want it left for a later run (budget exhausted)")
	}
	// Must stop BEFORE a third PendingJobIDs call once the budget is spent.
	if store.pendingCalls != 2 {
		t.Errorf("pendingCalls = %d, want 2 (must not claim again once MaxJobs is reached)", store.pendingCalls)
	}
}

// TestRunnerStaleGenerationWriteNotProcessedNorFailed exercises the fix for the
// generation race between this worker's read (JobGeneration/NearestJobs) and its write
// (SetSimilarJobIDs): a concurrent cmd/embed re-embed of job 2 between the two makes
// the store's SetSimilarJobIDs report applied=false. That must count toward neither
// Processed (nothing valid was written) nor Failed (it's not an error — cmd/embed's own
// re-embed already left similar_computed_at NULL for a later run to redo), while still
// being suppressed from retrying within this same run (mirrors the failed-job guard).
func TestRunnerStaleGenerationWriteNotProcessedNorFailed(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}, {2, 4}} // job 2 reappears — it never got written
	store.staleIDs[2] = true
	for _, id := range []int64{1, 2, 3, 4} {
		store.nearest[id] = []int64{id * 10}
	}

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 3 || stats.Failed != 0 || stats.Stale != 1 {
		t.Fatalf("stats = %+v, want processed=3 failed=0 stale=1", stats)
	}
	if _, ok := store.written[2]; ok {
		t.Errorf("job 2 was written despite losing the generation race")
	}
	// job 2's SetSimilarJobIDs must have been attempted exactly once, not twice —
	// proof the same in-run guard that suppresses failures also suppresses a stale
	// write's second appearance in wave 2.
	var setCallsFor2 int
	for _, id := range store.setCalls {
		if id == 2 {
			setCallsFor2++
		}
	}
	if setCallsFor2 != 1 {
		t.Errorf("SetSimilarJobIDs called for job 2 %d times, want exactly 1", setCallsFor2)
	}
	// The generation JobGeneration returned for job 1 must be the exact value passed
	// to its SetSimilarJobIDs call — proof the read-then-guarded-write plumbing
	// actually threads the value through, not just that both are called.
	if got, want := store.setGenArg[1], "gen-1"; got != want {
		t.Errorf("SetSimilarJobIDs generation arg for job 1 = %q, want %q", got, want)
	}
}

func TestRunnerNoPendingWorkIsANoOp(t *testing.T) {
	store := newFakeStore() // no waves at all

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 0 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want all zero", stats)
	}
	if store.pendingCalls != 1 {
		t.Errorf("pendingCalls = %d, want 1", store.pendingCalls)
	}
}

func TestRunnerAbortsRunOnPendingJobIDsError(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2}}
	store.nearest[1] = []int64{10}
	store.nearest[2] = []int64{20}
	store.pendingErrOnCall = 2
	store.pendingErr = errors.New("pool unreachable")

	stats, err := Runner{Store: store}.Run(context.Background(), opt())
	if !errors.Is(err, store.pendingErr) {
		t.Fatalf("err = %v, want wrapping %v", err, store.pendingErr)
	}
	if stats.Processed != 2 {
		t.Errorf("stats = %+v, want processed=2 (the first wave, processed before the failing claim)", stats)
	}
}

// TestRunnerConcurrencyRunsJobsInParallel proves Concurrency > 1 actually overlaps
// NearestJobs calls instead of serializing them — mirrors
// internal/embed's TestRunPool_RunsUpToConcurrencyItemsInParallel.
func TestRunnerConcurrencyRunsJobsInParallel(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}}
	store.arrived = make(chan int64, 3)
	store.release = make(chan struct{})
	for _, id := range []int64{1, 2, 3} {
		store.nearest[id] = []int64{id}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		o := opt()
		o.Concurrency = 3
		_, _ = Runner{Store: store}.Run(context.Background(), o)
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-store.arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for concurrent arrivals; got %d/3", i)
		}
	}
	close(store.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after release")
	}
}

// TestRunnerConcurrencyOneProcessesSequentially proves Concurrency<=1 does not
// parallelize — a control complementing the concurrency test above.
func TestRunnerConcurrencyOneProcessesSequentially(t *testing.T) {
	store := newFakeStore()
	store.waves = [][]int64{{1, 2, 3}}
	store.nearest[1] = []int64{1}
	store.nearest[2] = []int64{2}
	store.nearest[3] = []int64{3}

	// Assert the natural FIFO order held via setCalls (already recorded by the
	// fake) — only concurrency=1 guarantees this deterministically for a
	// channel-fed pool draining a single-producer queue in order.
	o := opt()
	o.Concurrency = 1
	stats, err := Runner{Store: store}.Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 3 {
		t.Fatalf("stats = %+v, want processed=3", stats)
	}
	want := []int64{1, 2, 3}
	if len(store.setCalls) != len(want) {
		t.Fatalf("setCalls = %v, want %v", store.setCalls, want)
	}
	for i, id := range want {
		if store.setCalls[i] != id {
			t.Errorf("setCalls[%d] = %d, want %d", i, store.setCalls[i], id)
		}
	}
}
