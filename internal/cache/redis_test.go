package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisCache(client), mr
}

func TestRedisCacheRoundTrip(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, found, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: found = false, want true for a key just set")
	}
	if string(got) != "v" {
		t.Errorf("Get = %q, want %q", got, "v")
	}
}

func TestRedisCacheMissingKeyIsNotAnError(t *testing.T) {
	c, _ := newTestRedisCache(t)

	got, found, err := c.Get(context.Background(), "absent")
	if err != nil {
		t.Fatalf("Get on an absent key returned an error: %v — redis.Nil is a miss, not a failure", err)
	}
	if found {
		t.Error("Get: found = true, want false for a key never set")
	}
	if got != nil {
		t.Errorf("Get = %q, want nil on a miss", got)
	}
}

func TestRedisCacheEntryExpires(t *testing.T) {
	c, mr := newTestRedisCache(t)
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	mr.FastForward(61 * time.Second)

	_, found, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get past TTL returned an error: %v — an expired key is a miss", err)
	}
	if found {
		t.Error("entry survived its TTL: found = true")
	}
}

// A caller can only fall back deliberately if it can tell "the backend is unreachable"
// from "this key was never written". Both lead to the same fallback, but only one is
// worth logging.
func TestRedisCacheReportsBackendFailure(t *testing.T) {
	c, mr := newTestRedisCache(t)
	ctx := context.Background()
	mr.Close()

	if _, _, err := c.Get(ctx, "k"); err == nil {
		t.Error("Get against a closed backend returned no error — a caller cannot distinguish it from a miss")
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err == nil {
		t.Error("Set against a closed backend returned no error")
	}
}

func TestRedisCacheNonPositiveTTLStoresNothing(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set with a zero TTL: %v", err)
	}

	// Redis treats SET with a zero expiry as an error and a negative one as "store
	// then immediately delete"; neither is what a caller asking for an expired entry
	// means. It must read back as a miss, not as a value that never expires.
	if _, found, _ := c.Get(ctx, "k"); found {
		t.Error("a non-positive TTL stored a live entry")
	}
}
