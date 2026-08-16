package cache

import (
	"context"
	"testing"
	"time"
)

// The two implementations must not differ in whether a caller can reach into the
// cache's own state. RedisCache serializes over a socket, so its callers physically
// cannot; Memory hands out whatever it was given. If Memory aliases, code that is
// correct against Redis silently corrupts the cache when run against Memory — the worst
// shape of leaky abstraction, because tests use Memory and production uses Redis.
//
// Run against both implementations so neither can drift from the other.
func forEachCache(t *testing.T, fn func(t *testing.T, c Cache)) {
	t.Helper()
	t.Run("Memory", func(t *testing.T) {
		m, _ := newTestMemory(t)
		fn(t, m)
	})
	t.Run("Redis", func(t *testing.T) {
		c, _ := newTestRedisCache(t)
		fn(t, c)
	})
}

func TestCacheDoesNotAliasStoredValue(t *testing.T) {
	forEachCache(t, func(t *testing.T, c Cache) {
		ctx := context.Background()
		val := []byte("original")

		if err := c.Set(ctx, "k", val, time.Minute); err != nil {
			t.Fatalf("Set: %v", err)
		}
		val[0] = 'X' // the caller reuses its buffer, as callers do

		got, _, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(got) != "original" {
			t.Errorf("Get = %q, want %q — mutating the slice passed to Set changed the cached entry", got, "original")
		}
	})
}

func TestCacheDoesNotAliasReturnedValue(t *testing.T) {
	forEachCache(t, func(t *testing.T, c Cache) {
		ctx := context.Background()

		if err := c.Set(ctx, "k", []byte("original"), time.Minute); err != nil {
			t.Fatalf("Set: %v", err)
		}

		first, _, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("first Get: %v", err)
		}
		first[0] = 'X' // a caller decoding in place, or just reusing the slice

		second, _, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("second Get: %v", err)
		}
		if string(second) != "original" {
			t.Errorf("Get = %q, want %q — mutating a returned slice changed the cached entry", second, "original")
		}
	})
}
