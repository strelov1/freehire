//go:build integration

// Integration tests for which title decorations the aggregator suppression pass treats as
// noise. A trailing colon clause is decoration an aggregator appends to an otherwise identical
// title; a parenthetical and a clause after a comma are NOT — they name the team or the
// specialty, and stripping them would merge separate roles at one company. Measured on prod
// before being written down: stripping parentheticals produced 39 wrong pairs out of 55.
// Reuses the helpers from aggregator_dedup_integration_test.go.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"testing"
)

// The shape whatjobs produces constantly: the ATS states the role, the aggregator appends the
// stack after a colon.
func TestSuppressAggregator_TrailingColonClauseIsDecoration(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Senior Software Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Senior Software Engineer: Full-Stack with TypeScript", []string{"US"}))

	suppressAggregators(t, q)

	atsID, atsDup := dupOf(t, pool, "acme:ats")
	if atsDup != -1 {
		t.Errorf("ATS row duplicate_of = %d, want NULL (canonical)", atsDup)
	}
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (trailing colon clause is decoration)", aggDup, atsID)
	}
}

// A colon clause in front of a dash clause must not stop the strip half-way.
func TestSuppressAggregator_ColonAndDashClausesBothStripped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Backend Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Backend Engineer: Go, Kubernetes - Remote", []string{"US"}))

	suppressAggregators(t, q)

	atsID, _ := dupOf(t, pool, "acme:ats")
	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (both clauses stripped)", aggDup, atsID)
	}
}

// A parenthetical names the team. One company runs Backend (Traffic), (Payments), (Identity) and
// (Infrastructure) as separate roles — stripping the parenthetical would collapse them into one.
func TestSuppressAggregator_ParentheticalIsNotDecoration(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Senior Software Engineer, Backend (Payments)", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Senior Software Engineer, Backend (Traffic)", []string{"US"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL: (Traffic) and (Payments) are different teams", aggDup)
	}
}

// A clause after a comma names the specialty.
func TestSuppressAggregator_CommaClauseIsNotDecoration(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Senior Software Engineer, Fullstack", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Senior Software Engineer, Backend", []string{"US"}))

	suppressAggregators(t, q)

	if _, aggDup := dupOf(t, pool, "acme:agg"); aggDup != -1 {
		t.Errorf("aggregator duplicate_of = %d, want NULL: Backend and Fullstack are different roles", aggDup)
	}
}
