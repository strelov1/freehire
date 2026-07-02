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

	t.Run("view_count counts distinct viewers, once each", func(t *testing.T) {
		reset(t)
		u1 := insertUser(t, pool, "v1@example.test")
		u2 := insertUser(t, pool, "v2@example.test")
		jid := insertJob(t, pool, "view-count-job")

		if v, _ := jobCounts(t, pool, jid); v != 0 {
			t.Fatalf("initial view_count = %d, want 0", v)
		}

		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: u1, JobID: jid}); err != nil {
			t.Fatalf("u1 first view: %v", err)
		}
		if v, _ := jobCounts(t, pool, jid); v != 1 {
			t.Fatalf("after u1 first view: view_count = %d, want 1", v)
		}

		// A repeat view by the same user must not increment again.
		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: u1, JobID: jid}); err != nil {
			t.Fatalf("u1 repeat view: %v", err)
		}
		if v, _ := jobCounts(t, pool, jid); v != 1 {
			t.Fatalf("after u1 repeat view: view_count = %d, want 1", v)
		}

		// A different user is a new distinct viewer.
		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: u2, JobID: jid}); err != nil {
			t.Fatalf("u2 view: %v", err)
		}
		if v, _ := jobCounts(t, pool, jid); v != 2 {
			t.Fatalf("after u2 view: view_count = %d, want 2", v)
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
