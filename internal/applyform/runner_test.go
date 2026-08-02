package applyform

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeStore is the queue seen from the runner: waves of claims, recorded completions and
// recorded failures. It hands out each wave once, so a drain loop terminates.
type fakeStore struct {
	mu sync.Mutex

	waves     [][]Claimed
	claimErr  error
	saveErr   error
	saved     map[int64]Form
	failures  map[int64]string
	attempts  map[int64]int
	maxClaims int
}

func newFakeStore(waves ...[]Claimed) *fakeStore {
	return &fakeStore{
		waves:    waves,
		saved:    map[int64]Form{},
		failures: map[int64]string{},
		attempts: map[int64]int{},
	}
}

// Claim honours the batch size the way the real query's LIMIT does — a fake that ignored
// it would let a bounded run overshoot its budget and still pass.
func (s *fakeStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	s.maxClaims++
	if len(s.waves) == 0 {
		return nil, nil
	}
	wave := s.waves[0]
	if batch > 0 && len(wave) > batch {
		s.waves[0] = wave[batch:]
		return wave[:batch], nil
	}
	s.waves = s.waves[1:]
	return wave, nil
}

func (s *fakeStore) Save(_ context.Context, _ int64, jobID int64, form Form) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved[jobID] = form
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, msg string, maxAttempts int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[outboxID] = msg
	s.attempts[outboxID]++
	return s.attempts[outboxID] >= maxAttempts, nil
}

// fakeFetcher answers per posting id: a form, or an error.
type fakeFetcher struct {
	mu      sync.Mutex
	forms   map[string]Form
	errs    map[string]error
	calls   int
	inFlite int
	peak    int
}

func (f *fakeFetcher) Fetch(_ context.Context, _, postingID string) (Form, error) {
	f.mu.Lock()
	f.calls++
	f.inFlite++
	if f.inFlite > f.peak {
		f.peak = f.inFlite
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlite--; f.mu.Unlock() }()

	if err := f.errs[postingID]; err != nil {
		return Form{}, err
	}
	return f.forms[postingID], nil
}

func claim(outboxID, jobID int64, provider, externalID string) Claimed {
	return Claimed{OutboxID: outboxID, JobID: jobID, Provider: provider, ExternalID: externalID}
}

func testOptions() RunOptions {
	return RunOptions{BatchSize: 10, LeaseSeconds: 3600, MaxAttempts: 3, Concurrency: 2}
}

func TestRunCapturesAndStores(t *testing.T) {
	store := newFakeStore([]Claimed{
		claim(1, 100, "greenhouse", "stripe:7954688"),
		claim(2, 200, "ashby", "n8n:47cc47be"),
	})
	fetcher := &fakeFetcher{forms: map[string]Form{
		"7954688":  {Provider: "greenhouse", Fields: []Field{{ID: "email"}}},
		"47cc47be": {Provider: "ashby", Fields: []Field{{ID: "_systemfield_email"}}},
	}}

	stats, err := Run(context.Background(), store, map[string]Fetcher{
		"greenhouse": fetcher, "ashby": fetcher,
	}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Captured != 2 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want 2 captured and none failed", stats)
	}
	if got := store.saved[100]; len(got.Fields) != 1 || got.Fields[0].ID != "email" {
		t.Errorf("job 100 stored %+v, want the fetched Greenhouse form", got)
	}
	if _, ok := store.saved[200]; !ok {
		t.Error("job 200 not stored")
	}
}

// One platform erroring must not cost the postings beside it — a board having a bad
// afternoon should not stall the whole queue.
func TestRunIsolatesAFailingCapture(t *testing.T) {
	store := newFakeStore([]Claimed{
		claim(1, 100, "greenhouse", "b:good"),
		claim(2, 200, "greenhouse", "b:bad"),
		claim(3, 300, "greenhouse", "b:alsogood"),
	})
	fetcher := &fakeFetcher{
		forms: map[string]Form{"good": {Provider: "greenhouse"}, "alsogood": {Provider: "greenhouse"}},
		errs:  map[string]error{"bad": errors.New("upstream 500")},
	}

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Captured != 2 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want 2 captured and 1 failed", stats)
	}
	if _, ok := store.saved[100]; !ok {
		t.Error("the posting before the failure was not stored")
	}
	if _, ok := store.saved[300]; !ok {
		t.Error("the posting after the failure was not stored")
	}
	if msg := store.failures[2]; msg == "" {
		t.Error("the failure recorded no message")
	}
}

// The whole point of a bounded pool is that one run cannot turn into a flood aimed at one
// platform.
func TestRunBoundsConcurrency(t *testing.T) {
	var claims []Claimed
	forms := map[string]Form{}
	for i := range 12 {
		id := string(rune('a' + i))
		claims = append(claims, claim(int64(i+1), int64(i+100), "greenhouse", "b:"+id))
		forms[id] = Form{Provider: "greenhouse"}
	}
	store := newFakeStore(claims)
	fetcher := &fakeFetcher{forms: forms}

	opts := testOptions()
	opts.Concurrency = 3
	if _, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if fetcher.peak > 3 {
		t.Errorf("peak concurrent fetches = %d, want at most 3", fetcher.peak)
	}
	if fetcher.calls != 12 {
		t.Errorf("fetched %d postings, want all 12", fetcher.calls)
	}
}

// The drain keeps going until the queue is empty, not for one wave.
func TestRunDrainsEveryWave(t *testing.T) {
	store := newFakeStore(
		[]Claimed{claim(1, 100, "greenhouse", "b:one")},
		[]Claimed{claim(2, 200, "greenhouse", "b:two")},
	)
	fetcher := &fakeFetcher{forms: map[string]Form{
		"one": {Provider: "greenhouse"}, "two": {Provider: "greenhouse"},
	}}

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Captured != 2 {
		t.Errorf("captured %d, want both waves drained", stats.Captured)
	}
}

// A capture that has failed its last permitted attempt is counted apart, because a run
// whose queue is quietly filling with dead letters is not a healthy run.
func TestRunCountsDeadLetters(t *testing.T) {
	store := newFakeStore([]Claimed{claim(1, 100, "greenhouse", "b:bad")})
	store.attempts[1] = 2 // one attempt short of the max
	fetcher := &fakeFetcher{errs: map[string]error{"bad": errors.New("gone")}}

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.DeadLettered != 1 {
		t.Errorf("dead-lettered = %d, want 1", stats.DeadLettered)
	}
}

// The enqueue gate and this registry share one source of truth, so a claim for a provider
// with no fetcher should be impossible. If one appears anyway it must be failed, not
// silently dropped — otherwise it is re-claimed on every run, forever.
func TestRunFailsAClaimItCannotFetch(t *testing.T) {
	store := newFakeStore([]Claimed{claim(1, 100, "workday", "tenant:R1")})

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": &fakeFetcher{}}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Failed != 1 {
		t.Errorf("stats = %+v, want the unfetchable claim counted as failed", stats)
	}
	if store.failures[1] == "" {
		t.Error("the unfetchable claim was dropped rather than failed")
	}
}

// A malformed external_id cannot be split into a board and a posting id, so there is
// nothing to fetch — and no later run will do better.
func TestRunFailsAnUnsplittableIdentity(t *testing.T) {
	store := newFakeStore([]Claimed{claim(1, 100, "greenhouse", "nocolon")})

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": &fakeFetcher{}}, testOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Failed != 1 || store.failures[1] == "" {
		t.Errorf("stats = %+v, failures = %v, want the claim failed", stats, store.failures)
	}
}

// A claim failure is the queue itself being unreachable — there is no per-item recovery
// from it, so the run stops and says so.
func TestRunReturnsAClaimFailure(t *testing.T) {
	store := newFakeStore()
	store.claimErr = errors.New("connection refused")

	if _, err := Run(context.Background(), store, nil, testOptions()); err == nil {
		t.Error("Run() = nil error when the queue could not be claimed")
	}
}

// A run takes a bounded bite of the backlog rather than all of it. The first production
// drain faces ~185k postings; without a budget one process works for hours, and because
// nothing here holds a lock — a systemd Type=oneshot unit simply refuses to start a second
// instance — that one long run silently swallows every scheduled firing behind it.
func TestRunStopsAtItsBudget(t *testing.T) {
	var waves [][]Claimed
	forms := map[string]Form{}
	for w := range 4 {
		var wave []Claimed
		for i := range 5 {
			id := string(rune('a'+w)) + string(rune('0'+i))
			wave = append(wave, claim(int64(w*5+i+1), int64(w*5+i+100), "greenhouse", "b:"+id))
			forms[id] = Form{Provider: "greenhouse"}
		}
		waves = append(waves, wave)
	}
	store := newFakeStore(waves...)
	fetcher := &fakeFetcher{forms: forms}

	opts := testOptions()
	opts.BatchSize = 5
	opts.MaxPerRun = 12

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stats.Captured > 12 {
		t.Errorf("captured %d, want no more than the budget of 12", stats.Captured)
	}
	if stats.Captured < 10 {
		t.Errorf("captured %d, want the run to take a real bite before stopping", stats.Captured)
	}
	// The rest of the backlog is still queued, waiting for the next run.
	if len(store.waves) == 0 {
		t.Error("the whole backlog was drained, want the budget to leave work for a later run")
	}
}

// A budget of zero is unbounded — the shape a one-off manual catch-up wants.
func TestRunWithoutABudgetDrainsEverything(t *testing.T) {
	store := newFakeStore(
		[]Claimed{claim(1, 100, "greenhouse", "b:one")},
		[]Claimed{claim(2, 200, "greenhouse", "b:two")},
	)
	fetcher := &fakeFetcher{forms: map[string]Form{
		"one": {Provider: "greenhouse"}, "two": {Provider: "greenhouse"},
	}}

	opts := testOptions()
	opts.MaxPerRun = 0

	stats, err := Run(context.Background(), store, map[string]Fetcher{"greenhouse": fetcher}, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Captured != 2 {
		t.Errorf("captured %d, want the whole queue", stats.Captured)
	}
}

// A cancelled run stops rather than working through the backlog burning retry budget.
func TestRunStopsWhenCancelled(t *testing.T) {
	store := newFakeStore(
		[]Claimed{claim(1, 100, "greenhouse", "b:one")},
		[]Claimed{claim(2, 200, "greenhouse", "b:two")},
	)
	fetcher := &fakeFetcher{forms: map[string]Form{"one": {Provider: "greenhouse"}}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Run(ctx, store, map[string]Fetcher{"greenhouse": fetcher}, testOptions()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.waves) == 0 {
		t.Error("a cancelled run drained every wave, want it to stop")
	}
}

// A run's exit status has to separate work POSTPONED from work ABANDONED. Against
// hundreds of thousands of third-party endpoints a handful of transient failures per run
// is normal and self-healing — the queue retries them — so treating them as a degraded run
// paints the unit red on nearly every firing and the status stops meaning anything. A dead
// letter is different: that work is given up on, and nobody will look at it again.
func TestRunOutcomeSeparatesPostponedFromAbandoned(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stats    RunStats
		degraded bool
	}{
		{"a clean run", RunStats{Captured: 2500}, false},
		{"a few transient failures among many captures", RunStats{Captured: 2490, Failed: 10}, false},
		{"an abandoned capture", RunStats{Captured: 2490, Failed: 10, DeadLettered: 1}, true},
		// Nothing captured while everything failed is not a blip — it is the platform
		// being gone, and it must not hide behind the same forgiveness.
		{"every capture failed", RunStats{Failed: 40}, true},
		{"an empty queue", RunStats{}, false},
	} {
		if got := tc.stats.Degraded(); got != tc.degraded {
			t.Errorf("%s: Degraded() = %v, want %v (%+v)", tc.name, got, tc.degraded, tc.stats)
		}
	}
}
