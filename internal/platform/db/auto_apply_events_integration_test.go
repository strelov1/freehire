//go:build integration

// Integration tests for the auto-apply history (migration 0196) and the retry/cancel
// queries built on it (openspec/changes/auto-apply-over-mcp).
package db

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func autoApplyEventKinds(t *testing.T, q *Queries, userID, jobID int64) []string {
	t.Helper()
	rows, err := q.ListAutoApplyEvents(context.Background(), ListAutoApplyEventsParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		kinds = append(kinds, rows[i].Kind)
	}
	return kinds
}

func TestAutoApplyEvents_RecordTheLifecycle(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "events@example.test")
	job := insertJob(t, pool, "events")
	id, err := q.EnqueueAutoApply(ctx, EnqueueAutoApplyParams{UserID: user, JobID: job})
	if err != nil {
		t.Fatal(err)
	}
	cv := insertCV(t, pool, user)
	if _, err := q.SetAutoApplyTailoredCV(ctx, SetAutoApplyTailoredCVParams{ID: id, TailoredCvID: &cv}); err != nil {
		t.Fatal(err)
	}
	preview := []byte(`{"fields":[],"pending":[{"label":"Why us?","will_draft_at_submission":false}]}`)
	if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: preview}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ApproveAutoApplyReview(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RecordAutoApplyFailure(ctx, RecordAutoApplyFailureParams{ID: id, LastError: "secret diagnostic", MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	unmapped, _ := json.Marshal([]map[string]any{{"id": "q1", "label": "Why us?", "required": true, "reason": "no known answer"}})
	if err := q.MarkAutoApplyBlocked(ctx, MarkAutoApplyBlockedParams{ID: id, LastError: "parked", Unmapped: unmapped}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: user}); err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteAutoApplyEntry(ctx, id); err != nil {
		t.Fatal(err)
	}

	want := []string{"queued", "tailored", "ready_for_review", "approved", "submitting", "attempt_failed", "blocked", "retried", "submitted"}
	if got := autoApplyEventKinds(t, q, user, job); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}

	rows, _ := q.ListAutoApplyEvents(ctx, ListAutoApplyEventsParams{UserID: user, JobID: job})
	for _, r := range rows {
		if strings.Contains(string(r.Detail), "secret diagnostic") {
			t.Errorf("event %s leaks last_error: %s", r.Kind, r.Detail)
		}
		if r.Kind == "blocked" && !strings.Contains(string(r.Detail), "Why us?") {
			t.Errorf("blocked detail = %s, want the question label", r.Detail)
		}
		if r.Kind == "ready_for_review" && !strings.Contains(string(r.Detail), "Why us?") {
			t.Errorf("ready_for_review detail = %s, want the pending label", r.Detail)
		}
	}

	subs, err := q.ListRecentAutoApplySubmissions(ctx, user)
	if err != nil || len(subs) != 1 || subs[0].JobID != job {
		t.Fatalf("recent submissions = %+v err=%v, want the one job", subs, err)
	}
}

func TestAutoApplyEvents_DeclineIsNotABlock(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "decline@example.test")
	job := insertJob(t, pool, "decline")
	id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)
	if _, err := q.DeclineAutoApplyReview(ctx, DeclineAutoApplyReviewParams{ID: id, LastError: "declined"}); err != nil {
		t.Fatal(err)
	}
	if got := autoApplyEventKinds(t, q, user, job); !slices.Equal(got, []string{"queued", "declined"}) {
		t.Fatalf("events = %v, want [queued declined]", got)
	}
}

// A park that names no field stores unmapped as JSON null (a nil Go slice); the trigger must
// not choke on it, or the park itself fails and the row is retried forever.
func TestAutoApplyEvents_ParkWithNoFields(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateAutoApplyQueue(t, pool)

	user := insertUser(t, pool, "nullpark@example.test")
	job := insertJob(t, pool, "nullpark")
	id := insertAutoApplyQueueEntry(t, pool, user, job)
	if err := q.MarkAutoApplyBlocked(ctx, MarkAutoApplyBlockedParams{ID: id, LastError: "captcha", Unmapped: []byte("null")}); err != nil {
		t.Fatalf("park with JSON null: %v", err)
	}
	if _, err := q.DeclineAutoApplyReview(ctx, DeclineAutoApplyReviewParams{ID: id, LastError: "x"}); err != nil {
		t.Fatalf("a later write to the same row: %v", err)
	}
	if got := autoApplyEventKinds(t, q, user, job); !slices.Equal(got, []string{"queued", "blocked"}) {
		t.Fatalf("events = %v, want [queued blocked]", got)
	}
}

func TestRetryAutoApplyEntry(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	setup := func(t *testing.T) (user, job, id int64) {
		truncateAutoApplyQueue(t, pool)
		user = insertUser(t, pool, "retry@example.test")
		job = insertJob(t, pool, "retry")
		return user, job, insertAutoApplyQueueEntry(t, pool, user, job)
	}

	t.Run("a dead letter is re-armed with a fresh budget and claimable again", func(t *testing.T) {
		user, _, id := setup(t)
		for range 3 {
			if _, err := q.RecordAutoApplyFailure(ctx, RecordAutoApplyFailureParams{ID: id, LastError: "x", MaxAttempts: 3}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: user}); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim after retry: rows=%d err=%v, want 1", len(claimed), err)
		}
	})

	t.Run("an uncertain submission is never re-armed", func(t *testing.T) {
		user, _, id := setup(t)
		if err := q.MarkAutoApplySubmitUncertain(ctx, MarkAutoApplySubmitUncertainParams{ID: id, LastError: "unconfirmed"}); err != nil {
			t.Fatal(err)
		}
		_, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: user})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("retry of an uncertain submission: err=%v, want ErrNoRows", err)
		}
	})

	t.Run("a healthy approved entry is not retryable", func(t *testing.T) {
		user, _, id := setup(t)
		_, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: user})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err=%v, want ErrNoRows", err)
		}
	})

	t.Run("another account's entry is not retryable", func(t *testing.T) {
		_, _, id := setup(t)
		other := insertUser(t, pool, "other@example.test")
		if err := q.MarkAutoApplyBlocked(ctx, MarkAutoApplyBlockedParams{ID: id, LastError: "p", Unmapped: []byte("[]")}); err != nil {
			t.Fatal(err)
		}
		_, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: other})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err=%v, want ErrNoRows", err)
		}
	})

	t.Run("a pending review is refreshed: its preview is dropped for the preview pass", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "refresh@example.test")
		job := insertJob(t, pool, "refresh")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)
		if _, err := q.SetAutoApplyResolvedPreview(ctx, SetAutoApplyResolvedPreviewParams{ID: id, ResolvedPreview: []byte(`{"fields":[]}`)}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.RetryAutoApplyEntry(ctx, RetryAutoApplyEntryParams{ID: id, UserID: user}); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("preview claim after refresh: rows=%d err=%v, want 1", len(claimed), err)
		}
	})
}

func TestCancelAutoApplyEntry(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	setup := func(t *testing.T) (user, job, id int64) {
		truncateAutoApplyQueue(t, pool)
		user = insertUser(t, pool, "cancel@example.test")
		job = insertJob(t, pool, "cancel")
		return user, job, insertAutoApplyQueueEntry(t, pool, user, job)
	}

	t.Run("refused while the submit pass holds the lease", func(t *testing.T) {
		user, _, id := setup(t)
		if _, err := q.ClaimAutoApplyBatch(ctx, ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		_, err := q.CancelAutoApplyEntry(ctx, CancelAutoApplyEntryParams{ID: id, UserID: user, LeaseSeconds: 3600})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err=%v, want ErrNoRows", err)
		}
	})

	t.Run("allowed while the preview pass holds the lease", func(t *testing.T) {
		truncateAutoApplyQueue(t, pool)
		user := insertUser(t, pool, "cancel-preview@example.test")
		job := insertJob(t, pool, "cancel-preview")
		id := insertTailoredAutoApplyQueueEntry(t, pool, user, job)
		if _, err := q.ClaimAutoApplyPreviewBatch(ctx, ClaimAutoApplyPreviewBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.CancelAutoApplyEntry(ctx, CancelAutoApplyEntryParams{ID: id, UserID: user, LeaseSeconds: 3600}); err != nil {
			t.Fatalf("cancel during a preview claim: %v", err)
		}
	})

	t.Run("never deletes an uncertain submission", func(t *testing.T) {
		user, _, id := setup(t)
		if err := q.MarkAutoApplySubmitUncertain(ctx, MarkAutoApplySubmitUncertainParams{ID: id, LastError: "unconfirmed"}); err != nil {
			t.Fatal(err)
		}
		_, err := q.CancelAutoApplyEntry(ctx, CancelAutoApplyEntryParams{ID: id, UserID: user, LeaseSeconds: 3600})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err=%v, want ErrNoRows", err)
		}
	})

	t.Run("deletes and records cancelled otherwise", func(t *testing.T) {
		user, job, id := setup(t)
		if _, err := q.CancelAutoApplyEntry(ctx, CancelAutoApplyEntryParams{ID: id, UserID: user, LeaseSeconds: 3600}); err != nil {
			t.Fatal(err)
		}
		got := autoApplyEventKinds(t, q, user, job)
		if got[len(got)-1] != "cancelled" {
			t.Fatalf("events = %v, want cancelled last", got)
		}
		if subs, _ := q.ListRecentAutoApplySubmissions(ctx, user); len(subs) != 0 {
			t.Errorf("a cancel listed as a submission: %+v", subs)
		}
	})
}
