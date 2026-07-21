//go:build integration

// Integration tests for the materialized engagement counters (jobs.view_count /
// jobs.applied_count). The counters are bumped inside the RecordJobView /
// MarkJobApplied upserts on a first-time transition only, which can only be
// verified against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func jobCounts(t *testing.T, pool *pgxpool.Pool, jobID int64) (view, applied int32) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		"SELECT view_count, applied_count FROM jobs WHERE id = $1", jobID).Scan(&view, &applied); err != nil {
		t.Fatalf("read counts: %v", err)
	}
	return view, applied
}

func TestJobEngagementCounts(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(ctx, "TRUNCATE user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}

	t.Run("view_count is not bumped by RecordJobView (worker owns it now)", func(t *testing.T) {
		reset(t)
		u1 := insertUser(t, pool, "v1@example.test")
		u2 := insertUser(t, pool, "v2@example.test")
		jid := insertJob(t, pool, "view-count-job")

		// RecordJobView records the per-user view (user_jobs.viewed_at) but must not
		// touch jobs.view_count — that counter is now maintained solely by the
		// nginx-log aggregation worker, across all traffic.
		for _, u := range []int64{u1, u1, u2} {
			if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: u, JobID: jid}); err != nil {
				t.Fatalf("RecordJobView u=%d: %v", u, err)
			}
		}
		if v, _ := jobCounts(t, pool, jid); v != 0 {
			t.Fatalf("view_count = %d after views, want 0 (worker-owned, not bumped by the beacon)", v)
		}

		// The per-user interaction rows still exist (viewed_at set): two distinct users.
		var rows int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM user_jobs WHERE job_id = $1 AND viewed_at IS NOT NULL", jid).Scan(&rows); err != nil {
			t.Fatalf("count user_jobs: %v", err)
		}
		if rows != 2 {
			t.Fatalf("user_jobs viewed rows = %d, want 2", rows)
		}
	})

	t.Run("applied_count bumps only on the NULL->set transition", func(t *testing.T) {
		reset(t)
		u1 := insertUser(t, pool, "a1@example.test")
		u2 := insertUser(t, pool, "a2@example.test")
		jid := insertJob(t, pool, "apply-count-job")

		// A prior view alone does not touch applied_count.
		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: u1, JobID: jid}); err != nil {
			t.Fatalf("u1 view: %v", err)
		}
		if _, a := jobCounts(t, pool, jid); a != 0 {
			t.Fatalf("after view-only: applied_count = %d, want 0", a)
		}

		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: u1, JobID: jid}); err != nil {
			t.Fatalf("u1 apply: %v", err)
		}
		if _, a := jobCounts(t, pool, jid); a != 1 {
			t.Fatalf("after u1 apply: applied_count = %d, want 1", a)
		}

		// Re-applying is idempotent and must not increment again.
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: u1, JobID: jid}); err != nil {
			t.Fatalf("u1 re-apply: %v", err)
		}
		if _, a := jobCounts(t, pool, jid); a != 1 {
			t.Fatalf("after u1 re-apply: applied_count = %d, want 1", a)
		}

		// A second user applying (insert path, applied_at set directly) bumps it.
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: u2, JobID: jid}); err != nil {
			t.Fatalf("u2 apply: %v", err)
		}
		if _, a := jobCounts(t, pool, jid); a != 2 {
			t.Fatalf("after u2 apply: applied_count = %d, want 2", a)
		}
	})
}
