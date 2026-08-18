package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/ratelimit"
)

// TestLimiterE2E drives the real constants through the real Redis-backed
// throttler, which the unit tests deliberately do not: they use fakes, so they
// prove the wiring and not the arithmetic.
func TestLimiterE2E(t *testing.T) {
	mr := miniredis.RunT(t)
	// Freeze the clock. redis_rate is GCRA: at 600/min a token leaks back every
	// 100ms, so a loop of 600 requests that takes longer than that on a slow
	// machine gets extra headroom and the "601st is refused" assertion becomes a
	// race against the runner. CI caught exactly that. A frozen clock makes the
	// budget arithmetic the only variable, which is what these cases are about.
	mr.SetTime(time.Unix(1755000000, 0))
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	th := ratelimit.NewRedisThrottler(client)

	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/cheap", publicReadLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/agent", agentSearchLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	hit := func(path, peer string) (int, string, string) {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, peer)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("X-RateLimit-Remaining"), resp.Header.Get("Retry-After")
	}

	// GCRA's instantaneous capacity is limit-1, not limit: the emission-interval
	// arithmetic reserves one slot. So these cases assert the honest contract —
	// about `limit` requests get through and the budget then refuses — rather than
	// an exact count that encodes one backend's off-by-one and breaks the day we
	// change limiters.
	admitUntilRefused := func(t *testing.T, path, peer string, ceiling int) int {
		t.Helper()
		for i := 1; i <= ceiling+1; i++ {
			code, rem, retry := hit(path, peer)
			if code == fiber.StatusTooManyRequests {
				if rem != "0" {
					t.Errorf("Remaining on refusal = %q, want 0", rem)
				}
				if retry == "" || retry == "0" {
					t.Errorf("Retry-After = %q, want a positive whole second", retry)
				}
				return i - 1
			}
			if code != fiber.StatusOK {
				t.Fatalf("request %d = %d, want 200 or 429", i, code)
			}
		}
		t.Fatalf("%s never refused within %d requests against a ceiling of %d", path, ceiling+1, ceiling)
		return 0
	}

	t.Run("cheap budget admits about publicReadsPerMinute then refuses", func(t *testing.T) {
		got := admitUntilRefused(t, "/cheap", "203.0.113.10", publicReadsPerMinute)
		if got < publicReadsPerMinute-1 || got > publicReadsPerMinute {
			t.Errorf("admitted %d, want %d or %d", got, publicReadsPerMinute-1, publicReadsPerMinute)
		}
		t.Logf("admitted %d of a %d ceiling, then refused", got, publicReadsPerMinute)
	})

	t.Run("agent budget is separate and smaller", func(t *testing.T) {
		const peer = "203.0.113.11"
		got := admitUntilRefused(t, "/agent", peer, agentSearchPerMinute)
		if got < agentSearchPerMinute-1 || got > agentSearchPerMinute {
			t.Errorf("agent admitted %d, want %d or %d", got, agentSearchPerMinute-1, agentSearchPerMinute)
		}
		if code, _, _ := hit("/cheap", peer); code != fiber.StatusOK {
			t.Errorf("/cheap after exhausting /agent = %d, want 200", code)
		}
		t.Logf("agent admitted %d then refused; cheap budget untouched", got)
	})

	t.Run("the live peak of 184 rpm never trips the agent budget", func(t *testing.T) {
		const peer = "203.0.113.12"
		for i := 1; i <= 184; i++ {
			if code, _, _ := hit("/agent", peer); code != fiber.StatusOK {
				t.Fatalf("ManyApplyAssist's measured peak was refused at request %d", i)
			}
		}
		t.Log("184 rpm — today's heaviest live client — passes untouched")
	})

	t.Run("loopback is exempt at ten times the ceiling", func(t *testing.T) {
		for i := 1; i <= agentSearchPerMinute*10; i++ {
			if code, _, _ := hit("/agent", "127.0.0.1"); code != fiber.StatusOK {
				t.Fatalf("loopback refused at request %d — SSR would be throttled", i)
			}
		}
		t.Logf("%d loopback requests, none refused", agentSearchPerMinute*10)
	})
}
