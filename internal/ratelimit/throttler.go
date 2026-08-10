// Package ratelimit provides a single rate-limiting facility shared by every
// rate-limited HTTP route in the API, backed by Redis.
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// allowTimeout bounds how long a single Allow call may take before Middleware
// gives up and fails open. Long enough for a healthy same-DC Redis round-trip,
// short enough that a partitioned backend degrades to "every request logs a
// warning and passes through" instead of stalling requests.
const allowTimeout = 100 * time.Millisecond

// Throttler checks whether a request rate limit has been exceeded for a key.
type Throttler interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

// Middleware creates a Fiber middleware that enforces rate limiting via a
// Throttler. It fails open — logging a warning and allowing the request — on
// any error from Allow, including exceeding the bounded per-call timeout it
// imposes. A nil throttler (no backend configured) also fails open, with no
// warning logged.
func Middleware(throttler Throttler, keyFunc func(c *fiber.Ctx) string, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if throttler == nil {
			return c.Next()
		}

		ctx, cancel := context.WithTimeout(c.Context(), allowTimeout)
		defer cancel()

		key := keyFunc(c)
		allowed, retryAfter, err := throttler.Allow(ctx, key, limit, window)
		if err != nil {
			log.Printf("ratelimit: Allow error for key %q: %v (failing open)", key, err)
			return c.Next()
		}
		if allowed {
			return c.Next()
		}
		// The header is whole seconds (RFC 9110 delta-seconds); floor it so a
		// sub-second retryAfter doesn't round down to "0" and tell a compliant
		// client to retry immediately into another denial.
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		c.Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		return fiber.NewError(fiber.StatusTooManyRequests, "too many requests, please try again later")
	}
}
