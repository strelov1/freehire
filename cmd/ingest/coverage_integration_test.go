//go:build integration

// Integration test for the aggregator coverage gate's adapter: the real query, through the
// real constructor, against a real Postgres. coverage_test.go covers the fold-and-credit
// mapping with a fake; this covers the part only the database can answer — that the freshness
// window and the aggregator exclusion actually decide the answer the pipeline acts on.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"
)

func TestCoverageLookupAgainstPostgres(t *testing.T) {
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
	seed(t, "greenhouse", "fresh:1", "freshco", now.Add(-time.Hour))
	// The reported defect in miniature: an employer whose only ATS row is from a board that
	// left sources/ and was last seen a month ago.
	seed(t, "trakstar", "stale:1", "staleco", now.Add(-31*24*time.Hour))
	// Hyphenated on the aggregator's side, squashed on the ATS's — one employer.
	seed(t, "lever", "fold:1", "cfoinsights", now.Add(-time.Hour))

	got, err := c.NonAggregatorCompanies(ctx,
		[]string{"freshco", "staleco", "cfo-insights", "nobodyco"},
		[]string{"himalayas", "remoteok"})
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}

	if !got["freshco"] {
		t.Error("freshco: a posting seen an hour ago must count as coverage")
	}
	if got["staleco"] {
		t.Error("staleco: a posting unseen for 31 days must NOT count as coverage — this is issue #2315")
	}
	if !got["cfo-insights"] {
		t.Error("cfo-insights: the hyphenated spelling must reach the squashed stored slug")
	}
	if got["nobodyco"] {
		t.Error("nobodyco: a company with no rows must not be reported as covered")
	}
}

func TestCoverageLookupIgnoresAggregatorPostings(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	c := newCoverage(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
		                  company_slug_folded, last_seen_at)
		VALUES ('himalayas', 'agg:1', 'https://x.test/agg', 'Engineer', 'agg-1', 'Co', 'aggonly',
		        'aggonly', now())`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := c.NonAggregatorCompanies(ctx, []string{"aggonly"}, []string{"himalayas"})
	if err != nil {
		t.Fatalf("NonAggregatorCompanies: %v", err)
	}
	if got["aggonly"] {
		t.Error("an aggregator's own posting must never make its company covered")
	}
}
