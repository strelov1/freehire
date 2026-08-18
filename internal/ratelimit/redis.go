package ratelimit

import (
	"context"
	"time"

	redisrate "github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// RedisThrottler implements Throttler as a GCRA (leaky-bucket) limiter backed
// by Redis, via redis_rate. It reports Redis errors to the caller rather than
// handling them itself — Middleware is the single place that decides to fail
// open, so any Throttler implementation gets that behavior uniformly.
type RedisThrottler struct {
	limiter *redisrate.Limiter
}

// NewRedisThrottler constructs a RedisThrottler using the given Redis client.
func NewRedisThrottler(client *redis.Client) *RedisThrottler {
	return &RedisThrottler{limiter: redisrate.NewLimiter(client)}
}

// Allow implements Throttler.
//
// Every field of the Decision comes from the one round trip redis_rate already
// makes: Remaining and ResetAfter are computed by the same Lua script that
// decides Allowed, so the budget a caller is told about cannot disagree with the
// verdict it accompanies.
func (t *RedisThrottler) Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error) {
	res, err := t.limiter.Allow(ctx, key, redisrate.Limit{Rate: limit, Burst: limit, Period: window})
	if err != nil {
		return Decision{}, err
	}

	decision := Decision{
		Allowed:    res.Allowed > 0,
		Limit:      limit,
		Remaining:  res.Remaining,
		ResetAfter: res.ResetAfter,
	}
	if decision.Allowed {
		return decision, nil
	}
	// A negative RetryAfter means redis_rate could not say when the next request
	// fits; the whole window is the safe over-estimate.
	decision.RetryAfter = res.RetryAfter
	if decision.RetryAfter < 0 {
		decision.RetryAfter = window
	}
	return decision, nil
}
