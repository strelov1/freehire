//go:build integration

// Integration tests for the auto_apply_queue claim/lease/park/dead-letter semantics —
// SQL behavior that can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// insertTailoredAutoApplyQueueEntry inserts an entry with a tailored CV but no review
// decision yet — exactly ClaimAutoApplyPreviewBatch's own claimable shape (openspec/changes/
// auto-apply-review-tracking): tailored, awaiting a resolved preview, not yet reviewed.
func insertTailoredAutoApplyQueueEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	cvID := insertCV(t, pool, userID)
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, tailored_cv_id) VALUES ($1, $2, $3) RETURNING id`,
		userID, jobID, cvID).Scan(&id); err != nil {
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

// TestClaimAutoApplyPreviewBatch and TestSetAutoApplyResolvedPreview cover
// openspec/changes/auto-apply-review-tracking's own claim/write pair — the second, disjoint
// predicate cmd/auto-apply's preview pass shares the queue table with, and the guard that
// keeps it from overwriting an entry the candidate already decided on.
func TestClaimAutoApplyPreviewBatch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a tailored, unreviewed entry is claimable", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-claim@example.test")
		job := insertJob(t, pool, "preview-claim")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)

		rows, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 300, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != id {
			t.Fatalf("claimed = %+v, want the one tailored entry", rows)
		}
	})

	t.Run("an already-approved entry is not claimable here — ClaimAutoApplyBatch owns it instead", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-approved@example.test")
		job := insertJob(t, pool, "preview-approved")
		insertAutoApplyQueueEntry(t, pool, user, job) // tailored AND approved

		rows, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 300, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("claimed = %+v, want none — the two claim predicates must stay disjoint", rows)
		}
	})

	t.Run("an entry with no tailored CV yet is not claimable", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-untailored@example.test")
		job := insertJob(t, pool, "preview-untailored")
		insertBareAutoApplyQueueEntry(t, pool, user, job)

		rows, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 300, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("claimed = %+v, want none — nothing to preview before tailoring finishes", rows)
		}
	})

	t.Run("an entry with a resolved preview already is not reclaimed", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-done@example.test")
		job := insertJob(t, pool, "preview-done")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)
		if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: []byte(`{"fields":[]}`)}); err != nil {
			t.Fatal(err)
		}

		rows, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 300, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("claimed = %+v, want none — already resolved", rows)
		}
	})
}

func TestSetAutoApplyResolvedPreview(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("persists the preview and returns the job's own fields", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-set@example.test")
		job := insertJob(t, pool, "preview-set")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)

		row, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{
			ID: id, ResolvedPreview: []byte(`{"fields":[{"label":"First name","value":"Ada"}]}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if row.UserID != user {
			t.Errorf("user_id = %d, want %d", row.UserID, user)
		}

		var stored []byte
		if err := pool.QueryRow(ctx, "SELECT resolved_preview FROM auto_apply_queue WHERE id = $1", id).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if string(stored) == "" {
			t.Error("resolved_preview was not persisted")
		}
	})

	t.Run("refuses to overwrite an already-decided entry", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-decided@example.test")
		job := insertJob(t, pool, "preview-decided")
		id := insertAutoApplyQueueEntry(t, pool, user, job) // tailored AND approved already

		_, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{
			ID: id, ResolvedPreview: []byte(`{"fields":[]}`),
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, want pgx.ErrNoRows — an already-decided entry must refuse a new preview", err)
		}

		var stored *string
		if err := pool.QueryRow(ctx, "SELECT resolved_preview FROM auto_apply_queue WHERE id = $1", id).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != nil {
			t.Errorf("resolved_preview = %v, want nil — the guard must leave it untouched", *stored)
		}
	})
}

// TestRecordAutoApplyPreviewFailure_IndependentFromSubmitFailureBudget guards a bug a code
// review found: the preview pass and the real submission pass used to share
// attempts/failed_at, so a transient preview-resolution error could spend down the same
// retry budget the real ATS submission depends on, and could dead-letter a row before a
// submission was ever attempted (openspec/changes/auto-apply-review-tracking).
func TestRecordAutoApplyPreviewFailure_IndependentFromSubmitFailureBudget(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("preview failures dead-letter preview_failed_at without ever touching failed_at", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-fail@example.test")
		job := insertJob(t, pool, "preview-fail")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)

		first, err := q.RecordAutoApplyPreviewFailure(ctx, RecordAutoApplyPreviewFailureParams{ID: id, LastError: "boom", MaxAttempts: 2})
		if err != nil {
			t.Fatal(err)
		}
		if first.PreviewFailedAt.Valid {
			t.Fatal("first preview failure dead-lettered, want it retried")
		}

		second, err := q.RecordAutoApplyPreviewFailure(ctx, RecordAutoApplyPreviewFailureParams{ID: id, LastError: "boom again", MaxAttempts: 2})
		if err != nil {
			t.Fatal(err)
		}
		if !second.PreviewFailedAt.Valid {
			t.Error("second preview failure not dead-lettered, want it marked failed at max attempts")
		}

		var attempts int
		var failedAt *time.Time
		if err := pool.QueryRow(ctx, "SELECT attempts, failed_at FROM auto_apply_queue WHERE id = $1", id).Scan(&attempts, &failedAt); err != nil {
			t.Fatal(err)
		}
		if attempts != 0 || failedAt != nil {
			t.Errorf("attempts=%d failed_at=%v, want both untouched by preview failures", attempts, failedAt)
		}
	})

	t.Run("the submit pass gets a fresh attempts budget regardless of prior preview failures", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "preview-then-submit@example.test")
		job := insertJob(t, pool, "preview-then-submit")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)

		// Exhaust the preview pass's own budget first.
		if _, err := q.RecordAutoApplyPreviewFailure(ctx, RecordAutoApplyPreviewFailureParams{ID: id, LastError: "flaky schema fetch", MaxAttempts: 1}); err != nil {
			t.Fatal(err)
		}

		// A resolved preview lands anyway (e.g. a later, out-of-band pass, or this test
		// simulating one) and the candidate approves.
		if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: []byte(`{"fields":[]}`)}); err != nil {
			t.Fatal(err)
		}
		if affected, err := q.ApproveAutoApplyReview(ctx, id); err != nil || affected != 1 {
			t.Fatalf("approve: affected=%d err=%v, want 1", affected, err)
		}

		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1 — a prior preview failure must not block the real claim", len(claimed), err)
		}

		// A full max_attempts=3 budget for the SUBMIT pass, unaffected by the preview
		// pass having already exhausted its own budget of 1 above.
		for i := 0; i < 2; i++ {
			row, err := q.RecordAutoApplyFailure(ctx, RecordAutoApplyFailureParams{ID: id, LastError: "transient", MaxAttempts: 3})
			if err != nil {
				t.Fatal(err)
			}
			if row.FailedAt.Valid {
				t.Fatalf("dead-lettered after %d submit failures, want 3 full attempts available", i+1)
			}
		}
	})
}

// TestSetAutoApplyTailoredCV_ClearsAStalePreviewOnRetailor guards the other half of the
// same review finding: a deliberate re-tailor must invalidate the preview computed against
// the PREVIOUS CV, and must not leave the entry permanently unclaimable by the preview pass
// if it had already exhausted its own attempts budget once.
func TestSetAutoApplyTailoredCV_ClearsAStalePreviewOnRetailor(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "retailor@example.test")
	job := insertJob(t, pool, "retailor")
	id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)
	if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: []byte(`{"fields":[{"label":"Old","value":"stale"}]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordAutoApplyPreviewFailure(ctx, RecordAutoApplyPreviewFailureParams{ID: id, LastError: "boom", MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}

	freshCV := insertCV(t, pool, user)
	if affected, err := q.SetAutoApplyTailoredCV(ctx, SetAutoApplyTailoredCVParams{ID: id, TailoredCvID: &freshCV}); err != nil || affected != 1 {
		t.Fatalf("re-tailor: affected=%d err=%v, want 1", affected, err)
	}

	var resolvedPreview *string
	var previewAttempts int
	var previewFailedAt *time.Time
	if err := pool.QueryRow(ctx, "SELECT resolved_preview, preview_attempts, preview_failed_at FROM auto_apply_queue WHERE id = $1", id).
		Scan(&resolvedPreview, &previewAttempts, &previewFailedAt); err != nil {
		t.Fatal(err)
	}
	if resolvedPreview != nil {
		t.Errorf("resolved_preview = %v, want cleared by the re-tailor", *resolvedPreview)
	}
	if previewAttempts != 0 || previewFailedAt != nil {
		t.Errorf("preview_attempts=%d preview_failed_at=%v, want both reset so the fresh CV gets its own attempt budget",
			previewAttempts, previewFailedAt)
	}

	rows, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 300, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("claimed = %+v, want the re-tailored entry claimable again", rows)
	}
}

// TestSetAutoApplyResolvedPreview_ReleasesTheLease guards a second review finding: without
// releasing claimed_at on success, an approval landing before the preview claim's own lease
// expires would sit unclaimed by the submit pass for up to the full lease window even
// though nothing is still working the row.
func TestSetAutoApplyResolvedPreview_ReleasesTheLease(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "lease-release@example.test")
	job := insertJob(t, pool, "lease-release")
	id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)

	if _, err := pool.Exec(ctx, "UPDATE auto_apply_queue SET claimed_at = now() WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: []byte(`{"fields":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if affected, err := q.ApproveAutoApplyReview(ctx, id); err != nil || affected != 1 {
		t.Fatalf("approve: affected=%d err=%v, want 1", affected, err)
	}

	// A long lease: if claimed_at were still the stale preview-claim timestamp from just
	// now, this claim would find nothing.
	rows, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("claimed = %+v, want the just-approved entry immediately claimable, not blocked by a stale lease", rows)
	}
}
