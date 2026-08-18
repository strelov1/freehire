package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisThrottler(t *testing.T) *RedisThrottler {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisThrottler(client)
}

func TestRedisThrottler_AllowsWithinLimit(t *testing.T) {
	th := newTestRedisThrottler(t)

	for i := 0; i < 3; i++ {
		decision, err := th.Allow(context.Background(), "key1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow %d: %v", i, err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d should be allowed (within limit 3)", i)
		}
	}
}

func TestRedisThrottler_RejectsOverLimit(t *testing.T) {
	th := newTestRedisThrottler(t)

	for i := 0; i < 2; i++ {
		if _, err := th.Allow(context.Background(), "key2", 2, time.Minute); err != nil {
			t.Fatalf("Allow %d: %v", i, err)
		}
	}

	decision, err := th.Allow(context.Background(), "key2", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if decision.Allowed {
		t.Fatal("3rd request should be rejected (limit 2)")
	}
	if decision.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0", decision.RetryAfter)
	}
}

func TestRedisThrottler_SeparateKeysAreIndependent(t *testing.T) {
	th := newTestRedisThrottler(t)

	if _, err := th.Allow(context.Background(), "a", 1, time.Minute); err != nil {
		t.Fatalf("Allow a: %v", err)
	}
	a2, _ := th.Allow(context.Background(), "a", 1, time.Minute)
	if a2.Allowed {
		t.Fatal("key a should be exhausted after 1 request (limit 1)")
	}

	b1, err := th.Allow(context.Background(), "b", 1, time.Minute)
	if err != nil {
		t.Fatalf("Allow b: %v", err)
	}
	if !b1.Allowed {
		t.Fatal("key b should be independent of key a")
	}
}

func TestRedisThrottler_ReturnsErrorWhenRedisUnreachable(t *testing.T) {
	// RedisThrottler reports backend errors rather than handling them — Middleware
	// is the single place that decides to fail open (see TestMiddleware_WithRedisThrottler_FailsOpenWhenRedisUnreachable).
	//
	// Port 1 is a reserved low port nothing listens on locally, and DialerRetries
	// bounds the connection pool's own dial attempts, so this fails fast rather
	// than hanging.
	client := redis.NewClient(&redis.Options{
		Addr:          "127.0.0.1:1",
		DialTimeout:   200 * time.Millisecond,
		DialerRetries: 1,
	})
	defer func() { _ = client.Close() }()

	th := NewRedisThrottler(client)

	_, err := th.Allow(context.Background(), "any-key", 1, time.Minute)
	if err == nil {
		t.Fatal("Allow: want a non-nil error when Redis is unreachable")
	}
}

func TestRedisThrottler_ReportsBudgetFromTheBackend(t *testing.T) {
	// The middleware tests prove the headers are written; this proves the numbers
	// in them are the backend's own. Remaining and ResetAfter come out of the same
	// Lua script that decides Allowed, so a caller's reported budget cannot drift
	// from the verdict it accompanies — which is the whole reason Allow returns a
	// Decision rather than a bare bool.
	th := newTestRedisThrottler(t)

	first, err := th.Allow(context.Background(), "budget", 3, time.Minute)
	if err != nil {
		t.Fatalf("first Allow: %v", err)
	}
	if !first.Allowed {
		t.Fatal("first request should be allowed")
	}
	if first.Limit != 3 {
		t.Errorf("Limit = %d, want 3", first.Limit)
	}
	if first.Remaining != 2 {
		t.Errorf("Remaining after 1 of 3 = %d, want 2", first.Remaining)
	}
	if first.ResetAfter <= 0 {
		t.Errorf("ResetAfter = %v, want > 0 once the bucket is partly drained", first.ResetAfter)
	}

	second, err := th.Allow(context.Background(), "budget", 3, time.Minute)
	if err != nil {
		t.Fatalf("second Allow: %v", err)
	}
	if second.Remaining >= first.Remaining {
		t.Errorf("Remaining did not decrease: %d then %d", first.Remaining, second.Remaining)
	}
}
