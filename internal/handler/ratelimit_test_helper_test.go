package handler

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/ratelimit"
)

// newTestThrottler builds a real RedisThrottler backed by an in-process fake Redis
// (miniredis), so handler tests exercise the actual production throttling path
// without needing Docker.
func newTestThrottler(t *testing.T) ratelimit.Throttler {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return ratelimit.NewRedisThrottler(client)
}
