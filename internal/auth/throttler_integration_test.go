//go:build integration

// Integration test for PGThrottler against a real Postgres: the check-then-increment
// must be atomic under concurrent callers, or the rate limiter it backs (login, register,
// password reset) stops being a rate limiter under exactly the traffic pattern it exists
// to stop — a scripted brute-force or credential-stuffing burst.
// Run with: go test -tags=integration ./internal/auth/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package auth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/testdb"
)

func TestPGThrottler_ConcurrentRequestsNeverExceedTheLimit(t *testing.T) {
	pool := testdb.Pool(t)
	th := NewPGThrottler(pool)

	const limit = 5
	const attempts = 30
	var allowed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			ok, _, err := th.Allow(context.Background(), "race-key", limit, time.Minute)
			if err != nil {
				t.Errorf("Allow: %v", err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != limit {
		t.Errorf("allowed = %d concurrent requests, want exactly the configured limit %d", got, limit)
	}
}
