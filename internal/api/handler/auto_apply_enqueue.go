package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyEnqueueSource is the only ATS this feature resolves a résumé upload field for
// today (openspec/changes/auto-apply-tailored-resume's own scope decision) — enqueuing for
// any other source would create an attempt that can tailor and get reviewed but can never
// actually submit. See openspec/changes/auto-apply-submit-trigger.
const autoApplyEnqueueSource = "greenhouse"

// autoApplyDeclinedResponse is what a permanently-declined pair answers with — distinct
// wording from autoApplyDeclineReason (auto_apply_tailor.go), which is the stored
// last_error, not a response body.
const autoApplyAlreadyDeclinedMessage = "this auto-apply attempt was already declined"

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

	id, err := h.queries.EnqueueAutoApply(c.Context(), db.EnqueueAutoApplyParams{UserID: userID, JobID: job.ID})
	switch {
	case err == nil:
		// This request's own INSERT won the race — it is the only caller that may ever
		// publish for this entry (see design.md: publishing on every request, including
		// an idempotent replay that touched no row, would let the orchestrator's executor
		// see the same auto-apply/submit more than once for one entry).
		h.publishSubmit(c.Context(), id)
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "queued"}})
	case errors.Is(err, pgx.ErrNoRows):
		existing, ferr := h.queries.GetAutoApplyQueueEntryForJob(c.Context(), db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		})
		if ferr != nil {
			return ferr
		}
		if existing.ReviewDecision.Valid && existing.ReviewDecision.String == autoApplyReviewDeclined {
			return fiber.NewError(fiber.StatusConflict, autoApplyAlreadyDeclinedMessage)
		}
		// A live, undecided entry already exists (the common "page reload, a second
		// tab" case per design.md) — reporting success without a second row is the
		// intended idempotent behavior, not a fault to log.
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "queued"}})
	default:
		return err
	}
}
