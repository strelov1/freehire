package adzunadesc

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeStore is an in-memory Store: enough queue semantics to drive Run through a few
// waves without a database. Not concurrency-hardened beyond a mutex — the point is
// deterministic assertions, not modeling Postgres's SKIP LOCKED.
type fakeStore struct {
	mu             sync.Mutex
	pending        []Claimed
	saved          map[int64]string // outboxID -> description
	failed         map[int64]int    // outboxID -> fail count
	deadLetteredID []int64
	discardedID    []int64
	claimErr       error
}

func (s *fakeStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	if batch > len(s.pending) {
		batch = len(s.pending)
	}
	out := s.pending[:batch]
	s.pending = s.pending[batch:]
	return out, nil
}

func (s *fakeStore) Save(_ context.Context, outboxID, _ int64, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saved == nil {
		s.saved = map[int64]string{}
	}
	s.saved[outboxID] = description
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, _ string, maxAttempts int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed == nil {
		s.failed = map[int64]int{}
	}
	s.failed[outboxID]++
	dead := s.failed[outboxID] >= maxAttempts
	if dead {
		s.deadLetteredID = append(s.deadLetteredID, outboxID)
	}
	return dead, nil
}

func (s *fakeStore) Discard(_ context.Context, outboxID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardedID = append(s.discardedID, outboxID)
	return nil
}

func TestRunDiscardsAClaimWhoseURLDriftedToTheAdNetworkRedirect(t *testing.T) {
	// A job's stored URL was Adzuna's own hosted page when this row was enqueued, but a
	// later re-crawl overwrote it with the ad-network tracking-redirect shape before the
	// worker got to it — reproduces the drift confirmed live 2026-08-09 (03:03 enqueue,
	// 08:25 job re-crawled with a /land/ad/ URL, 10:18 claim 403-ed three times and
	// dead-lettered). The claim carries the CURRENT (now-ineligible) URL.
	store := &fakeStore{pending: []Claimed{
		{OutboxID: 1, JobID: 10, URL: "https://www.adzuna.co.uk/jobs/land/ad/5829591081?se=x"},
		{OutboxID: 2, JobID: 20, URL: "https://www.adzuna.co.uk/jobs/details/2"},
	}}
	fetchCalled := 0
	fetch := func(_ context.Context, url string) (string, error) {
		fetchCalled++
		return "full text for " + url, nil
	}

	stats, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Captured != 1 {
		t.Errorf("Captured = %d, want 1 (the still-eligible claim)", stats.Captured)
	}
	if stats.Failed != 0 || stats.DeadLettered != 0 {
		t.Errorf("stats = %+v, want no Failed/DeadLettered — a drifted URL is not retried", stats)
	}
	if fetchCalled != 1 {
		t.Errorf("fetch called %d times, want 1 (never called for the drifted claim)", fetchCalled)
	}
	if len(store.discardedID) != 1 || store.discardedID[0] != 1 {
		t.Errorf("discarded = %v, want [1]", store.discardedID)
	}
	if stats.Degraded() {
		t.Error("a discarded drift should not mark the run degraded")
	}
}

func TestRunCapturesEveryClaim(t *testing.T) {
	store := &fakeStore{pending: []Claimed{
		{OutboxID: 1, JobID: 10, URL: "https://www.adzuna.co.uk/jobs/details/1"},
		{OutboxID: 2, JobID: 20, URL: "https://www.adzuna.co.uk/jobs/details/2"},
	}}
	fetch := func(_ context.Context, url string) (string, error) {
		return "full text for " + url, nil
	}

	stats, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Captured != 2 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Captured=2 Failed=0", stats)
	}
	if len(store.saved) != 2 {
		t.Errorf("saved %d entries, want 2", len(store.saved))
	}
}

func TestRunRetriesThenDeadLettersAFailingFetch(t *testing.T) {
	store := &fakeStore{pending: []Claimed{{OutboxID: 1, JobID: 10, URL: "https://www.adzuna.co.uk/jobs/details/1"}}}
	fetch := func(_ context.Context, _ string) (string, error) {
		return "", errors.New("blocked")
	}

	stats, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 1, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Failed != 1 || stats.DeadLettered != 1 {
		t.Errorf("stats = %+v, want Failed=1 DeadLettered=1", stats)
	}
	if !stats.Degraded() {
		t.Error("a dead letter should mark the run degraded")
	}
}

func TestRunDeadLettersATerminalFailureOnTheFirstAttempt(t *testing.T) {
	// errNoJobPosting/errEmptyDescription/a 404 are deterministic: the same URL will
	// answer the same way every time, so retrying wastes two more requests against a page
	// already known to fail — confirmed live 2026-08-09 (a normal-looking, non-blocked
	// Adzuna page whose ld+json simply carries no JobPosting block, majority failure mode
	// of the first production runs).
	store := &fakeStore{pending: []Claimed{{OutboxID: 1, JobID: 10, URL: "https://www.adzuna.co.uk/jobs/details/1"}}}
	fetch := func(_ context.Context, _ string) (string, error) {
		return "", errNoJobPosting
	}

	// MaxAttempts=3 would normally take three failing runs to dead-letter (see
	// TestRunRetriesThenDeadLettersAFailingFetch, which needs MaxAttempts=1 to dead-letter
	// in one call) — a terminal error skips straight there on the FIRST attempt.
	stats, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 1, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Failed != 1 || stats.DeadLettered != 1 {
		t.Errorf("stats = %+v, want Failed=1 DeadLettered=1 (dead-lettered on the first attempt)", stats)
	}
	if store.failed[1] != 1 {
		t.Errorf("Fail was called %d times for outbox 1, want exactly 1", store.failed[1])
	}
}

func TestRunStopsAtMaxPerRun(t *testing.T) {
	store := &fakeStore{pending: []Claimed{
		{OutboxID: 1, JobID: 10, URL: "https://www.adzuna.co.uk/jobs/details/1"},
		{OutboxID: 2, JobID: 20, URL: "https://www.adzuna.co.uk/jobs/details/2"},
		{OutboxID: 3, JobID: 30, URL: "https://www.adzuna.co.uk/jobs/details/3"},
	}}
	fetch := func(_ context.Context, url string) (string, error) { return "text", nil }

	stats, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 1, MaxPerRun: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Captured != 2 {
		t.Errorf("Captured = %d, want 2 (bounded by MaxPerRun)", stats.Captured)
	}
}

func TestRunReturnsErrorWhenClaimFails(t *testing.T) {
	store := &fakeStore{claimErr: errors.New("db down")}
	fetch := func(_ context.Context, _ string) (string, error) { return "text", nil }

	_, err := Run(context.Background(), store, fetch, RunOptions{BatchSize: 10, Concurrency: 1})
	if err == nil {
		t.Fatal("expected an error when the queue itself is unusable")
	}
}
