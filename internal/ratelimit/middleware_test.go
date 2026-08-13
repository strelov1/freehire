package ratelimit

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// memoryThrottler is a minimal in-memory Throttler double for testing Middleware
// in isolation from any real backend.
type memoryThrottler struct {
	mu     sync.Mutex
	counts map[string]int
}

func newMemoryThrottler() *memoryThrottler {
	return &memoryThrottler{counts: make(map[string]int)}
}

func (m *memoryThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[key] >= limit {
		return false, window, nil
	}
	m.counts[key]++
	return true, 0, nil
}

// erroringThrottler always returns an error, simulating a backend failure.
type erroringThrottler struct{}

func (erroringThrottler) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, errors.New("backend unreachable")
}

// hangingThrottler blocks until the context passed to Allow is done, then
// reports the context error — simulating a backend that never responds in time.
type hangingThrottler struct{}

func (hangingThrottler) Allow(ctx context.Context, _ string, _ int, _ time.Duration) (bool, time.Duration, error) {
	<-ctx.Done()
	return false, 0, ctx.Err()
}

func TestMiddleware_AllowsUnderLimit(t *testing.T) {
	th := newMemoryThrottler()
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 3, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestMiddleware_NilThrottlerPassesThrough(t *testing.T) {
	// A nil Throttler means "no backend configured" (e.g. a test harness that
	// doesn't care about rate limiting) — Middleware treats that as fail-open,
	// the same posture as an unreachable backend, rather than panicking.
	app := fiber.New()
	app.Get("/probe", Middleware(nil, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 (nil throttler fails open)", resp.StatusCode)
	}
}

func TestMiddleware_BlocksOverLimitWithRetryAfter(t *testing.T) {
	th := newMemoryThrottler()
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	first, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	defer first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first request status = %d, want 200", first.StatusCode)
	}

	second, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429", second.StatusCode)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestMiddleware_FailsOpenOnThrottlerError(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", Middleware(erroringThrottler{}, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 (fail open)", resp.StatusCode)
	}
}

func TestMiddleware_FailsOpenWhenThrottlerExceedsBoundedTimeout(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", Middleware(hangingThrottler{}, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	start := time.Now()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil), 1000)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 (fail open)", resp.StatusCode)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("request took %v, want it bounded well under the request's own deadline (Allow should be cut off around ~100ms)", elapsed)
	}
}

func TestMiddleware_FloorsRetryAfterToAtLeastOneSecond(t *testing.T) {
	// A Throttler reporting a sub-second retryAfter must still yield a whole-second
	// Retry-After header — "0" would tell a compliant client to retry immediately
	// into another denial.
	th := subSecondRetryThrottler{}
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want %q", got, "1")
	}
}

type subSecondRetryThrottler struct{}

func (subSecondRetryThrottler) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 200 * time.Millisecond, nil
}

// TestMiddleware_WithRedisThrottler_FailsOpenWhenRedisUnreachable exercises the
// real production path end-to-end: RedisThrottler propagates a genuine backend
// error, and Middleware — not RedisThrottler — is what decides to fail open.
func TestMiddleware_WithRedisThrottler_FailsOpenWhenRedisUnreachable(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:          "127.0.0.1:1",
		DialTimeout:   200 * time.Millisecond,
		DialerRetries: 1,
	})
	defer func() { _ = client.Close() }()

	th := NewRedisThrottler(client)
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil), 1000)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 (fail open through the real RedisThrottler + Middleware pair)", resp.StatusCode)
	}
}
