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
	store := &fakeStore{pending: []Claimed{{OutboxID: 1, JobID: 10, URL: "https://x"}}}
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

func TestRunStopsAtMaxPerRun(t *testing.T) {
	store := &fakeStore{pending: []Claimed{
		{OutboxID: 1, JobID: 10, URL: "https://a"},
		{OutboxID: 2, JobID: 20, URL: "https://b"},
		{OutboxID: 3, JobID: 30, URL: "https://c"},
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
