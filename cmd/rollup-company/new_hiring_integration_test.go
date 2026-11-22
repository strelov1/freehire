//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// TestHiringStartedAtGates seeds one company per gate of hiring_started_at and checks that
// only the company passing all of them is marked as having just started hiring.
func TestHiringStartedAtGates(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	aggregator := sources.TaxonomyAggregatorProviders()[0]

	for _, b := range []struct {
		provider, board string
		seenDaysAgo     int
	}{
		{"ats", "fresh", 60}, {"ats", "late", 5}, {"ats", "busy", 200}, {"portal", "region-1", 200}, {aggregator, "feed", 200},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO board_health (provider, board, first_seen_at) VALUES ($1, $2, now() - make_interval(days => $3))`, b.provider, b.board, b.seenDaysAgo); err != nil {
			t.Fatal(err)
		}
	}
	for _, j := range []struct {
		source, externalID, company string
		createdDaysAgo              int
		closedDaysAgo               *int
	}{
		{"ats", "fresh:1", "fresh", 5, nil},     // passes every gate
		{"ats", "fresh:2", "fresh", 3, nil},     // later posting: the start stays at the first
		{"ats", "late:1", "late", 5, nil},       // board first crawled the same day: an artifact
		{"ats", "busy:1", "busy", 100, ptr(50)}, // open during the quiet window before
		{"ats", "busy:2", "busy", 5, nil},
		{"portal", "region-1:1", "employer-a", 5, nil}, // a portal board lists two companies
		{"portal", "region-1:2", "employer-b", 5, nil},
		{aggregator, "feed:1", "aggregated", 5, nil},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO jobs (public_slug, source, external_id, title, url, company, company_slug, created_at, closed_at)
			VALUES ($1, $2, $3, 'Engineer', 'https://example.com/' || $1, $4, $4, now() - make_interval(days => $5),
				CASE WHEN $6::int IS NULL THEN NULL ELSE now() - make_interval(days => $6::int) END)`,
			j.source+"-"+j.externalID, j.source, j.externalID, j.company, j.createdDaysAgo, j.closedDaysAgo); err != nil {
			t.Fatal(err)
		}
	}

	// providerSpikeStarters companies on one provider all "starting" the same day: an
	// adapter change on our side, not a hiring wave.
	if _, err := pool.Exec(ctx, `INSERT INTO board_health (provider, board, first_seen_at)
		SELECT 'bulk', 'b' || g, now() - interval '60 days' FROM generate_series(1, $1::int) g`, providerSpikeStarters); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (public_slug, source, external_id, title, url, company, company_slug, created_at)
		SELECT 'bulk-' || g, 'bulk', 'b' || g || ':1', 'Engineer', 'https://example.com/bulk-' || g, 'bulk-' || g, 'bulk-' || g, now() - interval '4 days'
		FROM generate_series(1, $1::int) g`, providerSpikeStarters); err != nil {
		t.Fatal(err)
	}

	if err := rebuildCompanyRollups(ctx, pool, pool); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT company_slug, hiring_started_at IS NOT NULL,
		coalesce(round(extract(epoch FROM now() - hiring_started_at) / 86400)::int, 0)
		FROM insights_company_growth`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for rows.Next() {
		var slug string
		var started bool
		var daysAgo int
		if err := rows.Scan(&slug, &started, &daysAgo); err != nil {
			t.Fatal(err)
		}
		if started {
			got[slug] = daysAgo
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["fresh"] != 5 {
		t.Fatalf("companies marked as just started hiring (slug → days ago) = %v, want only fresh → 5", got)
	}
}

func ptr(n int) *int { return &n }
