//go:build integration

// Integration tests for the auto_apply_queue claim/lease/park/dead-letter semantics —
// SQL behavior that can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateAutoApplyQueue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE auto_apply_queue, applications, user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// insertAutoApplyQueueEntry inserts a claimable entry: reviewed and approved, carrying a
// tailored CV, exactly what ClaimAutoApplyBatch's WHERE now requires (openspec/changes/
// auto-apply-tailored-resume). Every existing caller here is testing lease/park/failure
// semantics, not the review gate, so making "claimable" the default keeps their behavior
// unchanged; TestAutoApplyQueueClaim's own "no approved tailored CV" subtest below exercises
// the gate itself with a bare insert.
func insertAutoApplyQueueEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	cvID := insertCV(t, pool, userID)
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, tailored_cv_id, review_decision)
		 VALUES ($1, $2, $3, 'approved') RETURNING id`, userID, jobID, cvID).Scan(&id); err != nil {
		t.Fatalf("insert auto_apply_queue entry: %v", err)
	}
	return id
}

// insertBareAutoApplyQueueEntry inserts an entry with no tailored CV and no review decision
// — the shape a would-be enqueue trigger writes before tailoring has run at all.
func insertBareAutoApplyQueueEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		"INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id", userID, jobID).Scan(&id); err != nil {
		t.Fatalf("insert auto_apply_queue entry: %v", err)
	}
	return id
}

func insertCV(t *testing.T, pool *pgxpool.Pool, userID int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`, userID).Scan(&id); err != nil {
		t.Fatalf("insert cv: %v", err)
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
		if claimed[0].TailoredCvID == nil {
			t.Error("claimed tailored_cv_id not set, want the approved tailored CV's id")
		}
	})

	t.Run("an entry with no approved tailored CV is never claimed", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "untailored@example.test")
		job := insertJob(t, pool, "untailored")
		insertBareAutoApplyQueueEntry(t, pool, user, job)

		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) != 0 {
			t.Errorf("claim = %d rows, want 0 for an entry with no tailored_cv_id/review_decision", len(claimed))
		}
	})

	t.Run("a tailored but not-yet-reviewed entry is never claimed", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "unreviewed@example.test")
		job := insertJob(t, pool, "unreviewed")
		cvID := insertCV(t, pool, user)
		id := insertBareAutoApplyQueueEntry(t, pool, user, job)
		if _, err := q.SetAutoApplyTailoredCV(ctx, SetAutoApplyTailoredCVParams{
			ID: id, TailoredCvID: &cvID,
		}); err != nil {
			t.Fatal(err)
		}

		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) != 0 {
			t.Errorf("claim = %d rows, want 0 for a tailored entry with no recorded review decision", len(claimed))
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

// TestSetAutoApplyTailoredCVGuardsAnAlreadyReviewedEntry guards the race a code review
// found: PostAutoApplyTailor's own review_decision check runs before its (potentially
// minutes-long) LLM tailoring pass, not after, so a candidate can record a decision on an
// EARLIER tailored CV while a stale or retried tailor call for the same entry is still in
// flight. Without this guard, that call's own write would silently attach a fresh,
// never-reviewed CV to an already-decided entry, which ClaimAutoApplyBatch's own predicate
// would then submit as if approved.
func TestSetAutoApplyTailoredCVGuardsAnAlreadyReviewedEntry(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "raced@example.test")
	job := insertJob(t, pool, "raced")
	id := insertBareAutoApplyQueueEntry(t, pool, user, job)
	firstCV := insertCV(t, pool, user)
	if _, err := q.SetAutoApplyTailoredCV(ctx, SetAutoApplyTailoredCVParams{ID: id, TailoredCvID: &firstCV}); err != nil {
		t.Fatal(err)
	}
	if affected, err := q.ApproveAutoApplyReview(ctx, id); err != nil || affected != 1 {
		t.Fatalf("approve: affected=%d err=%v, want 1", affected, err)
	}

	// A stale second tailor pass for the same entry finishes after the approval above and
	// tries to attach a DIFFERENT, never-reviewed CV.
	secondCV := insertCV(t, pool, user)
	affected, err := q.SetAutoApplyTailoredCV(ctx, SetAutoApplyTailoredCVParams{ID: id, TailoredCvID: &secondCV})
	if err != nil {
		t.Fatal(err)
	}
	if affected != 0 {
		t.Fatalf("affected = %d, want 0 — an already-reviewed entry must refuse a new tailored cv", affected)
	}

	var stored uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT tailored_cv_id FROM auto_apply_queue WHERE id = $1", id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != firstCV {
		t.Errorf("tailored_cv_id = %s, want the approved cv %s to survive unchanged", stored, firstCV)
	}
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
