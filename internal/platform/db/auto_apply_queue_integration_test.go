//go:build integration

// Integration tests for the auto_apply_queue claim/lease/park/dead-letter semantics —
// SQL behavior that can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateAutoApplyQueue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE auto_apply_queue, applications, user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func insertAutoApplyQueueEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		"INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", userID, jobID).Scan(&id); err != nil {
		t.Fatalf("insert auto_apply_queue entry: %v", err)
	}
	return id
}

func TestAutoApplyQueueClaim(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a live entry is claimed and carries the job's identity", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "claim@example.test")
		job := insertJob(t, pool, "gh-claim")
		insertAutoApplyQueueEntry(t, pool, user, job)

		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(claimed), err)
		}
		if claimed[0].UserID != user || claimed[0].JobID != job {
			t.Errorf("claimed identity = (%d, %d), want (%d, %d)", claimed[0].UserID, claimed[0].JobID, user, job)
		}
		if claimed[0].Source != "test" || claimed[0].URL != "http://example.test" {
			t.Errorf("claimed source/url = (%q, %q), want the job's own", claimed[0].Source, claimed[0].URL)
		}
		if claimed[0].ExternalID != "gh-claim" {
			t.Errorf("claimed external_id = %q, want the job's own (internal/atsapply needs it to reuse internal/applyform's schema fetchers)", claimed[0].ExternalID)
		}
	})

	t.Run("a claimed entry is leased away from a second claim", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "leased@example.test")
		job := insertJob(t, pool, "leased")
		insertAutoApplyQueueEntry(t, pool, user, job)

		if _, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		again, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 0 {
			t.Errorf("second claim = %d rows, want 0 while the lease holds", len(again))
		}
	})

	t.Run("an expired lease is reclaimed without a reaper", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "expired@example.test")
		job := insertJob(t, pool, "expired")
		insertAutoApplyQueueEntry(t, pool, user, job)

		if _, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		again, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 1 {
			t.Errorf("reclaim = %d rows, want 1 once its lease expired", len(again))
		}
	})
}

func TestAutoApplyQueueCompletion(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("completing an attempt removes it from the queue", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "done@example.test")
		job := insertJob(t, pool, "done")
		id := insertAutoApplyQueueEntry(t, pool, user, job)

		if err := q.DeleteAutoApplyEntry(ctx, id); err != nil {
			t.Fatal(err)
		}

		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM auto_apply_queue").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("queue rows = %d, want 0 after completion", n)
		}
	})
}

func TestAutoApplyQueueBlocked(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a blocked entry records its reasons and is never reclaimed", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "blocked@example.test")
		job := insertJob(t, pool, "blocked")
		id := insertAutoApplyQueueEntry(t, pool, user, job)
		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v", len(claimed), err)
		}

		unmapped, _ := json.Marshal([]map[string]any{{"id": "question_1", "label": "Why us?", "required": true, "reason": "no known answer"}})
		if err := q.MarkAutoApplyBlocked(ctx, MarkAutoApplyBlockedParams{
			ID: id, LastError: "1 required question unanswered", Unmapped: unmapped,
		}); err != nil {
			t.Fatal(err)
		}

		var blockedAt any
		var storedUnmapped []byte
		if err := pool.QueryRow(ctx, "SELECT blocked_at, unmapped FROM auto_apply_queue WHERE id = $1", id).
			Scan(&blockedAt, &storedUnmapped); err != nil {
			t.Fatal(err)
		}
		if blockedAt == nil {
			t.Error("blocked_at not set")
		}
		var decoded []map[string]any
		if err := json.Unmarshal(storedUnmapped, &decoded); err != nil || len(decoded) != 1 {
			t.Errorf("stored unmapped = %s, want the one field", storedUnmapped)
		}

		// Even with a lapsed lease, a blocked entry must not come back to a claim — it
		// needs new data, not another try.
		again, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 0 {
			t.Errorf("claim after blocking = %d rows, want 0", len(again))
		}
	})
}

func TestAutoApplyQueueFailure(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a failure is recorded and retried until attempts run out", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "flaky@example.test")
		job := insertJob(t, pool, "flaky")
		id := insertAutoApplyQueueEntry(t, pool, user, job)
		if _, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}

		first, err := q.RecordAutoApplyFailure(ctx, RecordAutoApplyFailureParams{ID: id, LastError: "boom", MaxAttempts: 2})
		if err != nil {
			t.Fatal(err)
		}
		if first.FailedAt.Valid {
			t.Fatal("first failure dead-lettered, want it retried")
		}

		retry, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil || len(retry) != 1 {
			t.Fatalf("reclaim on a later run: rows=%d err=%v, want 1", len(retry), err)
		}

		second, err := q.RecordAutoApplyFailure(ctx, RecordAutoApplyFailureParams{ID: id, LastError: "boom again", MaxAttempts: 2})
		if err != nil {
			t.Fatal(err)
		}
		if !second.FailedAt.Valid {
			t.Error("second failure not dead-lettered, want it marked failed at max attempts")
		}

		final, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(final) != 0 {
			t.Errorf("claim after dead-letter = %d rows, want 0", len(final))
		}
	})
}
