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
// function of the index and the request, and a published generation never changes, so an
// entry is exact for as long as that generation is current; the first answer computed
// against a newer one drops everything. It exists because counting the broad requests over
// the whole catalogue costs hundreds of milliseconds every time, while the answer changes
// only when a write publishes a generation.
//
// A generation is identified by its store and number, not by the *Index: holding a
// replaced index would keep a whole old catalogue alive until the next request. A store
// numbers its generations upwards, and the server replaces its store only when it had none.
type answerCache struct {
	mu    sync.Mutex
	st    *facetidx.Store
	gen   uint64
	m     map[string][]byte
	bytes int
	max   int // 0 = answerCacheMaxBytes; tests lower it
}

// get returns the cached answer for key under generation gen of st.
func (c *answerCache) get(st *facetidx.Store, gen uint64, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st != st || c.gen != gen {
		return nil, false
	}
	b, ok := c.m[key]
	return b, ok
}

// put stores an answer computed against generation gen of st. An answer for an older
// generation than the one cached is dropped; one for a newer generation starts the cache
// over. An answer bigger than the whole bound is not cached; past the bound the cache starts
// over: simpler than eviction, and a generation is short-lived.
func (c *answerCache) put(st *facetidx.Store, gen uint64, key string, b []byte) {
	limit := c.max
	if limit == 0 {
		limit = answerCacheMaxBytes
	}
	size := len(key) + len(b)
	if size > limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st != st || c.gen != gen || c.m == nil {
		if c.st == st && gen < c.gen {
			return
		}
		c.st, c.gen, c.m, c.bytes = st, gen, map[string][]byte{}, 0
	}
	if _, ok := c.m[key]; ok {
		return // two concurrent misses computed the same answer
	}
	if c.bytes+size > limit {
		c.m, c.bytes = map[string][]byte{}, 0
	}
	c.m[key] = b
	c.bytes += size
}
