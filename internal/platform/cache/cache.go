// Package cache provides the shared best-effort key-value cache for values that are
// expensive to compute, identical for every caller, and tolerable slightly stale.
//
// Nothing here is a source of truth: a caller that cannot read the cache recomputes, and
// no method promises a stored value is still there.
package cache

import (
	"context"
	"time"
)

// Cache stores bytes under a key for a bounded time.
//
// A caller MUST treat both a miss and an error as "no cached value" and fall back to
// its own source of truth. Errors are surfaced so "backend down" can be logged apart
// from "never written".
//
// Implementations are safe for concurrent use and never share storage with the caller:
// mutating a slice passed to Set or returned by Get does not change what is cached.
type Cache interface {
	// Get returns the stored bytes and whether a live entry was found. A missing or
	// expired key returns (nil, false, nil): a miss is an outcome, not a failure.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores val under key, to be forgotten after ttl elapses. A ttl of zero or
	// less stores nothing — a caller asking for an already-expired entry gets its
	// wish, not an error.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
}
