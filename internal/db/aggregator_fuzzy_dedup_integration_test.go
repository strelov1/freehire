//go:build integration

// Integration tests for the normalized-key (entity-decode + trailing-separator-strip)
// match path added to the aggregator suppression pass. These cover the title-mangling
// classes the exact key misses: an ATS title with an appended " - <suffix>" and an
// aggregator title carrying an undecoded HTML entity. Reuses the helpers from
// aggregator_dedup_integration_test.go.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"testing"
)

func TestSuppressAggregator_SuffixStrippedTitleMatches(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// ATS appends a department/location suffix the aggregator dropped.
	mustUpsert(t, q, atsJob("acme:ats", "Assistant Director of Sales - Leisure", []string{"AE"}))
	mustUpsert(t, q, aggJob("acme:agg", "Assistant Director of Sales", []string{"AE"}))

	suppressAggregators(t, q)

	atsID, atsDup := dupOf(t, pool, "acme:ats")
	if atsDup != -1 {
		t.Errorf("ATS row duplicate_of = %d, want NULL (canonical)", atsDup)
	}
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (suffix-stripped match)", aggDup, atsID)
	}
}

func TestSuppressAggregator_HtmlEntityTitleMatches(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Aggregator carries an undecoded entity; ATS has the decoded character.
	mustUpsert(t, q, atsJob("acme:ats", "Assistant F&B Marketing Manager", []string{"AE"}))
	mustUpsert(t, q, aggJob("acme:agg", "Assistant F&amp;B Marketing Manager", []string{"AE"}))

	suppressAggregators(t, q)

	atsID, _ := dupOf(t, pool, "acme:ats")
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (entity-decoded match)", aggDup, atsID)
	}
}

func TestSuppressAggregator_DifferentBaseNotMatchedBySuffixStrip(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Stripping the ATS suffix leaves "backend developer", which is NOT the aggregator's
	// "backend engineer" — the normalized key must not merge distinct bases.
	mustUpsert(t, q, atsJob("acme:ats", "Backend Developer - Remote", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Backend Engineer", []string{"US"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL (distinct base must not match)", aggDup)
	}
}

func TestSuppressAggregator_SuffixMatchStillCountryGated(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Same suffix-stripped base but disjoint non-empty countries: still not suppressed.
	mustUpsert(t, q, atsJob("acme:ats", "Store Manager - Downtown", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Store Manager", []string{"SG"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL (country gate holds for normalized key)", aggDup)
	}
}
