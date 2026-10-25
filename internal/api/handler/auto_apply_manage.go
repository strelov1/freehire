package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Retry and cancel for an auto-apply attempt (openspec/changes/auto-apply-over-mcp). Both are
// addressed by the job's slug, like the start route beside them: a queue id dies on submission
// and on cancel, while (caller, job) is the queue's own unique key and is what both the
// tracker drawer and an agent already hold.

// autoApplyNoAttemptMessage answers a retry or cancel for a job the caller has no attempt on.
const autoApplyNoAttemptMessage = "there is no auto-apply attempt for this job"

// autoApplyEntryForSlug resolves the caller's attempt on the job behind slug. An unknown slug
// is pgx.ErrNoRows (404 over HTTP); a known job with no attempt is a 404 that says so.
func (h *assistantHandlers) autoApplyEntryForSlug(ctx context.Context, userID int64, slug string) (db.Job, db.GetAutoApplyQueueEntryForJobRow, error) {
	job, err := h.queries.GetJobBySlug(ctx, slug)
	if err != nil {
		return db.Job{}, db.GetAutoApplyQueueEntryForJobRow{}, err
	}
	row, err := h.queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{UserID: userID, JobID: job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return job, row, fiber.NewError(fiber.StatusNotFound, autoApplyNoAttemptMessage)
	}
	return job, row, err
}

// retryAutoApply re-arms the caller's stuck attempt on the job behind slug — a park, a dead
// letter, or a tailoring failure — or refreshes the answer preview of one awaiting review, so
// an answer just added to the bank shows up in what is being approved.
//
// An attempt with no tailored CV yet has nothing the worker can pick up on its own: the
// orchestrator that tailors it ran and ended, so it is started again.
func (h *assistantHandlers) retryAutoApply(ctx context.Context, userID int64, slug string) error {
	_, entry, err := h.autoApplyEntryForSlug(ctx, userID, slug)
	if err != nil {
		return err
	}
	row, err := h.queries.RetryAutoApplyEntry(ctx, db.RetryAutoApplyEntryParams{ID: entry.ID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusConflict, autoApplyNotRetryableReason(entry))
	}
	if err != nil {
		return err
	}
	if !row.TailoredCvID.Valid {
		h.publishSubmit(ctx, row.ID)
	}
	return nil
}

// autoApplyUncertainMessage refuses both retry and cancel of an attempt the employer may
// already hold: either would open the way to sending it twice.
const autoApplyUncertainMessage = "this application may already have reached the employer, so it is not sent again — check with the employer before applying another way"

// autoApplyNotRetryableReason says why RetryAutoApplyEntry declined the row the caller read.
// A concurrent change between that read and the write lands on the last case, which is still
// true of the row as it now stands.
func autoApplyNotRetryableReason(entry db.GetAutoApplyQueueEntryForJobRow) string {
	switch {
	case entry.ReviewDecision.Valid && entry.ReviewDecision.String == autoApplyReviewDeclined:
		return "this attempt was declined; cancel it and start a new one to apply after all"
	case entry.SubmitUncertainAt.Valid:
		return autoApplyUncertainMessage
	default:
		return "this attempt is not stuck, so there is nothing to retry"
	}
}

// cancelAutoApply withdraws the caller's attempt on the job behind slug, unless the worker
// may be submitting it at this moment — that window is refused rather than raced, because the
// browser driver cannot be called back.
//
// The orchestrator run that is waiting for this attempt's review decision is told, so it ends
// now rather than at its seven-day timeout.
func (h *assistantHandlers) cancelAutoApply(ctx context.Context, userID int64, slug string) error {
	_, entry, err := h.autoApplyEntryForSlug(ctx, userID, slug)
	if err != nil {
		return err
	}
	_, err = h.queries.CancelAutoApplyEntry(ctx, db.CancelAutoApplyEntryParams{
		ID: entry.ID, UserID: userID, LeaseSeconds: int64(config.LoadAutoApply().LeaseSeconds),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if entry.SubmitUncertainAt.Valid {
			return fiber.NewError(fiber.StatusConflict, autoApplyUncertainMessage)
		}
		return fiber.NewError(fiber.StatusConflict, "this application is being submitted right now and can no longer be cancelled")
	}
	if err != nil {
		return err
	}
	if !entry.ReviewDecision.Valid {
		h.publishReviewDecided(ctx, entry.ID, autoApplyCancelled)
	}
	return nil
}

// autoApplyCancelled is the decision an orchestrator run waiting on a review is told when the
// attempt it waits for was withdrawn instead. The run only records it.
const autoApplyCancelled = "cancelled"

// PostJobAutoApplyRetry is retryAutoApply over HTTP.
func (h *assistantHandlers) PostJobAutoApplyRetry(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.retryAutoApply(c.Context(), userID, c.Params("slug")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"retried": true}})
}

// DeleteJobAutoApply is cancelAutoApply over HTTP.
func (h *assistantHandlers) DeleteJobAutoApply(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.cancelAutoApply(c.Context(), userID, c.Params("slug")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"cancelled": true}})
}
