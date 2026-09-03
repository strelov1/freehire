//go:build integration

// Integration tests for dbStore — the composed writes (LockJobForApply + MarkJobApplied +
// queue retirement, all one transaction) that can only be verified against a real Postgres.
// Run with: go test -tags=integration ./cmd/auto-apply/
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func truncateForAutoApply(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE auto_apply_queue, applications, user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func insertUserForTest(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertJobForTest(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, description, public_slug, company_slug)
		 VALUES ('greenhouse', $1, 'https://job-boards.greenhouse.io/acme/jobs/1', 'A job', 'Build things.', 'job-' || $1, 'acme') RETURNING id`,
		externalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func TestDBStore_Submit(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool)
	ctx := context.Background()

	t.Run("records the application and retires the queue entry together", func(t *testing.T) {
		truncateForAutoApply(t, pool)
		user := insertUserForTest(t, pool, "submit@example.test")
		job := insertJobForTest(t, pool, "submit-job")
		var queueID int64
		if err := pool.QueryRow(ctx, "INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", user, job).Scan(&queueID); err != nil {
			t.Fatal(err)
		}

		if err := store.Submit(ctx, autoapply.Claimed{QueueID: queueID, UserID: user, JobID: job}); err != nil {
			t.Fatalf("Submit: %v", err)
		}

		var appliedAt any
		if err := pool.QueryRow(ctx, "SELECT applied_at FROM applications WHERE user_id = $1 AND job_id = $2", user, job).Scan(&appliedAt); err != nil {
			t.Fatalf("application not recorded: %v", err)
		}
		if appliedAt == nil {
			t.Error("applied_at is null, want it set")
		}

		var queueRows int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM auto_apply_queue WHERE id = $1", queueID).Scan(&queueRows); err != nil {
			t.Fatal(err)
		}
		if queueRows != 0 {
			t.Errorf("queue rows = %d, want 0 — the entry must be retired", queueRows)
		}
	})

	// The spec's "never twice" requirement: MarkJobApplied is idempotent (ON CONFLICT DO
	// UPDATE), so even a reprocessed attempt for an already-applied pair must not bump
	// applied_count a second time or create a second applications row.
	t.Run("a pair that already has a submitted application is not double-counted", func(t *testing.T) {
		truncateForAutoApply(t, pool)
		user := insertUserForTest(t, pool, "twice@example.test")
		job := insertJobForTest(t, pool, "twice-job")
		var firstQueueID, secondQueueID int64
		if err := pool.QueryRow(ctx, "INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", user, job).Scan(&firstQueueID); err != nil {
			t.Fatal(err)
		}

		if err := store.Submit(ctx, autoapply.Claimed{QueueID: firstQueueID, UserID: user, JobID: job}); err != nil {
			t.Fatalf("first Submit: %v", err)
		}

		// Simulate a second attempt reaching Submit for the same pair (e.g. a second
		// queue row inserted before the first's completion was visible elsewhere).
		if err := pool.QueryRow(ctx, "INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", user, job).Scan(&secondQueueID); err != nil {
			t.Fatal(err)
		}
		if err := store.Submit(ctx, autoapply.Claimed{QueueID: secondQueueID, UserID: user, JobID: job}); err != nil {
			t.Fatalf("second Submit: %v", err)
		}

		var applications int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications WHERE user_id = $1 AND job_id = $2", user, job).Scan(&applications); err != nil {
			t.Fatal(err)
		}
		if applications != 1 {
			t.Errorf("applications = %d, want exactly 1 (idempotent re-apply, not a duplicate)", applications)
		}

		var appliedCount int
		if err := pool.QueryRow(ctx, "SELECT applied_count FROM jobs WHERE id = $1", job).Scan(&appliedCount); err != nil {
			t.Fatal(err)
		}
		if appliedCount != 1 {
			t.Errorf("jobs.applied_count = %d, want 1 (must not double-bump on the second Submit)", appliedCount)
		}
	})

	t.Run("Park records the reasons without touching user_jobs", func(t *testing.T) {
		truncateForAutoApply(t, pool)
		user := insertUserForTest(t, pool, "park@example.test")
		job := insertJobForTest(t, pool, "park-job")
		var queueID int64
		if err := pool.QueryRow(ctx, "INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", user, job).Scan(&queueID); err != nil {
			t.Fatal(err)
		}

		unmapped := []autoapply.UnmappedField{{ID: "question_1", Label: "Why us?", Required: true, Reason: "no known answer"}}
		if err := store.Park(ctx, queueID, unmapped, "1 required question unanswered"); err != nil {
			t.Fatalf("Park: %v", err)
		}

		var userJobsRows int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs WHERE user_id = $1 AND job_id = $2", user, job).Scan(&userJobsRows); err != nil {
			t.Fatal(err)
		}
		if userJobsRows != 0 {
			t.Errorf("user_jobs rows = %d, want 0 — parking must not touch tracking", userJobsRows)
		}

		var stillQueued int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM auto_apply_queue WHERE id = $1 AND blocked_at IS NOT NULL", queueID).Scan(&stillQueued); err != nil {
			t.Fatal(err)
		}
		if stillQueued != 1 {
			t.Error("queue entry not marked blocked")
		}
	})
}
