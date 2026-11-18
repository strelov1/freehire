package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the shared Cache: every process sees the same value. The caller supplies
// the redis client.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache constructs a RedisCache over an existing client.
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

// Get implements Cache. An absent or expired key is a miss; anything else, including an
// unreachable backend, is an error.
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
		return nil
	}
	return c.client.Set(ctx, key, val, ttl).Err()
}
