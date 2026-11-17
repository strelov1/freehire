//go:build integration

package handler

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestPostAutoApplyReview_ForeignEntryIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	owner, _ := autoApplyTailorUser(t, pool, iss, "rowner@example.test")
	_, otherCookie := autoApplyTailorUser(t, pool, iss, "rother@example.test")
	job := insertAutoApplyJob(t, pool, "review-foreign")
	queueID := insertAutoApplyQueueRow(t, pool, owner, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", otherCookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a foreign queue entry", resp.StatusCode)
	}
}

func TestPostAutoApplyReview_NoTailoredCVYetIsRefused(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	userID, cookie := autoApplyTailorUser(t, pool, iss, "notailored@example.test")
	job := insertAutoApplyJob(t, pool, "review-no-cv")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 for an entry with no tailored cv yet", resp.StatusCode)
	}
}

// setTailoredCV seeds a tailored CV and points the queue row at it directly — the review
// endpoint's own precondition, reached here without running a real tailoring pass.
func setTailoredCV(t *testing.T, pool *pgxpool.Pool, userID, jobID, queueID int64) uuid.UUID {
	t.Helper()
	var cvID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored)
		 VALUES ($1, 'Tailored', 'classic-ats', '{}'::jsonb, $2, true) RETURNING id`,
		userID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed tailored cv: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE auto_apply_queue SET tailored_cv_id = $1 WHERE id = $2", cvID, queueID); err != nil {
		t.Fatalf("attach tailored cv to queue row: %v", err)
	}
	return cvID
}

func TestPostAutoApplyReview_ApprovingMakesTheEntryClaimable(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})
	queries := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "approve@example.test")
	job := insertAutoApplyJob(t, pool, "review-approve")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	claimed, err := queries.ClaimAutoApplyBatch(context.Background(), db.ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != queueID {
		t.Fatalf("claimed = %+v, want the approved entry", claimed)
	}
}

func TestPostAutoApplyReview_DecliningParksAndExcludesFromClaim(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})
	queries := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "decline@example.test")
	job := insertAutoApplyJob(t, pool, "review-decline")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "declined"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var blockedAt any
	var lastError string
	if err := pool.QueryRow(context.Background(),
		"SELECT blocked_at, last_error FROM auto_apply_queue WHERE id = $1", queueID).
		Scan(&blockedAt, &lastError); err != nil {
		t.Fatal(err)
	}
	if blockedAt == nil {
		t.Error("blocked_at not set on decline")
	}
	if lastError != autoApplyDeclineReason {
		t.Errorf("last_error = %q, want the distinct decline reason", lastError)
	}

	claimed, err := queries.ClaimAutoApplyBatch(context.Background(), db.ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed = %+v, want none — a declined entry is never claimed", claimed)
	}
}

func TestPostAutoApplyReview_DecisionCannotBeRecordedTwice(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	userID, cookie := autoApplyTailorUser(t, pool, iss, "twice@example.test")
	job := insertAutoApplyJob(t, pool, "review-twice")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	first := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first review status = %d, want 200", first.StatusCode)
	}

	second := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "declined"})
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusConflict {
		t.Fatalf("second review status = %d, want 409", second.StatusCode)
	}

	var decision string
	if err := pool.QueryRow(context.Background(),
		"SELECT review_decision FROM auto_apply_queue WHERE id = $1", queueID).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if decision != "approved" {
		t.Errorf("review_decision = %q, want the first decision to stand unchanged", decision)
	}
}
