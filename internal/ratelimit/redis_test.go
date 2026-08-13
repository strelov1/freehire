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
		allowed, _, err := th.Allow(context.Background(), "key1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed (within limit 3)", i)
		}
	}
}

func TestRedisThrottler_RejectsOverLimit(t *testing.T) {
	th := newTestRedisThrottler(t)

	for i := 0; i < 2; i++ {
		if _, _, err := th.Allow(context.Background(), "key2", 2, time.Minute); err != nil {
			t.Fatalf("Allow %d: %v", i, err)
		}
	}

	allowed, retryAfter, err := th.Allow(context.Background(), "key2", 2, time.Minute)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allowed {
		t.Fatal("3rd request should be rejected (limit 2)")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %v, want > 0", retryAfter)
	}
}

func TestRedisThrottler_SeparateKeysAreIndependent(t *testing.T) {
	th := newTestRedisThrottler(t)

	if _, _, err := th.Allow(context.Background(), "a", 1, time.Minute); err != nil {
		t.Fatalf("Allow a: %v", err)
	}
	allowedA, _, _ := th.Allow(context.Background(), "a", 1, time.Minute)
	if allowedA {
		t.Fatal("key a should be exhausted after 1 request (limit 1)")
	}

	allowedB, _, err := th.Allow(context.Background(), "b", 1, time.Minute)
	if err != nil {
		t.Fatalf("Allow b: %v", err)
	}
	if !allowedB {
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

	_, _, err := th.Allow(context.Background(), "any-key", 1, time.Minute)
	if err == nil {
		t.Fatal("Allow: want a non-nil error when Redis is unreachable")
	}
}
