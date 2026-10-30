//go:build integration

// Integration test for the role × country rollup (insights_role_country_stats) behind
// the grid on /insights. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roleCountryCell struct{ open, prev int32 }

// roleCountryCells reads the whole table, keyed "category/country".
func roleCountryCells(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]roleCountryCell {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT category, country, open_count, open_count_prev FROM insights_role_country_stats`)
	if err != nil {
		t.Fatalf("query role-country stats: %v", err)
	}
	defer rows.Close()
	out := map[string]roleCountryCell{}
	for rows.Next() {
		var cat, country string
		var c roleCountryCell
		if err := rows.Scan(&cat, &country, &c.open, &c.prev); err != nil {
			t.Fatalf("scan role-country stats: %v", err)
		}
		out[cat+"/"+country] = c
	}
	return out
}

func rebuildRoleCountries(t *testing.T, ctx context.Context, q *Queries, prev pgtype.Timestamptz) {
	t.Helper()
	if err := q.DeleteAllInsightsRoleCountryStats(ctx); err != nil {
		t.Fatalf("delete role-country: %v", err)
	}
	if _, err := q.RebuildInsightsRoleCountryStatsGlobal(ctx, prev); err != nil {
		t.Fatalf("rebuild role-country global: %v", err)
	}
	if _, err := q.RebuildInsightsRoleCountryStatsByCountry(ctx, prev); err != nil {
		t.Fatalf("rebuild role-country by country: %v", err)
	}
}

func TestInsightsRoleCountryRollup(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	closed2 := 2
	// two: one posting in two countries, open since before the window.
	seedInsightsJob(t, ctx, q, pool, "two", insightSeed{category: "backend", seniority: "senior", countries: []string{"de", "nl"}, createdAgo: 60})
	// nosen: no seniority — the role rollup skips it, this one must not.
	seedInsightsJob(t, ctx, q, pool, "nosen", insightSeed{category: "backend", countries: []string{"de"}, createdAgo: 10})
	// backlog: first seen 5 days ago but published 60 days ago — it existed at the
	// window start, so it counts in open_count_prev (the insights.sql growth rule).
	seedInsightsJob(t, ctx, q, pool, "backlog", insightSeed{category: "backend", countries: []string{"nl"}, createdAgo: 5, postedAgo: intPtr(60)})
	// nowhere: no country — the all-countries bucket only.
	seedInsightsJob(t, ctx, q, pool, "nowhere", insightSeed{category: "backend", createdAgo: 5})
	// dup and priv would add to backend/de if counted.
	seedInsightsJob(t, ctx, q, pool, "dup", insightSeed{category: "backend", countries: []string{"de"}, createdAgo: 60})
	seedInsightsJob(t, ctx, q, pool, "priv", insightSeed{category: "backend", countries: []string{"de"}, createdAgo: 60})
	// gone: open a window ago, closed since — a row with open 0, prev 1.
	seedInsightsJob(t, ctx, q, pool, "gone", insightSeed{category: "design", countries: []string{"us"}, createdAgo: 60, closedAgo: &closed2})
	// uncategorized: counted nowhere.
	seedInsightsJob(t, ctx, q, pool, "nocat", insightSeed{countries: []string{"de"}, createdAgo: 60})

	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = (SELECT id FROM jobs WHERE external_id = 'two') WHERE external_id = 'dup'`); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private = true WHERE external_id = 'priv'`); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	rebuildRoleCountries(t, ctx, q, windowStart())

	want := map[string]roleCountryCell{
		"backend/":   {open: 4, prev: 2}, // two (once, not twice), nosen, nowhere, backlog
		"backend/de": {open: 2, prev: 1}, // two, nosen
		"backend/nl": {open: 2, prev: 2}, // two, backlog
		"design/":    {open: 0, prev: 1},
		"design/us":  {open: 0, prev: 1},
	}
	got := roleCountryCells(t, ctx, pool)
	for key, w := range want {
		if got[key] != w {
			t.Errorf("%s = %+v, want %+v", key, got[key], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("table holds %d cells %v, want exactly %d", len(got), got, len(want))
	}
}
