package cache

import (
	"bytes"
	"context"
	"sync"
	"time"
)

// Memory is an in-process Cache for tests and for deployments without Redis.
//
// There is no eviction, so it suits a bounded set of long-lived keys, not an unbounded
// keyspace. Expired entries are dropped when their key is read.
//
// It is per-process, so two processes disagree. Use RedisCache for anything published to
// users that must not differ between processes or reset on deploy.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry

	// now is injectable so expiry is testable without sleeping.
	now func() time.Time
}

type memoryEntry struct {
	val       []byte
	expiresAt time.Time
}

// NewMemory returns an empty in-process Cache.
func NewMemory() *Memory {
	return &Memory{entries: make(map[string]memoryEntry), now: time.Now}
}

// Get implements Cache.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}

	if !m.now().Before(entry.expiresAt) {
		// Re-check under the write lock so a concurrent Set is not discarded.
		m.mu.Lock()
		if cur, still := m.entries[key]; still && !m.now().Before(cur.expiresAt) {
			delete(m.entries, key)
		}
		m.mu.Unlock()
		return nil, false, nil
	}

	// Copy out, as Set copies in: callers must not reach stored bytes, as with RedisCache.
	return bytes.Clone(entry.val), true, nil
}

// Set implements Cache.
func (m *Memory) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	// Copy: the caller keeps its slice and may reuse the buffer.
	stored := bytes.Clone(val)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memoryEntry{val: stored, expiresAt: m.now().Add(ttl)}
	return nil
}
