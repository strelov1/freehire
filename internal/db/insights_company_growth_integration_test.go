//go:build integration

// Integration test for the per-company open/growth scalar (insights_company_growth)
// that backs the /insights/companies leaderboard. The recompute is SQL, so it is only
// verifiable against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

// companyGrowthRow mirrors an insights_company_growth row for assertions.
type companyGrowthRow struct {
	openCount     int
	openCountPrev int
}

func companyGrowth(t *testing.T, ctx context.Context, q *Queries, slug string) (companyGrowthRow, bool) {
	t.Helper()
	// Read straight from the table by slug; a missing row means the company was
	// filtered out entirely.
	rows, err := q.db.Query(ctx,
		`SELECT open_count, open_count_prev FROM insights_company_growth WHERE company_slug = $1`, slug)
	if err != nil {
		t.Fatalf("query company growth: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return companyGrowthRow{}, false
	}
	var r companyGrowthRow
	if err := rows.Scan(&r.openCount, &r.openCountPrev); err != nil {
		t.Fatalf("scan company growth: %v", err)
	}
	return r, true
}

func TestInsightsCompanyGrowthScalar(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// acme (ramping): a1 open from 40d ago, a2 open from 3d ago → open now 2, open a
	// window ago 1 (a2 too new). A duplicate copy must not inflate the count.
	a1 := seedCompanyJob(t, pool, "a1", "acme", 40, nil, nil)
	seedCompanyJob(t, pool, "a2", "acme", 3, nil, nil)
	seedCompanyJob(t, pool, "adup", "acme", 10, nil, &a1) // duplicate → excluded

	// beta (freezing): b1 still open from 60d ago, b2 opened 60d ago but closed 2d ago
	// → open now 1, open a window ago 2 (b2 closed after the window start).
	closed2 := 2
	seedCompanyJob(t, pool, "b1", "beta", 60, nil, nil)
	seedCompanyJob(t, pool, "b2", "beta", 60, &closed2, nil)

	// company-less job → excluded.
	seedCompanyJob(t, pool, "e1", "", 8, nil, nil)

	if err := q.DeleteAllInsightsCompanyGrowth(ctx); err != nil {
		t.Fatalf("delete growth: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyGrowth(ctx, windowStart()); err != nil {
		t.Fatalf("rebuild growth: %v", err)
	}

	acme, ok := companyGrowth(t, ctx, q, "acme")
	if !ok || acme.openCount != 2 || acme.openCountPrev != 1 {
		t.Errorf("acme growth = %+v (present=%v), want open_count=2 open_count_prev=1 (dup excluded)", acme, ok)
	}
	beta, ok := companyGrowth(t, ctx, q, "beta")
	if !ok || beta.openCount != 1 || beta.openCountPrev != 2 {
		t.Errorf("beta growth = %+v (present=%v), want open_count=1 open_count_prev=2", beta, ok)
	}
	if _, ok := companyGrowth(t, ctx, q, ""); ok {
		t.Errorf("empty-slug company should have no growth row")
	}
}
