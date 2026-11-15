package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// GetJSON reads key and decodes it into T. It is a free function because methods cannot
// have type parameters.
//
// A miss, an unreadable backend, and an undecodable payload all report found == false
// (recompute). The error is still returned for the last two so a payload written by a
// previous build shows up in logs.
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
