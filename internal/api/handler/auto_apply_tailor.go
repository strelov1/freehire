package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// This file holds the queue-scoped tailoring trigger and the candidate's review decision
// for the auto-apply pipeline. It deliberately leaves the interactive tailoring endpoints,
// middleware and safety posture alone (no widening of /autopilot).

// autoApplyQueueID parses the :queueId route param.
func autoApplyQueueID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid queue entry id")
	}
	return id, nil
}

// autoApplyEntry is the subset of an auto_apply_queue row PostAutoApplyTailor and
// PostAutoApplyReview need, whichever of resolveAutoApplyEntry's reads produced it.
type autoApplyEntry struct {
	ID             int64
	UserID         int64
	JobID          int64
	TailoredCvID   *uuid.UUID
	ReviewDecision pgtype.Text
}

// resolveAutoApplyEntry reads one auto-apply queue entry and settles who it belongs to.
//
// A human caller (cookie or a live api_keys row) must OWN the entry: the read enforces it
// (GetAutoApplyQueueEntryForReview), so a foreign or missing id both give pgx.ErrNoRows
// → 404 without revealing which.
//
// The trusted orchestrator (isAutoApplySystemCaller, auto_apply_orchestrator_auth.go)
// authenticates with the deployment's shared secret, not as a candidate, so it reads by id
// alone (GetAutoApplyQueueEntryByID) and acts as whichever user the row names.
func (h *assistantHandlers) resolveAutoApplyEntry(ctx context.Context, queueID int64) (autoApplyEntry, error) {
	if isAutoApplySystemCaller(ctx) {
		row, err := h.queries.GetAutoApplyQueueEntryByID(ctx, queueID)
		// The generated row is field-for-field identical to autoApplyEntry, so a direct
		// conversion keeps the two reads from drifting apart.
		return autoApplyEntry(row), err
	}
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return autoApplyEntry{}, err
	}
	row, err := h.queries.GetAutoApplyQueueEntryForReview(ctx, db.GetAutoApplyQueueEntryForReviewParams{
		ID: queueID, UserID: userID,
	})
	return autoApplyEntry(row), err
}

// autoApplyTailorBudget bounds ONE unattended tailoring run, end to end.
//
// It must stay shorter than the caller's patience (autoapplyorchestrate.hireRequestTimeout,
// 12 minutes): if the orchestrator hangs up first, the run keeps going and the queue entry
// is never told. Ending here lets the tailored CV be recorded and the entry move to review;
// the runner treats an ended context as a cancellation (assistant.Runner.Run's
// StopCancelled), stopping between rounds with its work committed.
//
// A var so a test can shorten it.
var autoApplyTailorBudget = 10 * time.Minute

// autoApplyTailorResponse reports the tailored CV id and its per-requirement account
// (cv.AutopilotEntry, as in the interactive workspace).
type autoApplyTailorResponse struct {
	TailoredCVID    string              `json:"tailored_cv_id"`
	AutopilotReport []cv.AutopilotEntry `json:"autopilot_report,omitempty"`
}

// PostAutoApplyTailor starts an unattended tailoring run for exactly ONE auto-apply queue
// entry the caller owns (API key or cookie, mw.key like TailorCV). It bootstraps or reuses
// the tailored CV for the entry's own vacancy, never a caller-supplied session id, runs the
// interactive autopilot pass, and responds when it finishes. NOT a variant of
// POST /assistant/sessions/:id/autopilot, which stays cookie-only.
//
// A foreign entry is 404 (never 403, like TailorCV); one already reviewed is 409.
func (h *assistantHandlers) PostAutoApplyTailor(c *fiber.Ctx) error {
	queueID, err := autoApplyQueueID(c.Params("queueId"))
	if err != nil {
		return err
	}
	if h.runner == nil || h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}

	entry, err := h.resolveAutoApplyEntry(c.Context(), queueID)
	if err != nil {
		return err // foreign or missing → pgx.ErrNoRows → 404, per RenderError's own mapping
	}
	userID := entry.UserID
	if entry.ReviewDecision.Valid {
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}
	// The paid gate lives here, not in route middleware: the user this run spends for is the
	// entry's OWNER, known only once the entry is read. A lapsed subscriber gets no
	// unattended tailoring. Deliberately NOT recorded as a paywall hit: nobody pressed a paid button.
	if ok, err := authorizePaidAIUnrecorded(c, entitlementOf(h.plans), userID, plan.FeatureTailor); !ok {
		return err
	}

	job, err := h.queries.GetJob(c.Context(), entry.JobID)
	if err != nil {
		return err
	}

	// The plan-limit gate is TailorCV's cold-start pre-check, called directly rather than
	// via h.meterTurn: no allowance means refusal before any CV, session or model call.
	if refused, err := h.cv.refuseNewTailoring(c, userID, job.ID); refused {
		return err
	}

	if err := h.cv.reseedBaseIfStaleVsUpload(c.Context(), userID, uuid.Nil); err != nil {
		log.Printf("auto-apply: base refresh before tailor bootstrap user=%d: %v", userID, err)
	}
	_, tailored, _, err := h.cv.cvStore.Tailor(c.Context(), userID, job.ID, uuid.Nil, tailoredCVTitle(job.Title), h.cv.seedSource(), skilltag.PreferredFromText(job.Description))
	if errors.Is(err, cv.ErrNoResume) {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before tailoring")
	}
	if err != nil {
		return mapCVError(err)
	}

	sessionID, err := h.cv.existingTailoringSession(c.Context(), userID, tailored.ID)
	if err != nil {
		return err
	}
	justStarted := sessionID == ""
	if justStarted {
		sessionID, err = h.cv.startTailoringSession(c.Context(), userID, tailored.ID, job.ID)
		if err != nil {
			return err
		}
	}
	// Same charge as TailorCV's cold start: only a session THIS request created is charged,
	// so resuming one costs nothing further.
	if justStarted {
		if _, err := h.plans.StartSession(c.Context(), userID, sessionID); err != nil {
			log.Printf("plan: charging a tailoring session (auto-apply) user=%d session=%s: %v", userID, sessionID, err)
		}
	}

	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("parse minted tailoring session id: %w", err)
	}
	sess, err := h.store.Session(c.Context(), sessUUID, userID)
	if err != nil {
		return mapAssistantError(err)
	}

	ctx, cancel := context.WithTimeout(c.Context(), autoApplyTailorBudget)
	defer cancel()
	// tryClaim, not claim: this is a synchronous API-key call, so refuse immediately and
	// let the caller retry. claim would queue a place in LINE this handler cannot give
	// back, which wedges the session (see tryClaim's doc).
	slot, ok := h.turns.tryClaim(sess.ID, cancel)
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "this tailoring session is busy with another run")
	}
	defer h.turns.release(sess.ID, slot)

	analysis, err := h.prepareAutopilotAnalysis(c, sess, job)
	if err != nil {
		return err
	}
	runner, err := h.boundRunner(c.UserContext(), sess)
	if err != nil {
		return err
	}
	reg := h.registry(sess, uuid.New())
	system := assistant.SystemPrompt(sess.Preset, h.userLanguage(ctx, userID))
	noop := func(assistant.Event) {}

	if err := h.runAutopilotToCompletion(ctx, analysis, sess, job.Description, runner, reg, system, noop); err != nil {
		log.Printf("auto-apply: tailoring run for queue entry %d (cv %s): %v", queueID, tailored.ID, err)
		h.recordTailoringFailure(c, queueID, err)
		// Return the CAUSE, not a phrased 500: the streamed autopilot reports the same error
		// (reportStreamFault in assistant.go). Unwrapped so classify still sees
		// context.Canceled (orchestrator hung up) as a 499, not a fault.
		return err
	}

	affected, err := h.queries.SetAutoApplyTailoredCV(c.Context(), db.SetAutoApplyTailoredCVParams{
		ID: queueID, TailoredCvID: &tailored.ID,
	})
	if err != nil {
		log.Printf("auto-apply: recording tailored cv for queue entry %d: %v", queueID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "could not record the tailored cv")
	}
	if affected == 0 {
		// The candidate reviewed an earlier CV while this stale or retried run was in flight;
		// the statement's guard refused to attach this one. Output discarded, no notification.
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	rec, err := h.cv.cvStore.Get(c.Context(), tailored.ID, userID)
	if err != nil {
		log.Printf("auto-apply: re-reading tailored cv %s after run: %v", tailored.ID, err)
	}

	// No notification here: cmd/auto-apply's preview pass notifies once the answer preview is ready.

	return c.JSON(fiber.Map{"data": autoApplyTailorResponse{
		TailoredCVID: tailored.ID.String(), AutopilotReport: rec.AutopilotReport,
	}})
}

// autoApplyTailorFailedNotification is the kind for a tailoring run that gave up. Separate
// from autoApplyReadyForReviewNotification because the centre groups by kind.
const autoApplyTailorFailedNotification = "auto_apply_tailoring_failed"

// recordTailoringFailure marks an entry whose run produced no CV, and tells the candidate.
//
// Best-effort: the caller is already returning the run's failure. It must not fail silently
// by leaving the entry looking in progress.
//
// Runs on a context detached from the request's: the caller giving up first is the usual
// failure, so the request context is often dead (as with runAutopilotToCompletion's refresh).
//
// The error text is a stored diagnostic, never shown to the candidate (ResolvedAttempt has
// no field for it); they see the status and the notification.
func (h *assistantHandlers) recordTailoringFailure(c *fiber.Ctx, queueID int64, cause error) {
	ctx := context.WithoutCancel(c.Context())

	row, err := h.queries.MarkAutoApplyTailorFailed(ctx, db.MarkAutoApplyTailorFailedParams{
		ID: queueID, LastError: cause.Error(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A guard fired: the entry already has a tailored CV or a decision; nothing to
			// report (see MarkAutoApplyTailorFailed).
			return
		}
		log.Printf("auto-apply: recording the tailoring failure for queue entry %d: %v", queueID, err)
		return
	}

	if _, err := h.queries.RecordNotification(ctx, db.RecordNotificationParams{
		UserID: row.UserID,
		Kind:   autoApplyTailorFailedNotification,
		Title:  "We couldn't prepare your application",
		Body: fmt.Sprintf("Tailoring your CV for %s at %s didn't finish. Nothing was sent, and the job is still on your board.",
			row.Title, row.Company),
		PublicSlug: pgconv.TextValid(row.PublicSlug),
	}); err != nil {
		// Best-effort, the same convention every other RecordNotification call follows.
		log.Printf("auto-apply: notifying the tailoring failure for queue entry %d: %v", queueID, err)
	}
}

const (
	autoApplyReviewApproved = "approved"
	autoApplyReviewDeclined = "declined"
)

// autoApplyDeclineReason is DeclineAutoApplyReview's last_error, distinct from any
// form-field park reason so the two read apart in the queue history.
const autoApplyDeclineReason = "candidate declined the tailored CV"

// PostAutoApplyReview records the candidate's approve/decline decision for one owned,
// tailored-but-not-yet-reviewed entry. Approving makes it eligible for submission
// (autoapply.Store.Claim's predicate); declining parks it via the MarkAutoApplyBlocked
// fields with a distinct reason, so it is never claimed. A decision is recorded only once
// (second attempt 409, decision unchanged).
func (h *assistantHandlers) PostAutoApplyReview(ctx context.Context, in *apigen.AutoApplyReviewRequest, params apigen.PostAutoApplyReviewParams) (*apigen.PostAutoApplyReviewOK, error) {
	queueID, err := autoApplyQueueID(params.QueueId)
	if err != nil {
		return nil, err
	}
	entry, err := h.resolveAutoApplyEntry(ctx, queueID)
	if err != nil {
		return nil, err // foreign or missing → pgx.ErrNoRows → 404
	}
	decision := in.Decision.Value
	if err := h.decideAutoApplyReview(ctx, entry, decision); err != nil {
		return nil, err
	}
	return &apigen.PostAutoApplyReviewOK{Data: apigen.AutoApplyDecision{Decision: decision}}, nil
}

// decideAutoApplyReview records decision ("approved" or "declined") for an entry the caller
// has already resolved and owns, then wakes the orchestrator. The route and the MCP tool both
// call it. A decision may be recorded only once; a second attempt is refused as 409 and the
// existing decision is unchanged.
func (h *assistantHandlers) decideAutoApplyReview(ctx context.Context, entry autoApplyEntry, decision string) error {
	if decision != autoApplyReviewApproved && decision != autoApplyReviewDeclined {
		return fiber.NewError(fiber.StatusBadRequest, `decision must be "approved" or "declined"`)
	}
	if entry.TailoredCvID == nil {
		return fiber.NewError(fiber.StatusConflict, "this entry has no tailored cv to review yet")
	}
	if entry.ReviewDecision.Valid {
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	var affected int64
	var err error
	if decision == autoApplyReviewApproved {
		affected, err = h.queries.ApproveAutoApplyReview(ctx, entry.ID)
	} else {
		affected, err = h.queries.DeclineAutoApplyReview(ctx, db.DeclineAutoApplyReviewParams{
			ID: entry.ID, LastError: autoApplyDeclineReason,
		})
	}
	if err != nil {
		return err
	}
	if affected == 0 {
		// The caller's read ruled out "not found" and "already reviewed", so zero rows means
		// a concurrent decision won the race.
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	h.publishReviewDecided(ctx, entry.ID, decision)
	return nil
}
