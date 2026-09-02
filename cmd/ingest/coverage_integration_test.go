//go:build integration

// Integration test for the aggregator coverage gate's WIRING: that newCoverage hands the query
// the parameters it means to and reads the answer back in the caller's vocabulary.
//
// The matrix of what does and does not count as coverage (closed rows, aggregator sources, the
// freshness cutoff, the fold) belongs to the query and is exhausted in
// internal/platform/db/coverage_freshness_integration_test.go. Repeating it here would spend a
// second container proving the same statement twice. What only this level can prove is that
// the three parameters arrive correctly — a swapped or dropped one would leave every case
// above still passing at the db layer while the gate silently answered nothing.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"
)

func TestCoverageAdapterWiring(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	c := newCoverage(pool)

	// company_slug_folded is written beside company_slug by every write path (see
	// internal/platform/db/folded_slug_rule_test.go), so the seeds write it too.
	seed := func(t *testing.T, source, externalID, slug string, lastSeen time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  company_slug_folded, last_seen_at)
			VALUES ($1, $2, 'https://x.test/'||$2, 'Engineer', $2, 'Co', $3,
			        replace($3, '-', ''), $4)`,
			source, externalID, slug, lastSeen)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
	}

	now := time.Now()
	// One row per parameter the adapter is responsible for passing:
	//   freshco    — proves seen_after is a cutoff and not, say, an equality or an upper bound
	//   staleco    — proves seen_after is actually applied (issue #2315 in miniature: an
	//                employer whose only ATS row is from a board that left sources/)
	//   aggonly    — proves the aggregator list reaches the query
	//   cfoinsights— proves the adapter folds before asking and credits the answer back
	seed(t, "greenhouse", "fresh:1", "freshco", now.Add(-time.Hour))
	seed(t, "trakstar", "stale:1", "staleco", now.Add(-coverageFreshness-24*time.Hour))
	seed(t, "himalayas", "agg:1", "aggonly", now)
	seed(t, "lever", "fold:1", "cfoinsights", now.Add(-time.Hour))

	got, err := c.NonAggregatorCompanies(ctx,
		[]string{"freshco", "staleco", "aggonly", "cfo-insights", "nobodyco"},
		[]string{"himalayas", "remoteok"})
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}

	for _, want := range []struct {
		slug    string
		covered bool
		why     string
	}{
		{"freshco", true, "a posting seen an hour ago is coverage"},
		{"staleco", false, "a posting unseen past the window is NOT coverage — issue #2315"},
		{"aggonly", false, "an aggregator's own posting never covers its company"},
		{"cfo-insights", true, "the hyphenated spelling must reach the squashed stored slug"},
		{"nobodyco", false, "a company with no rows is not covered"},
	} {
		if got[want.slug] != want.covered {
			t.Errorf("%s: covered = %v, want %v — %s", want.slug, got[want.slug], want.covered, want.why)
		}
	}
	// The answer is keyed by what was asked, never by the folded form the query speaks.
	if _, leaked := got["cfoinsights"]; leaked {
		t.Error("the answer leaked the folded spelling, which the caller never asked about")
	}
}
