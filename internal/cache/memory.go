package cache

import (
	"bytes"
	"context"
	"sync"
	"time"
)

// Memory is an in-process Cache. It exists for two callers: tests, which want a Cache
// without a Redis backend, and a deployment running without Redis at all.
//
// It is deliberately not a general-purpose cache — there is no eviction, so it suits a
// bounded set of long-lived keys (the catalogue-scale snapshot is one key) and not an
// unbounded keyspace. Expired entries are dropped when their key is read.
//
// A Memory is per-process, so two processes holding one do not agree with each other.
// Where that matters — a figure published to users, which must not differ between two
// web processes or reset on deploy — RedisCache is the implementation to use.
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
		// Drop it rather than leaving a dead entry pinning its value. Re-checking
		// under the write lock keeps a concurrent Set from being discarded.
		m.mu.Lock()
		if cur, still := m.entries[key]; still && !m.now().Before(cur.expiresAt) {
			delete(m.entries, key)
		}
		m.mu.Unlock()
		return nil, false, nil
	}

	// Copy out for the same reason Set copies in: RedisCache round-trips through a
	// socket, so its callers cannot reach its stored bytes. Memory must not be the
	// implementation where they can, or code exercised against Memory in tests would
	// behave differently against Redis in production.
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
