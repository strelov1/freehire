package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- fakes ------------------------------------------------------------------

// funcProvider is shared by a wave's workers, so the call count is mutex-guarded.
type funcProvider struct {
	fn func(JobInput) (Enrichment, error)

	mu    sync.Mutex
	calls int
}

func (p *funcProvider) Enrich(_ context.Context, j JobInput) (Enrichment, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.fn(j)
}

// callCount reads the tally after Run has joined all workers (race-free).
func (p *funcProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// fakeStore's Complete/Fail are called from concurrent workers, so the result
// slices are mutex-guarded.
type fakeStore struct {
	claims   [][]Claimed
	claimIdx int
	jobs     map[int64]JobInput
	jobErr   map[int64]error
	deadFor  map[int64]bool

	mu        sync.Mutex
	enqueued  bool
	completed []int64
	failed    []int64
}

func (s *fakeStore) Enqueue(_ context.Context, _ int) (int64, error) {
	s.enqueued = true
	return 0, nil
}

func (s *fakeStore) Claim(_ context.Context, _, _ int) ([]Claimed, error) {
	if s.claimIdx >= len(s.claims) {
		return nil, nil
	}
	c := s.claims[s.claimIdx]
	s.claimIdx++
	return c, nil
}

func (s *fakeStore) Job(_ context.Context, id int64) (JobInput, error) {
	return s.jobs[id], s.jobErr[id]
}

func (s *fakeStore) Complete(_ context.Context, entry Claimed, _ json.RawMessage) error {
	s.mu.Lock()
	s.completed = append(s.completed, entry.OutboxID)
	s.mu.Unlock()
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, _ string, _ int) (bool, error) {
	s.mu.Lock()
	s.failed = append(s.failed, outboxID)
	s.mu.Unlock()
	return s.deadFor[outboxID], nil
}

func opts() RunOptions {
	return RunOptions{TargetVersion: Version, Concurrency: 4, LeaseSeconds: 300, MaxAttempts: 3}
}

// --- tests ------------------------------------------------------------------

func TestRun_validIsWrittenAndDequeued(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 1, JobID: 100, TargetVersion: Version}}},
		jobs:   map[int64]JobInput{100: {Title: "Go dev"}},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		return Enrichment{Seniority: "senior", WorkMode: "remote"}, nil
	}}

	stats, err := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !store.enqueued {
		t.Error("expected Enqueue to be called")
	}
	if len(store.completed) != 1 || store.completed[0] != 1 {
		t.Errorf("completed = %v, want [1]", store.completed)
	}
	if len(store.failed) != 0 {
		t.Errorf("failed = %v, want none", store.failed)
	}
	if stats != (Stats{Enriched: 1}) {
		t.Errorf("stats = %+v, want {Enriched:1}", stats)
	}
}

func TestRun_outOfVocabEnumIsSanitizedAndWritten(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 7, JobID: 100, TargetVersion: Version}}},
		jobs:   map[int64]JobInput{100: {Title: "x"}},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		// "sr" is not a vocabulary value; "frontend" is. The stray value must be
		// dropped and the rest of the payload written — not dead-lettered.
		return Enrichment{Seniority: "sr", Category: "frontend"}, nil
	}}

	stats, err := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.completed) != 1 || store.completed[0] != 7 {
		t.Errorf("completed = %v, want [7] (sanitized payload written)", store.completed)
	}
	if len(store.failed) != 0 {
		t.Errorf("failed = %v, want none", store.failed)
	}
	if prov.callCount() != 1 {
		t.Errorf("provider called %d times, want 1 (sanitize fixes it, no retry)", prov.callCount())
	}
	if stats.Enriched != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Enriched:1", stats)
	}
}

func TestRun_providerErrorIsFailed(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 3, JobID: 100, TargetVersion: Version}}},
		jobs:   map[int64]JobInput{100: {}},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		return Enrichment{}, errors.New("llm down")
	}}

	stats, _ := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if len(store.completed) != 0 || len(store.failed) != 1 {
		t.Errorf("completed=%v failed=%v, want failed only", store.completed, store.failed)
	}
	if stats.Failed != 1 {
		t.Errorf("stats = %+v, want Failed:1", stats)
	}
}

func TestRun_transportErrorIsRetried(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 8, JobID: 100, TargetVersion: Version}}},
		jobs:   map[int64]JobInput{100: {Title: "x"}},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		return Enrichment{}, errors.New("gateway timeout")
	}}

	stats, _ := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if prov.callCount() != 2 {
		t.Errorf("provider called %d times, want 2 (a transport error is retried once)", prov.callCount())
	}
	if stats.Failed != 1 {
		t.Errorf("stats = %+v, want Failed:1", stats)
	}
}

func TestRun_unparseableResponseIsNotRetried(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 4, JobID: 100, TargetVersion: Version}}},
		jobs:   map[int64]JobInput{100: {Title: "x"}},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		// A non-JSON model response is deterministic for the prompt: an in-process
		// retry would re-send the identical call. The next cron run re-attempts it.
		return Enrichment{}, fmt.Errorf("%w: trailing garbage", errUnparseableResponse)
	}}

	stats, _ := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if prov.callCount() != 1 {
		t.Errorf("provider called %d times, want 1 (an unparseable response must not be retried in-process)", prov.callCount())
	}
	if len(store.failed) != 1 || stats.Failed != 1 {
		t.Errorf("failed=%v stats=%+v, want one failure", store.failed, stats)
	}
}

func TestRun_deadLetterCounted(t *testing.T) {
	store := &fakeStore{
		claims:  [][]Claimed{{{OutboxID: 9, JobID: 100, TargetVersion: Version}}},
		jobs:    map[int64]JobInput{100: {}},
		deadFor: map[int64]bool{9: true},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		return Enrichment{}, errors.New("boom")
	}}

	stats, _ := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if stats.DeadLettered != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want DeadLettered:1", stats)
	}
}

func TestRun_oneFailureDoesNotAbortBatch(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{
			{OutboxID: 1, JobID: 1, TargetVersion: Version},
			{OutboxID: 2, JobID: 2, TargetVersion: Version},
		}},
		jobs: map[int64]JobInput{1: {Title: "bad"}, 2: {Title: "good"}},
	}
	prov := &funcProvider{fn: func(j JobInput) (Enrichment, error) {
		if j.Title == "bad" {
			return Enrichment{}, errors.New("llm down")
		}
		return Enrichment{Seniority: "junior"}, nil
	}}

	stats, err := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.completed) != 1 || store.completed[0] != 2 {
		t.Errorf("completed = %v, want [2]", store.completed)
	}
	if len(store.failed) != 1 || store.failed[0] != 1 {
		t.Errorf("failed = %v, want [1]", store.failed)
	}
	if stats.Enriched != 1 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want Enriched:1 Failed:1", stats)
	}
}

func TestRun_waveDrainsConcurrently(t *testing.T) {
	const wave = 3
	claimed := make([]Claimed, wave)
	jobs := map[int64]JobInput{}
	for i := range claimed {
		id := int64(i + 1)
		claimed[i] = Claimed{OutboxID: id, JobID: id, TargetVersion: Version}
		jobs[id] = JobInput{Title: "j"}
	}
	store := &fakeStore{claims: [][]Claimed{claimed}, jobs: jobs}

	// Barrier: each worker signals arrival then blocks on release. The main goroutine
	// only releases once all `wave` workers have arrived, so a sequential drain (which
	// would block on the first worker forever) cannot reach the barrier and times out.
	var arrived sync.WaitGroup
	arrived.Add(wave)
	release := make(chan struct{})
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		arrived.Done()
		<-release
		return Enrichment{Seniority: "senior"}, nil
	}}

	allArrived := make(chan struct{})
	go func() { arrived.Wait(); close(allArrived) }()

	done := make(chan Stats)
	go func() {
		stats, err := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
		if err != nil {
			t.Errorf("Run: %v", err)
		}
		done <- stats
	}()

	select {
	case <-allArrived:
		close(release)
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not run concurrently: not all entries reached the barrier (sequential drain)")
	}

	stats := <-done
	if stats.Enriched != wave || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Enriched:%d", stats, wave)
	}
	if len(store.completed) != wave {
		t.Errorf("completed = %d entries, want %d", len(store.completed), wave)
	}
}

func TestRun_jobFetchErrorIsFailed(t *testing.T) {
	store := &fakeStore{
		claims: [][]Claimed{{{OutboxID: 5, JobID: 100, TargetVersion: Version}}},
		jobErr: map[int64]error{100: errors.New("gone")},
	}
	prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
		t.Fatal("provider should not be called when the job fetch fails")
		return Enrichment{}, nil
	}}

	stats, _ := Runner{Provider: prov, Store: store}.Run(context.Background(), opts())
	if len(store.failed) != 1 || stats.Failed != 1 {
		t.Errorf("failed=%v stats=%+v, want one failure", store.failed, stats)
	}
}
