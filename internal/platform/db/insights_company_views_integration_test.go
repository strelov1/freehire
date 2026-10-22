//go:build integration

// Integration tests for the per-company posting-views rollup (insights_company_views):
// page_uniques summed over a company's postings for the last 30 days and the 30 before.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedJobViews writes one job_daily_views row daysAgo days back. uniques and
// page_uniques are seeded to DIFFERENT values on purpose: page_uniques defaults to 0,
// so a test seeding only one column passes whichever the query reads.
func seedJobViews(t *testing.T, pool *pgxpool.Pool, jobID int64, daysAgo, uniques, pageUniques int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO job_daily_views (day, job_id, uniques, page_uniques)
		 VALUES (current_date - $2::int, $1, $3, $4)`,
		jobID, daysAgo, uniques, pageUniques); err != nil {
		t.Fatalf("seed views: %v", err)
	}
}

func rebuildViews(t *testing.T, ctx context.Context, q *Queries) {
	t.Helper()
	if err := q.DeleteAllInsightsCompanyViews(ctx); err != nil {
		t.Fatalf("delete views: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyViews(ctx); err != nil {
		t.Fatalf("rebuild views: %v", err)
	}
}

func TestInsightsCompanyViewsTwoWindowsOfPageUniques(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	a1 := seedCompanyJob(t, pool, "a1", "acme", 50, nil, nil)
	// A repost still carries the company's slug; a view of it is a view of the company.
	a2 := seedCompanyJob(t, pool, "a2", "acme", 50, nil, &a1)
	seedJobViews(t, pool, a1, 3, 500, 7)
	seedJobViews(t, pool, a2, 5, 90, 3)
	seedJobViews(t, pool, a1, 40, 60, 4)
	// Older than both windows: ignored.
	seedJobViews(t, pool, a1, 70, 80, 9)

	rebuildViews(t, ctx, q)

	row, err := q.GetCompanyViews(ctx, "acme")
	if err != nil {
		t.Fatalf("get views: %v", err)
	}
	if row.Views != 10 || row.ViewsPrev != 4 {
		t.Errorf("views = %d / %d, want 10 / 4 (page_uniques, never uniques)", row.Views, row.ViewsPrev)
	}
}

func TestInsightsCompanyViewsDropsCompaniesWithoutRecentViews(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	old := seedCompanyJob(t, pool, "o1", "oldco", 200, nil, nil)
	seedJobViews(t, pool, old, 70, 10, 10)
	none := seedCompanyJob(t, pool, "n1", "", 10, nil, nil)
	seedJobViews(t, pool, none, 1, 10, 10)

	rebuildViews(t, ctx, q)

	for _, slug := range []string{"oldco", ""} {
		if _, err := q.GetCompanyViews(ctx, slug); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%q: err = %v, want no row", slug, err)
		}
	}
}
