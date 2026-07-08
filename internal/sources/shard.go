package sources

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// ParseShard parses a "i/n" shard spec: crawl shard i of n (both 1-based counts).
// It validates 1 <= i <= n and n >= 1, so a malformed or out-of-range spec fails
// fast at the worker rather than silently crawling the wrong slice.
func ParseShard(s string) (i, n int, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("sources: shard %q: want the form i/n", s)
	}
	i, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("sources: shard %q: %w", s, err)
	}
	n, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("sources: shard %q: %w", s, err)
	}
	if n < 1 || i < 1 || i > n {
		return 0, 0, fmt.Errorf("sources: shard %q: need 1 <= i <= n and n >= 1", s)
	}
	return i, n, nil
}

// Shard returns a copy of the config holding only the boards belonging to shard i of
// n — spreading a single provider's huge board list (e.g. workday) across several
// staggered runs so each finishes within its timeout. Distinct companies (keyed by
// their normalized company_slug, in first-appearance order) are assigned round-robin
// to shards, and ALL of a company's boards go to its shard. Grouping by company — not
// by raw board index — is required for the stale-job sweep: the sweep scopes closes by
// company_slug, so a company split across shards could have one shard close the
// still-live boards another shard owns. i is 1-based (1..n); n <= 1 returns the config
// unchanged. The file's default provider is preserved.
func (c Config) Shard(i, n int) Config {
	if n <= 1 {
		return c
	}
	shardOf := make(map[string]int)
	next := 0
	var picked []CompanyEntry
	for _, e := range c.Sources {
		slug := normalize.Slug(e.Company)
		s, ok := shardOf[slug]
		if !ok {
			s = next % n
			shardOf[slug] = s
			next++
		}
		if s == i-1 {
			picked = append(picked, e)
		}
	}
	return Config{Provider: c.Provider, Sources: picked}
}
