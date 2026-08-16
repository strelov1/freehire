package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the shared Cache: every process reading it sees the same value, which
// is the point for anything published to users. It carries no transport of its own —
// the caller supplies the client the process already builds.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache constructs a RedisCache over an existing client.
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

// Get implements Cache. A key that is absent or has expired reports a miss; anything
// else — including an unreachable backend — is returned as an error, so the caller can
// log the difference even though it falls back the same way.
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

// Set implements Cache.
func (c *RedisCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	if ttl <= 0 {
		// Redis rejects a zero expiry and treats a negative one as store-then-delete.
		// Neither matches "this entry is already expired", and the round-trip buys
		// nothing, so skip it.
		return nil
	}
	return c.client.Set(ctx, key, val, ttl).Err()
}
