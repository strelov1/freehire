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
func (t *RedisThrottler) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	res, err := t.limiter.Allow(ctx, key, redisrate.Limit{Rate: limit, Burst: limit, Period: window})
	if err != nil {
		return false, 0, err
	}
	if res.Allowed > 0 {
		return true, 0, nil
	}
	retryAfter := res.RetryAfter
	if retryAfter < 0 {
		retryAfter = window
	}
	return false, retryAfter, nil
}
