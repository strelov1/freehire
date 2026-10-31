//go:build integration

// Integration test for the per-category role pulse (insights_role_pulse) behind
// /insights/roles/<category>. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func jobIDByExternal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ext string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM jobs WHERE external_id = $1`, ext).Scan(&id); err != nil {
		t.Fatalf("id of %s: %v", ext, err)
	}
	return id
}

func setClosedReason(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ext, reason string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_reason = $1 WHERE external_id = $2`, reason, ext); err != nil {
		t.Fatalf("closed_reason %s: %v", ext, err)
	}
}

func rebuildRolePulse(t *testing.T, ctx context.Context, q *Queries) {
	t.Helper()
	if err := q.DeleteAllInsightsRolePulse(ctx); err != nil {
		t.Fatalf("delete pulse: %v", err)
	}
	now := time.Now().UTC()
	if _, err := q.RebuildInsightsRolePulse(ctx, RebuildInsightsRolePulseParams{
		WindowStart: pgtype.Timestamptz{Time: now.AddDate(0, 0, -30), Valid: true},
		PriorStart:  pgtype.Timestamptz{Time: now.AddDate(0, 0, -60), Valid: true},
	}); err != nil {
		t.Fatalf("rebuild pulse: %v", err)
	}
}

func TestInsightsRolePulseRollup(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	days := func(n int) *int { return &n }

	// --- competition (backend) ---------------------------------------------------
	// old: on the site through both windows (30 + 30 posting-days).
	seedInsightsJob(t, ctx, q, pool, "old", insightSeed{category: "backend", createdAgo: 100})
	// fresh: on the site ~9.x days of the current window — exposure starts at
	// created_at, so its posted_at 50 days back must NOT reach the earlier window.
	seedInsightsJob(t, ctx, q, pool, "fresh", insightSeed{category: "backend", createdAgo: 10, postedAgo: days(50)})
	// shut: open through the earlier window, closed 20 days ago; a view after that is
	// not attention to an open posting.
	seedInsightsJob(t, ctx, q, pool, "shut", insightSeed{category: "backend", createdAgo: 100, closedAgo: days(20)})
	setClosedReason(t, ctx, pool, "shut", "board_unreachable")
	// dup and priv would add views and exposure if counted.
	seedInsightsJob(t, ctx, q, pool, "dup", insightSeed{category: "backend", createdAgo: 100})
	seedInsightsJob(t, ctx, q, pool, "priv", insightSeed{category: "backend", createdAgo: 100})
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = (SELECT id FROM jobs WHERE external_id = 'old') WHERE external_id = 'dup'`); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private = true WHERE external_id = 'priv'`); err != nil {
		t.Fatalf("mark private: %v", err)
	}
	old := jobIDByExternal(t, ctx, pool, "old")
	// The freshest views day is yesterday, so both windows end there.
	seedJobViews(t, pool, old, 1, 99, 2)
	seedJobViews(t, pool, old, 3, 500, 7)
	seedJobViews(t, pool, old, 40, 60, 4)
	seedJobViews(t, pool, old, 70, 80, 9) // before both windows
	seedJobViews(t, pool, jobIDByExternal(t, ctx, pool, "fresh"), 2, 30, 5)
	shut := jobIDByExternal(t, ctx, pool, "shut")
	seedJobViews(t, pool, shut, 10, 40, 6) // after it closed
	seedJobViews(t, pool, shut, 45, 40, 3)
	seedJobViews(t, pool, jobIDByExternal(t, ctx, pool, "dup"), 3, 50, 50)
	seedJobViews(t, pool, jobIDByExternal(t, ctx, pool, "priv"), 3, 50, 50)

	// --- time to close (design) --------------------------------------------------
	closes := []struct {
		ext                   string
		createdAgo, closedAgo int
		postedAgo             *int
		reason                string
	}{
		{"c-backlog", 5, 1, days(60), "unseen"},   // lifetime 59 from existence, not 4
		{"c-removed", 20, 2, nil, "feed_removed"}, // 18
		{"c-probe", 30, 3, nil, "probe_expired"},  // 27
		{"c-unreach", 50, 4, nil, "board_unreachable"},
		{"c-expired", 50, 4, nil, "expired"},
		{"c-empty", 50, 4, nil, "feed_empty"},
		{"c-retired", 50, 4, nil, "board_retired"},
		{"c-mod", 50, 4, nil, "moderated"},
		{"c-unknown", 50, 4, nil, ""},
		{"c-earlier", 50, 40, nil, "employer_closed"}, // earlier window, 10
	}
	for _, c := range closes {
		seedInsightsJob(t, ctx, q, pool, c.ext, insightSeed{category: "design", createdAgo: c.createdAgo, postedAgo: c.postedAgo, closedAgo: days(c.closedAgo)})
		setClosedReason(t, ctx, pool, c.ext, c.reason)
	}

	// --- inflow (qa) -------------------------------------------------------------
	seedInsightsJob(t, ctx, q, pool, "i-backlog", insightSeed{category: "qa", createdAgo: 3, postedAgo: days(45)}) // published before the window
	seedInsightsJob(t, ctx, q, pool, "i-new", insightSeed{category: "qa", createdAgo: 3})
	seedInsightsJob(t, ctx, q, pool, "i-posted", insightSeed{category: "qa", createdAgo: 10, postedAgo: days(5)})
	seedInsightsJob(t, ctx, q, pool, "i-ancient", insightSeed{category: "qa", createdAgo: 80})

	rebuildRolePulse(t, ctx, q)

	backend, err := q.GetInsightsRolePulse(ctx, "backend")
	if err != nil {
		t.Fatalf("get backend: %v", err)
	}
	if backend.Views != 2+7+5 || backend.ViewsPrev != 4+3 {
		t.Errorf("backend views = %d / %d, want 14 / 7 (page_uniques only, open days only, no dup/priv)", backend.Views, backend.ViewsPrev)
	}
	// old 30 + fresh ~9.x + shut ~(30 - 20 + 1) in the current window; old 30 + shut 30 before.
	if backend.ExposureDays < 48 || backend.ExposureDays > 52 {
		t.Errorf("backend exposure = %.2f, want ~50", backend.ExposureDays)
	}
	if math.Abs(backend.ExposureDaysPrev-60) > 0.01 {
		t.Errorf("backend exposure prev = %.2f, want 60", backend.ExposureDaysPrev)
	}
	if want := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"); backend.ViewsThrough.Time.Format("2006-01-02") != want {
		t.Errorf("views_through = %v, want %s", backend.ViewsThrough.Time, want)
	}
	if backend.Closed != 0 {
		t.Errorf("backend closed = %d, want 0 (board_unreachable is not a close)", backend.Closed)
	}

	design, err := q.GetInsightsRolePulse(ctx, "design")
	if err != nil {
		t.Fatalf("get design: %v", err)
	}
	if design.Closed != 3 || design.ClosedPrev != 1 {
		t.Errorf("design closed = %d / %d, want 3 / 1", design.Closed, design.ClosedPrev)
	}
	if !design.MedianCloseDays.Valid || math.Abs(float64(design.MedianCloseDays.Float32)-27) > 0.1 {
		t.Errorf("design median = %+v, want 27", design.MedianCloseDays)
	}
	if !design.MedianCloseDaysPrev.Valid || math.Abs(float64(design.MedianCloseDaysPrev.Float32)-10) > 0.1 {
		t.Errorf("design median prev = %+v, want 10", design.MedianCloseDaysPrev)
	}

	qa, err := q.GetInsightsRolePulse(ctx, "qa")
	if err != nil {
		t.Fatalf("get qa: %v", err)
	}
	if qa.Inflow != 2 {
		t.Errorf("qa inflow = %d, want 2 (i-backlog existed before the window)", qa.Inflow)
	}
	if qa.MedianCloseDays.Valid {
		t.Errorf("qa median = %+v, want NULL with no close", qa.MedianCloseDays)
	}

	// The rebuild is idempotent.
	rebuildRolePulse(t, ctx, q)
	again, err := q.GetInsightsRolePulse(ctx, "design")
	if err != nil || again.Closed != design.Closed || again.MedianCloseDays != design.MedianCloseDays {
		t.Errorf("second rebuild = %+v (%v), want %+v", again, err, design)
	}
}
