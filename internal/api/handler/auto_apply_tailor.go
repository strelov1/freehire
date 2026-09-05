package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/platform/db"
)

// This file is freehire's own half of an external, out-of-repository automation pipeline
// (openspec/changes/auto-apply-tailored-resume): the queue-scoped tailoring trigger and the
// candidate's review decision. It deliberately does not touch the interactive tailoring
// surface's own endpoints, middleware, or safety posture — see the change's design.md,
// "why not widen /autopilot".

// autoApplyQueueID parses the :queueId route param.
func autoApplyQueueID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("queueId"), 10, 64)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid queue entry id")
	}
	return id, nil
}

// autoApplyEntry is the subset of an auto_apply_queue row both PostAutoApplyTailor and
// PostAutoApplyReview need, regardless of which of resolveAutoApplyEntry's two reads
// produced it.
type autoApplyEntry struct {
	ID             int64
	UserID         int64
	JobID          int64
	TailoredCvID   *uuid.UUID
	ReviewDecision pgtype.Text
}

// resolveAutoApplyEntry reads one auto-apply queue entry and settles who it belongs to.
//
// A human caller (cookie or a live api_keys row) must OWN the entry: the read itself
// enforces that (GetAutoApplyQueueEntryForReview), so a foreign or missing id both come
// back as pgx.ErrNoRows → 404, never revealing which to a probing caller — unchanged from
// before this function existed.
//
// The trusted auto-apply orchestrator (isAutoApplySystemCaller — see
// auto_apply_orchestrator_auth.go) authenticated as the deployment's own shared secret,
// not as any particular candidate, so it has no ownership claim of its own to check the
// row against. It reads by id alone (GetAutoApplyQueueEntryByID) and acts as whichever
// user the row itself names.
func (h *assistantHandlers) resolveAutoApplyEntry(c *fiber.Ctx, queueID int64) (autoApplyEntry, error) {
	if isAutoApplySystemCaller(c) {
		row, err := h.queries.GetAutoApplyQueueEntryByID(c.Context(), queueID)
		// The generated row is field-for-field identical to autoApplyEntry (same names,
		// types, order) — a direct conversion, not a copy, so the two reads below can
		// never drift apart from what this type actually carries.
		return autoApplyEntry(row), err
	}
	userID, err := requireUserID(c)
	if err != nil {
		return autoApplyEntry{}, err
	}
	row, err := h.queries.GetAutoApplyQueueEntryForReview(c.Context(), db.GetAutoApplyQueueEntryForReviewParams{
		ID: queueID, UserID: userID,
	})
	return autoApplyEntry(row), err
}

// autoApplyTailorResponse is what starting a tailoring run reports: the tailored CV id and
// its per-requirement account of itself, the same report shape the interactive workspace
// shows (cv.AutopilotEntry).
type autoApplyTailorResponse struct {
	TailoredCVID    string              `json:"tailored_cv_id"`
	AutopilotReport []cv.AutopilotEntry `json:"autopilot_report,omitempty"`
}

// PostAutoApplyTailor starts an unattended tailoring run for exactly ONE auto-apply queue
// entry the caller owns, authenticated by API key or cookie (mw.key, matching TailorCV's own
// posture). It bootstraps (or reuses) the tailored CV for the entry's own vacancy — never a
// caller-supplied session id — runs the same unattended autopilot pass the interactive
// workspace's autopilot uses, and responds once the run finishes. This is NOT a variant of
// POST /assistant/sessions/:id/autopilot: that endpoint stays cookie-only, unchanged, and
// this one never accepts a session id at all.
//
// Refuses an entry that does not belong to the caller as 404 (never 403 — mirrors TailorCV's
// own ownership check), and an entry whose review has already been recorded as 409, rather
// than re-tailoring it.
func (h *assistantHandlers) PostAutoApplyTailor(c *fiber.Ctx) error {
	queueID, err := autoApplyQueueID(c)
	if err != nil {
		return err
	}
	if h.runner == nil || h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}

	entry, err := h.resolveAutoApplyEntry(c, queueID)
	if err != nil {
		return err // foreign or missing → pgx.ErrNoRows → 404, per RenderError's own mapping
	}
	userID := entry.UserID
	if entry.ReviewDecision.Valid {
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	job, err := h.queries.GetJob(c.Context(), entry.JobID)
	if err != nil {
		return err
	}

	// The plan-limit gate, called directly rather than through h.meterTurn (design.md):
	// this is exactly TailorCV's own cold-start pre-check, reused as-is — a caller with no
	// tailor allowance left is refused before any CV, session, or model call is made.
	if refused, err := h.cv.refuseNewTailoring(c, userID, job.ID); refused {
		return err
	}

	if err := h.cv.reseedBaseIfStaleVsUpload(c, userID); err != nil {
		log.Printf("auto-apply: base refresh before tailor bootstrap user=%d: %v", userID, err)
	}
	_, tailored, _, err := h.cv.cvStore.Tailor(c.Context(), userID, job.ID, tailoredCVTitle(job.Title), h.cv.seedSource(), skilltag.PreferredFromText(job.Description))
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
	// Same charge TailorCV takes on its own cold start, and for the same reason: only a
	// session THIS request created is charged, so resuming an existing one (an entry
	// re-tailored after adding evidence, say) costs nothing further.
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

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	slot, waiter, err := h.turns.claim(sess.ID, cancel)
	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	if waiter != nil {
		// Unlike streamSSE (a human watching a live stream, worth a queued wait), this is
		// a synchronous API-key call: refusing immediately lets the caller retry later
		// rather than holding the connection open for up to a minute against a run it
		// cannot observe anyway.
		return fiber.NewError(fiber.StatusConflict, "this tailoring session is busy with another run")
	}
	defer h.turns.release(sess.ID, slot)

	analysis := h.prepareAutopilotAnalysis(c, sess, job)
	runner := h.boundRunner(ctx, sess)
	reg := h.registry(sess, uuid.New())
	system := assistant.SystemPrompt(sess.Preset, h.userLanguage(ctx, userID))
	noop := func(assistant.Event) {}

	if err := h.runAutopilotToCompletion(ctx, analysis, sess, job.Description, runner, reg, system, noop); err != nil {
		log.Printf("auto-apply: tailoring run for queue entry %d (cv %s): %v", queueID, tailored.ID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "the tailoring run failed")
	}

	affected, err := h.queries.SetAutoApplyTailoredCV(c.Context(), db.SetAutoApplyTailoredCVParams{
		ID: queueID, TailoredCvID: &tailored.ID,
	})
	if err != nil {
		log.Printf("auto-apply: recording tailored cv for queue entry %d: %v", queueID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "could not record the tailored cv")
	}
	if affected == 0 {
		// The candidate reviewed an earlier tailored CV for this same entry while this
		// (stale or retried) run was still in flight — the guard on the statement itself
		// refused to attach this fresh, never-seen CV to an already-decided entry. No
		// notification, no success response: this run's own output is simply discarded.
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	rec, err := h.cv.cvStore.Get(c.Context(), tailored.ID, userID)
	if err != nil {
		log.Printf("auto-apply: re-reading tailored cv %s after run: %v", tailored.ID, err)
	}

	// Only the run that first attaches a tailored CV to this entry notifies — a retried or
	// resumed call against an entry that already had one (a stale Inngest retry after a
	// timeout, or a deliberate re-tailor once more evidence is added) must not re-send a
	// notification the candidate already got for the same review.
	if entry.TailoredCvID == nil {
		h.notifyTailoredCVReady(c.Context(), userID, job)
	}

	return c.JSON(fiber.Map{"data": autoApplyTailorResponse{
		TailoredCVID: tailored.ID.String(), AutopilotReport: rec.AutopilotReport,
	}})
}

// notifyTailoredCVReady records the in-app notification the candidate sees once a queue
// entry's tailored CV is ready to review, linking into the same tailoring workspace an
// interactive session shows (/tailor/[slug]) — best-effort, the same convention every other
// engine's RecordNotification call already follows (internal/engage/notify, /nudge): a
// failure here must never fail the run it accompanies.
func (h *assistantHandlers) notifyTailoredCVReady(ctx context.Context, userID int64, job db.Job) {
	if _, err := h.queries.RecordNotification(ctx, db.RecordNotificationParams{
		UserID:     userID,
		Kind:       "auto_apply_tailor_ready",
		Title:      "Your tailored CV is ready to review",
		Body:       fmt.Sprintf("We tailored your CV for %s at %s — take a look before it goes out.", job.Title, job.Company),
		PublicSlug: pgtype.Text{String: job.PublicSlug, Valid: true},
	}); err != nil {
		log.Printf("auto-apply: recording tailored-cv-ready notification for user %d: %v", userID, err)
	}
}

// autoApplyReviewRequest is the candidate's decision on one queue entry's tailored CV.
type autoApplyReviewRequest struct {
	Decision string `json:"decision"`
}

const (
	autoApplyReviewApproved = "approved"
	autoApplyReviewDeclined = "declined"
)

// autoApplyDeclineReason is DeclineAutoApplyReview's last_error — distinct from any
// unresolved-form-field park reason, per design.md, so the two are legible apart in the
// queue's own history purely by this text.
const autoApplyDeclineReason = "candidate declined the tailored CV"

// PostAutoApplyReview records the candidate's approve/decline decision for one owned,
// tailored-but-not-yet-reviewed queue entry. Approving marks it eligible for the ATS-
// submission step (internal/application/autoapply.Store.Claim's own predicate); declining
// parks it — through the same fields MarkAutoApplyBlocked sets, with a distinct reason — so
// it is never claimed and never confused with a form-field park. A decision may be recorded
// only once; a second attempt is refused as 409, and the existing decision is unchanged.
func (h *assistantHandlers) PostAutoApplyReview(c *fiber.Ctx) error {
	queueID, err := autoApplyQueueID(c)
	if err != nil {
		return err
	}
	var in autoApplyReviewRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if in.Decision != autoApplyReviewApproved && in.Decision != autoApplyReviewDeclined {
		return fiber.NewError(fiber.StatusBadRequest, `decision must be "approved" or "declined"`)
	}

	entry, err := h.resolveAutoApplyEntry(c, queueID)
	if err != nil {
		return err // foreign or missing → pgx.ErrNoRows → 404
	}
	if entry.TailoredCvID == nil {
		return fiber.NewError(fiber.StatusConflict, "this entry has no tailored cv to review yet")
	}
	if entry.ReviewDecision.Valid {
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	var affected int64
	if in.Decision == autoApplyReviewApproved {
		affected, err = h.queries.ApproveAutoApplyReview(c.Context(), queueID)
	} else {
		affected, err = h.queries.DeclineAutoApplyReview(c.Context(), db.DeclineAutoApplyReviewParams{
			ID: queueID, LastError: autoApplyDeclineReason,
		})
	}
	if err != nil {
		return err
	}
	if affected == 0 {
		// The read above already ruled out "not found" and "already reviewed" for the row
		// it saw — zero rows here means a concurrent decision on this same entry won the
		// race between that read and this write.
		return fiber.NewError(fiber.StatusConflict, "this entry has already been reviewed")
	}

	h.publishReviewDecided(c.Context(), queueID, in.Decision)

	return c.JSON(fiber.Map{"data": fiber.Map{"decision": in.Decision}})
}
