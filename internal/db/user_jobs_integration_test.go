//go:build integration

// Integration tests for the user_jobs upsert queries — view recording and apply
// marking are ON CONFLICT semantics that can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestUserJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	countRows := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	t.Run("RecordJobView creates then refreshes one row", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "TRUNCATE user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		uid := insertUser(t, pool, "viewer@example.test")
		jid := insertJob(t, pool, "view-job")

		first, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("first view: %v", err)
		}
		if !first.ViewedAt.Valid || first.AppliedAt.Valid {
			t.Errorf("first view: viewed=%v applied=%v, want viewed/not-applied", first.ViewedAt.Valid, first.AppliedAt.Valid)
		}

		second, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("second view: %v", err)
		}
		if n := countRows(t); n != 1 {
			t.Errorf("rows = %d, want 1 (one per (user, job))", n)
		}
		if second.ViewedAt.Time.Before(first.ViewedAt.Time) {
			t.Errorf("second viewed_at %v is before first %v — not refreshed", second.ViewedAt.Time, first.ViewedAt.Time)
		}
		if second.AppliedAt.Valid {
			t.Error("a view must not set applied_at")
		}
	})

	t.Run("MarkJobApplied sets applied_at without a prior view and is idempotent", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "TRUNCATE user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		uid := insertUser(t, pool, "applier@example.test")
		jid := insertJob(t, pool, "apply-job")

		// No prior RecordJobView: the insert path must still populate viewed_at.
		first, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("first apply: %v", err)
		}
		if !first.AppliedAt.Valid || !first.ViewedAt.Valid {
			t.Errorf("first apply: applied=%v viewed=%v, want both set", first.AppliedAt.Valid, first.ViewedAt.Valid)
		}

		second, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("second apply: %v", err)
		}
		if n := countRows(t); n != 1 {
			t.Errorf("rows = %d, want 1 (idempotent)", n)
		}
		if second.AppliedAt.Time.Before(first.AppliedAt.Time) {
			t.Errorf("second applied_at %v is before first %v", second.AppliedAt.Time, first.AppliedAt.Time)
		}
	})

	t.Run("apply after view keeps the single row and preserves viewed_at", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "TRUNCATE user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		uid := insertUser(t, pool, "both@example.test")
		jid := insertJob(t, pool, "both-job")

		viewed, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("view: %v", err)
		}
		applied, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if n := countRows(t); n != 1 {
			t.Errorf("rows = %d, want 1 (view then apply is the same row)", n)
		}
		if !applied.AppliedAt.Valid {
			t.Error("apply after view must set applied_at")
		}
		if !applied.ViewedAt.Time.Equal(viewed.ViewedAt.Time) {
			t.Errorf("apply changed viewed_at: %v -> %v", viewed.ViewedAt.Time, applied.ViewedAt.Time)
		}
	})
}

func TestUserJobsSaveAndList(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(ctx, "TRUNCATE user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	countRows := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	t.Run("SaveJob creates the row without a prior view and is idempotent", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "saver@example.test")
		jid := insertJob(t, pool, "save-job")

		first, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("first save: %v", err)
		}
		if !first.SavedAt.Valid || !first.ViewedAt.Valid || first.AppliedAt.Valid {
			t.Errorf("first save: saved=%v viewed=%v applied=%v, want saved+viewed, not applied",
				first.SavedAt.Valid, first.ViewedAt.Valid, first.AppliedAt.Valid)
		}

		second, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("second save: %v", err)
		}
		if n := countRows(t); n != 1 {
			t.Errorf("rows = %d, want 1 (idempotent)", n)
		}
		if second.SavedAt.Time.Before(first.SavedAt.Time) {
			t.Errorf("second saved_at %v is before first %v", second.SavedAt.Time, first.SavedAt.Time)
		}
	})

	t.Run("UnsaveJob clears saved_at but keeps view and apply history", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "unsaver@example.test")
		jid := insertJob(t, pool, "unsave-job")

		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("view: %v", err)
		}
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("save: %v", err)
		}

		row, err := q.UnsaveJob(ctx, UnsaveJobParams{UserID: uid, JobID: jid})
		if err != nil {
			t.Fatalf("unsave: %v", err)
		}
		if row.SavedAt.Valid {
			t.Error("unsave left saved_at set")
		}
		if !row.ViewedAt.Valid || !row.AppliedAt.Valid {
			t.Errorf("unsave lost history: viewed=%v applied=%v, want both kept", row.ViewedAt.Valid, row.AppliedAt.Valid)
		}
		if n := countRows(t); n != 1 {
			t.Errorf("rows = %d, want 1 (unsave must not delete the row)", n)
		}
	})

	t.Run("UnsaveJob without an interaction row reports no rows", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "ghost@example.test")
		jid := insertJob(t, pool, "ghost-job")

		_, err := q.UnsaveJob(ctx, UnsaveJobParams{UserID: uid, JobID: jid})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("unsave without row: err = %v, want pgx.ErrNoRows", err)
		}
		if n := countRows(t); n != 0 {
			t.Errorf("rows = %d, want 0 (unsave must not create a row)", n)
		}
	})

	t.Run("ListUserJobs filters, orders by latest activity, and scopes to the user", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "lister@example.test")
		other := insertUser(t, pool, "other@example.test")
		viewedJob := insertJob(t, pool, "only-viewed")
		savedJob := insertJob(t, pool, "viewed-then-saved")
		appliedJob := insertJob(t, pool, "applied")

		// Interactions in chronological order: view A, view+save B, apply C.
		// Latest-activity ordering must therefore be C, B, A.
		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: viewedJob}); err != nil {
			t.Fatalf("view A: %v", err)
		}
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: savedJob}); err != nil {
			t.Fatalf("save B: %v", err)
		}
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: appliedJob}); err != nil {
			t.Fatalf("apply C: %v", err)
		}
		// Another user's row must never leak into the listing.
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: other, JobID: viewedJob}); err != nil {
			t.Fatalf("other save: %v", err)
		}

		all, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "all", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("list all: %d rows, want 3", len(all))
		}
		gotOrder := []int64{all[0].Job.ID, all[1].Job.ID, all[2].Job.ID}
		wantOrder := []int64{appliedJob, savedJob, viewedJob}
		for i := range wantOrder {
			if gotOrder[i] != wantOrder[i] {
				t.Fatalf("list all order = %v, want %v (latest activity first)", gotOrder, wantOrder)
			}
		}

		viewed, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "viewed", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list viewed: %v", err)
		}
		if len(viewed) != 1 || viewed[0].Job.ID != viewedJob {
			t.Fatalf("list viewed: got %d rows, want exactly the view-only job (not saved, not applied)", len(viewed))
		}

		saved, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "saved", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list saved: %v", err)
		}
		if len(saved) != 1 || saved[0].Job.ID != savedJob {
			t.Fatalf("list saved: got %d rows, want exactly the saved job", len(saved))
		}

		applied, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "applied", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list applied: %v", err)
		}
		if len(applied) != 1 || applied[0].Job.ID != appliedJob {
			t.Fatalf("list applied: got %d rows, want exactly the applied job", len(applied))
		}

		counts, err := q.CountUserJobs(ctx, uid)
		if err != nil {
			t.Fatalf("counts: %v", err)
		}
		if counts.All != 3 || counts.Viewed != 1 || counts.Saved != 1 || counts.Applied != 1 {
			t.Errorf("counts = %+v, want all=3 viewed=1 saved=1 applied=1", counts)
		}
	})

	t.Run("saved is ordered by saved_at and a later view does not bump it", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "orderer@example.test")
		older := insertJob(t, pool, "saved-older")
		newer := insertJob(t, pool, "saved-newer")

		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: older}); err != nil {
			t.Fatalf("save older: %v", err)
		}
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: newer}); err != nil {
			t.Fatalf("save newer: %v", err)
		}
		// Pin saved_at deterministically: `older` saved before `newer`.
		if _, err := pool.Exec(ctx, "UPDATE user_jobs SET saved_at = $1 WHERE user_id = $2 AND job_id = $3",
			"2020-01-01T00:00:00Z", uid, older); err != nil {
			t.Fatalf("pin older saved_at: %v", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE user_jobs SET saved_at = $1 WHERE user_id = $2 AND job_id = $3",
			"2020-06-01T00:00:00Z", uid, newer); err != nil {
			t.Fatalf("pin newer saved_at: %v", err)
		}

		wantSaved := func(t *testing.T, label string) {
			t.Helper()
			saved, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "saved", Limit: 10, Offset: 0})
			if err != nil {
				t.Fatalf("list saved (%s): %v", label, err)
			}
			got := []int64{}
			for _, r := range saved {
				got = append(got, r.Job.ID)
			}
			want := []int64{newer, older}
			if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
				t.Fatalf("saved order (%s) = %v, want %v (most recently saved first)", label, got, want)
			}
		}

		wantSaved(t, "after save")

		// Re-viewing the older-saved job refreshes its viewed_at to now(), which is
		// far newer than either saved_at. A saved list ordered by GREATEST(...) would
		// wrongly float `older` to the top; ordering by saved_at must keep it last.
		if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: older}); err != nil {
			t.Fatalf("re-view older: %v", err)
		}
		wantSaved(t, "after re-view")
	})
}
