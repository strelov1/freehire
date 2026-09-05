package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyEnqueueSource is the only ATS this feature resolves a résumé upload field for
// today (openspec/changes/auto-apply-tailored-resume's own scope decision) — enqueuing for
// any other source would create an attempt that can tailor and get reviewed but can never
// actually submit. See openspec/changes/auto-apply-submit-trigger.
const autoApplyEnqueueSource = "greenhouse"

// autoApplyAlreadyDeclinedMessage is what a permanently-declined pair answers with —
// distinct wording from autoApplyDeclineReason (auto_apply_tailor.go), which is the stored
// last_error, not a response body.
const autoApplyAlreadyDeclinedMessage = "this auto-apply attempt was already declined"

// autoApplyStatusQueued is the wire status for a live, undecided auto-apply entry —
// shared between this endpoint's own success response and GetJob's status overlay
// (jobs.go), so the two can never drift apart on what "queued" is spelled as.
const autoApplyStatusQueued = "queued"

// autoApplyStatusFailed is the wire status for an entry cmd/auto-apply gave up on:
// dead-lettered after exhausting its retries (RecordAutoApplyFailure's own failed_at) or
// parked because the ATS form needed data we don't have (MarkAutoApplyBlocked's own
// blocked_at). Both leave review_decision at 'approved' (only an approved entry is ever
// claimed for submission), so without checking these two columns a permanently stuck
// attempt reads exactly like a healthy one still in flight — a code review found this: the
// candidate would see "queued" forever, with no way to learn the attempt died.
const autoApplyStatusFailed = "failed"

// autoApplyEntryStatus derives the wire status EnqueueAutoApply's idempotent path and
// GetJob's overlay both report, from the same three columns
// GetAutoApplyQueueEntryForJob reads. Declined is checked first: DeclineAutoApplyReview
// also sets blocked_at (it reuses MarkAutoApplyBlocked's own park vocabulary), so checking
// failed/blocked before review_decision would misreport the candidate's own decline as an
// operational failure.
func autoApplyEntryStatus(reviewDecision pgtype.Text, failedAt, blockedAt pgtype.Timestamptz) string {
	if reviewDecision.Valid && reviewDecision.String == autoApplyReviewDeclined {
		return autoApplyReviewDeclined
	}
	if failedAt.Valid || blockedAt.Valid {
		return autoApplyStatusFailed
	}
	return autoApplyStatusQueued
}

// autoApplyQueuedResponse is the success body for both a fresh enqueue and an idempotent
// repeat against an existing, undecided entry — the caller cannot (and need not) tell
// the two apart.
func autoApplyQueuedResponse(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": fiber.Map{"status": autoApplyStatusQueued}})
}

// PostJobAutoApply is the candidate-facing trigger auto-apply-tailored-resume and
// auto-apply-inngest-orchestration both assumed would exist: it creates one durable
// auto-apply attempt for the caller and the named job, starting the already-built
// tailor-then-review sequence. Cookie-only (mw.cookie, never mw.key) — the same "the
// browser is the only place the candidate can watch it happen and undo it" reasoning
// /autopilot already uses, at higher stakes here: a fresh entry can end in a REAL
// submitted job application, not only an unattended CV rewrite.
//
// Gates, in order: the job must be a Greenhouse posting, the caller must be on the PRO
// plan tier, and the caller must already have a base CV. A repeat request for a pair that
// already has a live, undecided entry succeeds without creating a second row; a repeat for
// a pair whose entry was already declined by the candidate is refused permanently.
func (h *assistantHandlers) PostJobAutoApply(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // pgx.ErrNoRows -> 404, per RenderError's own mapping
	}
	if job.Source != autoApplyEnqueueSource {
		return fiber.NewError(fiber.StatusBadRequest, "auto-apply is only available for Greenhouse postings today")
	}

	proUntil, err := h.queries.GetProUntil(c.Context(), userID)
	if err != nil {
		return err
	}
	if plan.TierOf(proUntil.Time, time.Now()) != plan.TierPro {
		return fiber.NewError(fiber.StatusPaymentRequired, "auto-apply is a PRO feature")
	}

	if h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	if _, ok, err := h.cv.cvStore.BaseCV(c.Context(), userID); err != nil {
		return err
	} else if !ok {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before using auto-apply")
	}

	// cmd/auto-apply/store.go's Submit deletes the queue row in the same transaction it
	// marks the job applied (applications.applied_at, via MarkJobApplied), so a completed
	// attempt leaves no queue row behind for EnqueueAutoApply's own ON CONFLICT to catch —
	// without this check a re-click after a real submission would start a second one.
	if applied, err := h.queries.GetUserJobApplied(c.Context(), db.GetUserJobAppliedParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	}); err != nil {
		return err
	} else if applied {
		return fiber.NewError(fiber.StatusConflict, "already applied to this job")
	}

	id, err := h.queries.EnqueueAutoApply(c.Context(), db.EnqueueAutoApplyParams{UserID: userID, JobID: job.ID})
	switch {
	case err == nil:
		// This request's own INSERT won the race — it is the only caller that may ever
		// publish for this entry (see design.md: publishing on every request, including
		// an idempotent replay that touched no row, would let the orchestrator's executor
		// see the same auto-apply/submit more than once for one entry).
		h.publishSubmit(c.Context(), id)
		return autoApplyQueuedResponse(c)
	case errors.Is(err, pgx.ErrNoRows):
		existing, ferr := h.queries.GetAutoApplyQueueEntryForJob(c.Context(), db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		})
		if ferr != nil {
			return ferr
		}
		switch autoApplyEntryStatus(existing.ReviewDecision, existing.FailedAt, existing.BlockedAt) {
		case autoApplyReviewDeclined:
			return fiber.NewError(fiber.StatusConflict, autoApplyAlreadyDeclinedMessage)
		case autoApplyStatusFailed:
			// cmd/auto-apply gave up on this entry (dead-lettered or parked on an
			// unresolvable form field) — reporting "queued" here would tell the
			// candidate their attempt is still in flight when it is permanently stuck.
			return fiber.NewError(fiber.StatusConflict, "this auto-apply attempt could not be completed")
		default:
			// A live, undecided entry already exists (the common "page reload, a second
			// tab" case per design.md) — reporting success without a second row is the
			// intended idempotent behavior, not a fault to log.
			return autoApplyQueuedResponse(c)
		}
	default:
		return err
	}
}
