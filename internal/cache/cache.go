// Package cache provides the shared best-effort key-value cache used for values that
// are expensive to compute, identical for every caller, and tolerable slightly stale.
//
// Nothing here is a source of truth. Every value in a Cache can also be obtained (or
// approximated) some other way, and a caller that cannot read the cache is expected to
// do exactly that — which is why the interface reports errors rather than swallowing
// them, and why no method promises the value it stored is still there.
package cache

import (
	"context"
	"time"
)

// Cache stores bytes under a key for a bounded time.
//
// A caller MUST treat both a miss and an error as "no cached value" and fall back to
// its own source of truth. The interface surfaces the error instead of hiding it so
// that decision stays with the caller — the same split ratelimit.Throttler uses, where
// the implementation reports and one caller decides to fail open. An implementation
// that swallowed errors would make "the backend is down" indistinguishable from "this
// key was never written", and those want different logging even though they want the
// same fallback.
//
// Implementations are safe for concurrent use, and never share storage with the caller:
// mutating a slice passed to Set, or one returned by Get, does not change what is
// cached. RedisCache gets that for free by round-tripping through a socket, so Memory
// copies to match — otherwise code exercised against Memory in tests would behave
// differently against Redis in production.
type Cache interface {
	// Get returns the stored bytes and whether a live entry was found. A missing or
	// expired key returns (nil, false, nil): a miss is an outcome, not a failure.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores val under key, to be forgotten after ttl elapses. A ttl of zero or
	// less stores nothing — a caller asking for an already-expired entry gets its
	// wish, not an error.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
}
