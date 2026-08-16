package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// GetJSON reads key and decodes it into T. It is a free function rather than a method
// because Go does not permit type parameters on methods.
//
// A miss, an unreadable backend, and a payload that no longer decodes into T all report
// found == false, because all three mean the same thing to a caller: there is no usable
// cached value, recompute. The error is still returned for the last two, so a stale
// incompatible payload — typically a deploy that changed T while entries written by the
// previous build are still live — is visible in logs rather than looking like ordinary
// cache churn.
func GetJSON[T any](ctx context.Context, c Cache, key string) (T, bool, error) {
	var zero T

	raw, found, err := c.Get(ctx, key)
	if err != nil || !found {
		return zero, false, err
	}

	var val T
	if err := json.Unmarshal(raw, &val); err != nil {
		return zero, false, fmt.Errorf("cache: decoding %q: %w", key, err)
	}
	return val, true, nil
}

// SetJSON encodes val as JSON and stores it under key for ttl.
func SetJSON[T any](ctx context.Context, c Cache, key string, val T, ttl time.Duration) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache: encoding %q: %w", key, err)
	}
	return c.Set(ctx, key, raw, ttl)
}
