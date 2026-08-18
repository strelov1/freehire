package handler

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"

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

	t.Run("cheap budget admits exactly publicReadsPerMinute", func(t *testing.T) {
		const peer = "203.0.113.10"
		for i := 1; i <= publicReadsPerMinute; i++ {
			code, rem, _ := hit("/cheap", peer)
			if code != fiber.StatusOK {
				t.Fatalf("request %d/%d = %d, want 200 (remaining=%s)", i, publicReadsPerMinute, code, rem)
			}
			// Remaining is a GCRA reading, not limit-minus-count: redis_rate is a
			// leaky bucket, so the figure trails naive counting by up to one while
			// the elapsed time has not yet credited a whole token back. Assert the
			// band it must stay inside rather than an exact value that encodes the
			// arithmetic of a particular backend.
			if i == 1 {
				n, err := strconv.Atoi(rem)
				if err != nil || n < publicReadsPerMinute-2 || n > publicReadsPerMinute-1 {
					t.Errorf("first Remaining = %q, want %d or %d", rem, publicReadsPerMinute-2, publicReadsPerMinute-1)
				}
			}
		}
		code, rem, retry := hit("/cheap", peer)
		if code != fiber.StatusTooManyRequests {
			t.Fatalf("request %d = %d, want 429", publicReadsPerMinute+1, code)
		}
		if rem != "0" {
			t.Errorf("Remaining on refusal = %q, want 0", rem)
		}
		if retry == "" || retry == "0" {
			t.Errorf("Retry-After = %q, want a positive whole second", retry)
		}
		t.Logf("admitted %d, refused the next with Retry-After=%ss", publicReadsPerMinute, retry)
	})

	t.Run("agent budget is separate and smaller", func(t *testing.T) {
		const peer = "203.0.113.11"
		for i := 1; i <= agentSearchPerMinute; i++ {
			if code, rem, _ := hit("/agent", peer); code != fiber.StatusOK {
				t.Fatalf("agent request %d = %d (remaining=%s)", i, code, rem)
			}
		}
		if code, _, _ := hit("/agent", peer); code != fiber.StatusTooManyRequests {
			t.Fatalf("agent request %d = %d, want 429", agentSearchPerMinute+1, code)
		}
		if code, _, _ := hit("/cheap", peer); code != fiber.StatusOK {
			t.Errorf("/cheap after exhausting /agent = %d, want 200", code)
		}
		t.Logf("agent admitted %d then refused; cheap budget untouched", agentSearchPerMinute)
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
