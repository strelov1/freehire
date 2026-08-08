package auth

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Throttler checks whether a request rate limit has been exceeded for a key.
type Throttler interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

// PGThrottler implements a sliding window rate limiter using the rate_limits PostgreSQL table.
type PGThrottler struct {
	pool *pgxpool.Pool
}

// NewPGThrottler constructs a PGThrottler with the given pgxpool.
func NewPGThrottler(pool *pgxpool.Pool) *PGThrottler {
	return &PGThrottler{pool: pool}
}

// Allow checks if the given key is within the rate limit over the specified duration window.
// If the pool is nil or a database error occurs, it logs a warning and fails open.
func (t *PGThrottler) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if t == nil || t.pool == nil {
		return true, 0, nil
	}

	now := time.Now().UTC()
	cutoff := now.Add(-window)

	// Clean up old entries outside the window
	_, _ = t.pool.Exec(ctx, "DELETE FROM rate_limits WHERE key = $1 AND window_start < $2", key, cutoff)

	// Count requests within window
	var count int
	var oldest time.Time
	err := t.pool.QueryRow(ctx, "SELECT COALESCE(SUM(request_count), 0), COALESCE(MIN(window_start), $2) FROM rate_limits WHERE key = $1 AND window_start >= $2", key, cutoff).Scan(&count, &oldest)
	if err != nil {
		log.Printf("throttler: DB query error for key %q: %v (failing open)", key, err)
		return true, 0, nil // Fail open
	}

	if count >= limit {
		retryAfter := window - now.Sub(oldest)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter, nil
	}

	// Insert or increment current window request count
	windowStart := now.Truncate(time.Second)
	_, err = t.pool.Exec(ctx, `
		INSERT INTO rate_limits (key, window_start, request_count)
		VALUES ($1, $2, 1)
		ON CONFLICT (key, window_start)
		DO UPDATE SET request_count = rate_limits.request_count + 1
	`, key, windowStart)
	if err != nil {
		log.Printf("throttler: DB insert error for key %q: %v (failing open)", key, err)
		return true, 0, nil // Fail open
	}

	return true, 0, nil
}

// ThrottleMiddleware creates a Fiber middleware that enforces rate limiting via a Throttler.
func ThrottleMiddleware(throttler Throttler, keyFunc func(c *fiber.Ctx) string, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if throttler == nil {
			return c.Next()
		}
		key := keyFunc(c)
		allowed, retryAfter, err := throttler.Allow(c.Context(), key, limit, window)
		if err != nil || allowed {
			return c.Next()
		}
		c.Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		return fiber.NewError(fiber.StatusTooManyRequests, "too many requests, please try again later")
	}
}
