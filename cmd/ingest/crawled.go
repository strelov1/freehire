package main

import (
	"sort"
	"sync"
)

// crawledSet records, per provider, the distinct company slugs a run actually wrote.
// The post-run stale sweep scopes its closes to these slugs, so a partial run only closes
// jobs of the companies it crawled. Safe for concurrent Save calls.
type crawledSet struct {
	mu sync.Mutex
	m  map[string]map[string]struct{} // provider -> set of company slugs
}

func newCrawledSet() *crawledSet {
	return &crawledSet{m: make(map[string]map[string]struct{})}
}

// record notes that this run wrote a job for (provider, slug). An empty slug is ignored:
// a job with no company can't be safely scope-closed, so it never joins the crawled set.
func (c *crawledSet) record(provider, slug string) {
	if slug == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	set, ok := c.m[provider]
	if !ok {
		set = make(map[string]struct{})
		c.m[provider] = set
	}
	set[slug] = struct{}{}
}

// slugs returns the provider's crawled company slugs, sorted for a deterministic sweep.
func (c *crawledSet) slugs(provider string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	set := c.m[provider]
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
