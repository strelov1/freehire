package ratelimit

import (
	"context"
	"errors"
	"net/http"
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

func (m *memoryThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[key] >= limit {
		return Decision{Limit: limit, Remaining: 0, ResetAfter: window, RetryAfter: window}, nil
	}
	m.counts[key]++
	return Decision{
		Allowed:    true,
		Limit:      limit,
		Remaining:  limit - m.counts[key],
		ResetAfter: window,
	}, nil
}

// erroringThrottler always returns an error, simulating a backend failure.
type erroringThrottler struct{}

func (erroringThrottler) Allow(context.Context, string, int, time.Duration) (Decision, error) {
	return Decision{}, errors.New("backend unreachable")
}

// hangingThrottler blocks until the context passed to Allow is done, then
// reports the context error — simulating a backend that never responds in time.
type hangingThrottler struct{}

func (hangingThrottler) Allow(ctx context.Context, _ string, _ int, _ time.Duration) (Decision, error) {
	<-ctx.Done()
	return Decision{}, ctx.Err()
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

// fractionalRetryThrottler reports a retryAfter with a fraction, to pin that the
// header rounds UP. Truncating 1.7s to "1" tells a compliant client to retry a
// full second before its budget exists, straight into the denial it just got.
type fractionalRetryThrottler struct{}

func (fractionalRetryThrottler) Allow(_ context.Context, _ string, limit int, window time.Duration) (Decision, error) {
	return Decision{Limit: limit, ResetAfter: window, RetryAfter: 1700 * time.Millisecond}, nil
}

func TestMiddleware_RoundsRetryAfterUp(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", Middleware(fractionalRetryThrottler{}, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
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
	if got := resp.Header.Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After for 1.7s = %q, want %q", got, "2")
	}
}

type subSecondRetryThrottler struct{}

func (subSecondRetryThrottler) Allow(_ context.Context, _ string, limit int, window time.Duration) (Decision, error) {
	return Decision{Limit: limit, ResetAfter: window, RetryAfter: 200 * time.Millisecond}, nil
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

func TestMiddleware_SetsBudgetHeadersOnAllowedRequest(t *testing.T) {
	// The headers exist so a ceiling can be respected rather than merely
	// discovered: a client reading Remaining can slow down before it is refused,
	// whereas one that only ever sees Retry-After has already failed a request.
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
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-RateLimit-Limit"); got != "3" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", got, "3")
	}
	if got := resp.Header.Get("X-RateLimit-Remaining"); got != "2" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", got, "2")
	}
	if resp.Header.Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset on an allowed response")
	}
}

func TestMiddleware_RemainingDecreasesAcrossRequests(t *testing.T) {
	th := newMemoryThrottler()
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 5, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	remaining := func() string {
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.Header.Get("X-RateLimit-Remaining")
	}

	if first, second := remaining(), remaining(); first != "4" || second != "3" {
		t.Errorf("Remaining across two requests = %q, %q; want %q, %q", first, second, "4", "3")
	}
}

func TestMiddleware_SetsBudgetHeadersOnRejection(t *testing.T) {
	th := newMemoryThrottler()
	app := fiber.New()
	app.Get("/probe", Middleware(th, KeyByIP("test"), 1, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	first, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	first.Body.Close()

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil))
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Error("Retry-After must survive alongside the new headers")
	}
	if got := resp.Header.Get("X-RateLimit-Limit"); got != "1" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", got, "1")
	}
	if got := resp.Header.Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", got, "0")
	}
	if resp.Header.Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset on a 429 response")
	}
}

func TestMiddleware_FailOpenReportsNoBudget(t *testing.T) {
	// No check happened, so there is no budget to report. Inventing one would be
	// worse than silence: a client would pace itself against a fabricated number.
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
		t.Fatalf("status = %d, want 200 (fail open)", resp.StatusCode)
	}
	for _, h := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if got := resp.Header.Get(h); got != "" {
			t.Errorf("%s = %q on a fail-open response, want it absent", h, got)
		}
	}
}

// peerApp builds a test app whose c.IP() reads X-Real-IP, which is exactly how
// c.IP() resolves in production: cmd/server sets ProxyHeader to X-Real-IP and
// nginx populates it. Fiber's app.Test always reports a 0.0.0.0 remote address
// and ignores httptest's RemoteAddr, so this is the only way to drive the peer
// a middleware sees.
func peerApp(th Throttler, limit int) *fiber.App {
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/probe", Middleware(th, KeyByIP("test"), limit, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func getAsPeer(t *testing.T, app *fiber.App, peer string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, peer)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request as %s: %v", peer, err)
	}
	return resp
}

func TestMiddleware_TrustedPeerIsNeverLimited(t *testing.T) {
	// SSR reaches the API over loopback without a client-address header, so every
	// server-rendered page presents the same peer. Counting those would put the
	// whole site in one caller's budget and throttle it as a single abusive
	// client — the limiter exists to bound external abuse, and our own front end
	// flooding the API is a different bug that a 429 makes worse, not better.
	app := peerApp(newMemoryThrottler(), 1)

	for i := range 5 {
		resp := getAsPeer(t, app, "127.0.0.1")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d status = %d, want 200 (loopback is never limited, limit is 1)", i, resp.StatusCode)
		}
		if got := resp.Header.Get("X-RateLimit-Limit"); got != "" {
			t.Errorf("request %d: X-RateLimit-Limit = %q, want it absent — no check happened", i, got)
		}
		resp.Body.Close()
	}
}

func TestMiddleware_PrivateNetworkPeerIsNeverLimited(t *testing.T) {
	app := peerApp(newMemoryThrottler(), 1)

	// Three requests each against a limit of 1: one apiece would pass even
	// unexempted, since every peer is its own key.
	for _, peer := range []string{"10.1.2.3", "172.16.9.9", "192.168.1.10"} {
		for i := range 3 {
			resp := getAsPeer(t, app, peer)
			if resp.StatusCode != fiber.StatusOK {
				t.Errorf("peer %s request %d status = %d, want 200 (private peer is never limited)", peer, i, resp.StatusCode)
			}
			resp.Body.Close()
		}
	}
}

func TestMiddleware_ExternalPeerIsStillLimited(t *testing.T) {
	// The companion to the exemption: it must not become a blanket bypass.
	app := peerApp(newMemoryThrottler(), 1)

	first := getAsPeer(t, app, "203.0.113.7")
	first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first request status = %d, want 200", first.StatusCode)
	}

	second := getAsPeer(t, app, "203.0.113.7")
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429 — an external caller is still limited", second.StatusCode)
	}
}

func ignoringTrustedPeersApp(th Throttler, limit int) *fiber.App {
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/probe", MiddlewareIgnoringTrustedPeers(th, KeyByIP("test"), limit, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

// TestMiddlewareIgnoringTrustedPeers_LoopbackIsStillLimited guards the compensating
// control internal/api/handler's autoApplyOrchestratorGate depends on:
// ratelimit.Middleware's ordinary trusted-peer exemption would silently exempt 127.0.0.1
// — exactly the address the orchestrator calls hire from in production — voiding a shared
// secret's own rate limit entirely.
func TestMiddlewareIgnoringTrustedPeers_LoopbackIsStillLimited(t *testing.T) {
	app := ignoringTrustedPeersApp(newMemoryThrottler(), 1)

	first := getAsPeer(t, app, "127.0.0.1")
	first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first request status = %d, want 200", first.StatusCode)
	}

	second := getAsPeer(t, app, "127.0.0.1")
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429 — loopback must not be exempt here", second.StatusCode)
	}
}
