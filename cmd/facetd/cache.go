package main

import (
	"sync"

	"github.com/strelov1/freehire/internal/search/facetidx"
)

// answerCacheMaxBytes bounds the encoded answers one index generation keeps. A generation
// lives minutes (the drain writes in batches), and the hot requests (the unfiltered /jobs
// page, broad multi-value filters) are few, so the bound only matters for a flood of
// one-off filters (a posted_ts cut-off that moves every second).
const answerCacheMaxBytes = 256 << 20

// answerCache holds encoded /facets answers for ONE index generation. An answer is a pure
// function of the index and the request, and a published Index never changes, so an entry
// is exact for as long as that *Index is current; the first lookup against a newer one
// drops everything. It exists because counting the broad requests over the whole
// catalogue costs hundreds of milliseconds every time, while the answer changes only when
// a write publishes a generation.
type answerCache struct {
	mu    sync.Mutex
	ix    *facetidx.Index
	m     map[string][]byte
	bytes int
}

// get returns the cached answer for key under ix.
func (c *answerCache) get(ix *facetidx.Index, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ix != ix {
		return nil, false
	}
	b, ok := c.m[key]
	return b, ok
}

// put stores an answer computed against ix. An answer for an older generation than the one
// cached is dropped; one for a newer generation starts the cache over. Past the byte bound
// the cache starts over too: simpler than eviction, and a generation is short-lived.
func (c *answerCache) put(ix *facetidx.Index, key string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ix != ix {
		if c.ix != nil && ix.Generation() < c.ix.Generation() {
			return
		}
		c.ix, c.m, c.bytes = ix, map[string][]byte{}, 0
	}
	size := len(key) + len(b)
	if c.bytes+size > answerCacheMaxBytes {
		c.m, c.bytes = map[string][]byte{}, 0
	}
	c.m[key] = b
	c.bytes += size
}
