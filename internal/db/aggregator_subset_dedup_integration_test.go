//go:build integration

// Integration tests for the word-subset match path in the aggregator suppression pass:
// an aggregator title whose words are a subset of an ATS title's is suppressed, gated
// against seniority-only differences and single-word generics. Reuses the helpers from
// aggregator_dedup_integration_test.go.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"testing"
)

func TestSuppressAggregator_MiddleWordDropMatchesBySubset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// No " - " separator (so ntitle2 does not catch it); the ATS keeps words the
	// aggregator dropped. Only the word-subset path can match this.
	mustUpsert(t, q, atsJob("acme:ats", "Guest Service Agent Front Office", []string{"AE"}))
	mustUpsert(t, q, aggJob("acme:agg", "Guest Service Agent", []string{"AE"}))

	suppressAggregators(t, q)

	atsID, atsDup := dupOf(t, pool, "acme:ats")
	if atsDup != -1 {
		t.Errorf("ATS row duplicate_of = %d, want NULL (canonical)", atsDup)
	}
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (word-subset match)", aggDup, atsID)
	}
}

func TestSuppressAggregator_NonSeniorityAddedWordMerges(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Software Engineer Payments", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Software Engineer", []string{"US"}))

	suppressAggregators(t, q)

	atsID, _ := dupOf(t, pool, "acme:ats")
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (non-seniority added word)", aggDup, atsID)
	}
}

func TestSuppressAggregator_SeniorityOnlyDifferenceNotMerged(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// ATS adds only a seniority marker over the aggregator title: distinct grade, not a dup.
	mustUpsert(t, q, atsJob("acme:ats", "Senior Software Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Software Engineer", []string{"US"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL (seniority-only difference must not merge)", aggDup)
	}
}

func TestSuppressAggregator_SingleWordTitleNotMatchedBySubset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// A one-word aggregator title is too generic for the subset path.
	mustUpsert(t, q, atsJob("acme:ats", "Chef De Partie Kitchen", []string{"AE"}))
	mustUpsert(t, q, aggJob("acme:agg", "Chef", []string{"AE"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL (single-word title must not subset-match)", aggDup)
	}
}

func TestSuppressAggregator_SubsetStillCountryGated(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Store Manager Downtown Branch", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Store Manager", []string{"SG"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL (country gate holds for subset path)", aggDup)
	}
}
