//go:build integration

// Integration tests for the per-company posting-lifetime rollup
// (insights_company_lifetime): the median days from created_at to an EMPLOYER-side
// closure, over the canonical postings closed in the last 180 days.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedClosedCompanyJob inserts one closed job opened createdAgo days ago and closed
// closedAgo days ago with the given closed_reason. dupOf marks it a duplicate.
func seedClosedCompanyJob(t *testing.T, pool *pgxpool.Pool, ext, slug string, createdAgo, closedAgo int, reason string, dupOf *int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, duplicate_of_role,
			created_at, closed_at, closed_reason)
		VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3,
			now() - make_interval(days => $4::int), now() - make_interval(days => $5::int), $6)`,
		ext, slug, dupOf, createdAgo, closedAgo, reason)
	if err != nil {
		t.Fatalf("seed closed company job %s: %v", ext, err)
	}
}

func rebuildLifetime(t *testing.T, ctx context.Context, q *Queries) {
	t.Helper()
	if err := q.DeleteAllInsightsCompanyLifetime(ctx); err != nil {
		t.Fatalf("delete lifetime: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyLifetime(ctx); err != nil {
		t.Fatalf("rebuild lifetime: %v", err)
	}
}

func TestInsightsCompanyLifetimeMedianOfEmployerClosures(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Open 10, 20 and 30 days before closing 5 days ago, under three different
	// employer-side reasons.
	seedClosedCompanyJob(t, pool, "a1", "acme", 15, 5, "unseen", nil)
	seedClosedCompanyJob(t, pool, "a2", "acme", 25, 5, "employer_closed", nil)
	seedClosedCompanyJob(t, pool, "a3", "acme", 35, 5, "expired", nil)
	// Still open: no closure, nothing to measure.
	seedCompanyJob(t, pool, "a4", "acme", 40, nil, nil)

	rebuildLifetime(t, ctx, q)

	row, err := q.GetCompanyLifetime(ctx, "acme")
	if err != nil {
		t.Fatalf("get lifetime: %v", err)
	}
	if row.Closed != 3 {
		t.Errorf("closed = %d, want 3", row.Closed)
	}
	if row.MedianDays < 19.9 || row.MedianDays > 20.1 {
		t.Errorf("median_days = %v, want 20", row.MedianDays)
	}
}

func TestInsightsCompanyLifetimeExcludesOurOwnClosures(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Every one of these says something about our crawler or our decisions, not
	// about how fast the employer hires.
	for i, reason := range []string{"board_unreachable", "feed_empty", "source_misattributed", "moderated"} {
		seedClosedCompanyJob(t, pool, "x"+string(rune('a'+i)), "crawlfail", 30, 2, reason, nil)
	}

	rebuildLifetime(t, ctx, q)

	if _, err := q.GetCompanyLifetime(ctx, "crawlfail"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("crawlfail row: err = %v, want no row", err)
	}
}

func TestInsightsCompanyLifetimeExcludesOldClosuresAndDuplicates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	canonical := seedCompanyJob(t, pool, "c1", "globex", 300, nil, nil)
	// Closed 200 days ago: outside the window.
	seedClosedCompanyJob(t, pool, "old", "globex", 260, 200, "unseen", nil)
	// A repost copy closed yesterday: the canonical posting stands for it.
	seedClosedCompanyJob(t, pool, "dup", "globex", 5, 1, "unseen", &canonical)
	// Closed before it was created: a data quirk, not a negative lifetime.
	seedClosedCompanyJob(t, pool, "skew", "globex", 1, 3, "unseen", nil)
	// One qualifying closure so the company has a row to inspect.
	seedClosedCompanyJob(t, pool, "ok", "globex", 8, 1, "feed_removed", nil)

	rebuildLifetime(t, ctx, q)

	row, err := q.GetCompanyLifetime(ctx, "globex")
	if err != nil {
		t.Fatalf("get lifetime: %v", err)
	}
	if row.Closed != 1 || row.MedianDays < 6.9 || row.MedianDays > 7.1 {
		t.Errorf("globex = %+v, want closed 1, median 7", row)
	}
}

// insertCompanyStat writes one insights_company_stats row directly, so the series
// tests control the activity days exactly.
func insertCompanyStat(t *testing.T, pool *pgxpool.Pool, slug, day string, added, removed int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO insights_company_stats (company_slug, day, added, removed, open) VALUES ($1, $2::date, $3, $4, 0)`,
		slug, day, added, removed); err != nil {
		t.Fatalf("insert company stat: %v", err)
	}
}

func dayTS(t *testing.T, day string) pgtype.Timestamp {
	t.Helper()
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.Timestamp{Time: d, Valid: true}
}

func TestListCompanyActivityIsDense(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	if _, err := pool.Exec(ctx, `DELETE FROM insights_company_stats`); err != nil {
		t.Fatal(err)
	}

	insertCompanyStat(t, pool, "acme", "2026-10-02", 3, 0)
	insertCompanyStat(t, pool, "acme", "2026-10-05", 1, 2)
	// Another company's activity on the same days must not leak in.
	insertCompanyStat(t, pool, "globex", "2026-10-02", 9, 9)

	rows, err := q.ListCompanyActivity(ctx, ListCompanyActivityParams{
		CompanySlug: "acme", Unit: "day", FromTs: dayTS(t, "2026-10-01"), ToTs: dayTS(t, "2026-10-06"),
	})
	if err != nil {
		t.Fatalf("list company activity: %v", err)
	}
	want := []struct{ added, removed int32 }{{0, 0}, {3, 0}, {0, 0}, {0, 0}, {1, 2}, {0, 0}}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i].Added != w.added || rows[i].Removed != w.removed {
			t.Errorf("day %d = %+v, want %+v", i, rows[i], w)
		}
	}
}

func TestListCompanyActivityRollsUpWeeks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	if _, err := pool.Exec(ctx, `DELETE FROM insights_company_stats`); err != nil {
		t.Fatal(err)
	}

	// 2026-10-05 is a Monday: the first two days fall in the week of 09-28, the
	// last two in the week of 10-05.
	insertCompanyStat(t, pool, "acme", "2026-10-01", 1, 0)
	insertCompanyStat(t, pool, "acme", "2026-10-03", 2, 1)
	insertCompanyStat(t, pool, "acme", "2026-10-05", 4, 0)
	insertCompanyStat(t, pool, "acme", "2026-10-08", 0, 3)

	rows, err := q.ListCompanyActivity(ctx, ListCompanyActivityParams{
		CompanySlug: "acme", Unit: "week", FromTs: dayTS(t, "2026-10-01"), ToTs: dayTS(t, "2026-10-11"),
	})
	if err != nil {
		t.Fatalf("list company activity: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d weeks, want 2: %+v", len(rows), rows)
	}
	if rows[0].Added != 3 || rows[0].Removed != 1 || rows[1].Added != 4 || rows[1].Removed != 3 {
		t.Errorf("weeks = %+v, want {3,1} then {4,3}", rows)
	}
}

func TestGetCompanyGrowth(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	if _, err := pool.Exec(ctx, `DELETE FROM insights_company_growth;
		INSERT INTO insights_company_growth (company_slug, open_count, open_count_prev) VALUES ('acme', 42, 30)`); err != nil {
		t.Fatal(err)
	}

	row, err := q.GetCompanyGrowth(ctx, "acme")
	if err != nil {
		t.Fatalf("get company growth: %v", err)
	}
	if row.OpenCount != 42 || row.OpenCountPrev != 30 {
		t.Errorf("growth = %+v, want 42 / 30", row)
	}
	if _, err := q.GetCompanyGrowth(ctx, "nobody"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("nobody: err = %v, want no row", err)
	}
}
