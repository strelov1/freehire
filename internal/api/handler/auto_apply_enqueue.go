package handler

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyEnqueueSources are the ATS providers auto-apply can queue an attempt against at
// all. Only Greenhouse can currently SUBMIT one (fillProviders in internal/api/atsapply) —
// an Ashby/Workable/Lever/Recruitee attempt still tailors and gets reviewed like any other,
// then parks with an honest, named reason once cmd/auto-apply reaches it, rather than being
// refused at the door. Widening this list is a product decision (a candidate spends a real
// daily tailoring turn on an attempt that is not yet guaranteed to submit), not a technical
// one — see openspec/changes/auto-apply-submit-trigger.
var autoApplyEnqueueSources = map[string]bool{
	"greenhouse": true,
	"ashby":      true,
	"workable":   true,
	"lever":      true,
	"recruitee":  true,
}

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

// autoApplyEligibilityEnforce gates whether an eligibility mismatch (see
// eligibilityBlocker) actually refuses enqueueing or only logs what would have been
// refused. Ships OFF (shadow-only) by default, mirroring internal/ai/plan's PLAN_ENFORCE
// rollout: a false positive here has an immediate, visible cost — a paying candidate who
// cannot auto-apply to a job they are, in fact, eligible for — so it earns the same
// observe-then-enforce caution before being flipped on.
//
// Reads os.Getenv on every call rather than memoizing: the read itself is a cheap map
// lookup, and memoizing (e.g. via sync.OnceValue) would freeze whichever value the FIRST
// call happened to see for the rest of the process — including a test binary, where
// t.Setenv could then never make a later test see a different value.
// See openspec/changes/add-auto-apply-eligibility-gate.
func autoApplyEligibilityEnforce() bool {
	return os.Getenv("AUTO_APPLY_ELIGIBILITY_ENFORCE") == "1"
}

// autoApplyEligibilityMismatchPrefix is what a caller sees when the eligibility gate
// refuses to enqueue — distinct from a generic failure, naming the reason so the
// candidate understands why (and does not read it as a transient error worth retrying).
const autoApplyEligibilityMismatchPrefix = "this job's requirements do not match your profile: "

// eligibilityBlocker reports the first unmet work-authorization or location-and-work-mode
// blocker for the (userID, job) pair, reusing the same deterministic evaluator and input
// assembly the match-analysis surface already uses (h.cv.match.jobBlockers) — see
// internal/candidate/hardconstraint. Every other blocker category (experience, education,
// certification, language) is irrelevant here: those bear on whether the candidate is a
// good FIT, not on whether submitting the application would misrepresent them.
//
// Degrades to "no blocker" (never refuses) whenever the evaluation cannot be run at all —
// no wired profile/résumé service, no profile, no structured résumé — the same
// "never emit a false blocker" discipline hardconstraint itself follows: absence of
// evidence is never treated as evidence of ineligibility.
func (h *assistantHandlers) eligibilityBlocker(ctx context.Context, userID int64, job db.Job) (hardconstraint.Blocker, bool) {
	if h.cv == nil || h.cv.match == nil || h.profile == nil || h.profile.userProfile == nil {
		return hardconstraint.Blocker{}, false
	}
	profile, err := h.profile.userProfile.Get(ctx, userID)
	if err != nil {
		return hardconstraint.Blocker{}, false
	}
	return firstEligibilityBlocker(h.cv.match.jobBlockers(ctx, userID, job, profile))
}

// firstEligibilityBlocker is the pure decision this gate makes: the first unmet
// work-authorization or location-and-work-mode entry among the blockers the evaluator
// already computed. Split out from eligibilityBlocker so this filtering — the actual
// policy — is unit-testable without a résumé store or a profile service.
func firstEligibilityBlocker(blockers []hardconstraint.Blocker) (hardconstraint.Blocker, bool) {
	for _, b := range blockers {
		if b.Met {
			continue
		}
		if b.Category == hardconstraint.CategoryWorkAuth || b.Category == hardconstraint.CategoryLocationWorkMode {
			return b, true
		}
	}
	return hardconstraint.Blocker{}, false
}

// PostJobAutoApply is the candidate-facing trigger auto-apply-tailored-resume and
// auto-apply-inngest-orchestration both assumed would exist: it creates one durable
// auto-apply attempt for the caller and the named job, starting the already-built
// tailor-then-review sequence. Cookie-only (mw.cookie, never mw.key) — the same
// "the browser is the only place the candidate can watch it happen and undo it" reasoning
// /autopilot already uses, at higher stakes here: a fresh entry can end in a REAL
// submitted job application, not only an unattended CV rewrite. The one other door is the
// signed-in MCP server, and only for an account that switched agents_may_apply on from
// this same browser session (openspec/changes/auto-apply-over-mcp).
func (h *assistantHandlers) PostJobAutoApply(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var refused *autoApplyAllowanceRefusal
	switch err := h.startAutoApply(c.Context(), userID, c.Params("slug")); {
	case errors.As(err, &refused):
		return refuse(c, refused.decision)
	case err != nil:
		return err
	}
	return autoApplyQueuedResponse(c)
}

// autoApplyAllowanceRefusal is startAutoApply's answer when the day's allowance is spent. A
// type of its own rather than a fiber.Error because the browser renders it as the 402 body
// every plan refusal shares — figures, reset time, upgrade link — while an agent reads only
// the sentence.
type autoApplyAllowanceRefusal struct{ decision plan.Decision }

func (r *autoApplyAllowanceRefusal) Error() string {
	return refusalMessage(r.decision.Feature, r.decision.FairUse, planIncludes(viewDecision(r.decision)))
}

// startAutoApply creates one auto-apply attempt for userID and the job behind slug, or
// reports why it may not. The route and the MCP tool both call it, so the gates cannot
// differ between them.
//
// Gates, in order: the job must be on an ATS auto-apply can queue against, the caller must
// already have a base CV, the pair must not already be applied to, and the caller must have
// auto-apply allowance left today. The allowance is LAST so that everything a request can be
// refused for on other grounds costs nothing — and it is where the old "must be on the PRO
// tier" check went, since free is configured to no allowance at all. A repeat request for a
// pair that already has a live, undecided entry succeeds without creating a second row; a
// repeat for a pair whose entry was already declined by the candidate is refused permanently.
//
// An unknown slug comes back as pgx.ErrNoRows, which the HTTP path renders as 404.
func (h *assistantHandlers) startAutoApply(ctx context.Context, userID int64, slug string) error {
	job, err := h.queries.GetJobBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if !autoApplyEnqueueSources[job.Source] {
		return fiber.NewError(fiber.StatusBadRequest, "auto-apply cannot queue an attempt against this posting's ATS")
	}

	if h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	if _, ok, err := h.cv.cvStore.BaseCV(ctx, userID); err != nil {
		return err
	} else if !ok {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before using auto-apply")
	}

	// cmd/auto-apply/store.go's Submit deletes the queue row in the same transaction it
	// marks the job applied (applications.applied_at, via MarkJobApplied), so a completed
	// attempt leaves no queue row behind for EnqueueAutoApply's own ON CONFLICT to catch —
	// without this check a re-click after a real submission would start a second one.
	if applied, err := h.queries.GetUserJobApplied(ctx, db.GetUserJobAppliedParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	}); err != nil {
		return err
	} else if applied {
		return fiber.NewError(fiber.StatusConflict, "already applied to this job")
	}

	// Eligibility gate (openspec/changes/add-auto-apply-eligibility-gate): refuse — or, in
	// shadow mode, merely log — enqueueing a pair the candidate's own known
	// work-authorization or location evidence positively conflicts with. Runs before the
	// allowance charge, like every other gate here, so a refused request costs nothing.
	if b, mismatched := h.eligibilityBlocker(ctx, userID, job); mismatched {
		enforced := autoApplyEligibilityEnforce()
		// Logged in both modes — tagged by category — so the false-positive rate among
		// Pro candidates can be read from shadow-mode logs before enforcement is flipped
		// on, and stays observable afterward too (see design.md's Migration Plan).
		verb := "would refuse"
		if enforced {
			verb = "refused"
		}
		log.Printf("auto-apply: eligibility gate %s user %d job %d (%s): %s", verb, userID, job.ID, b.Category, b.Reason)
		if enforced {
			return fiber.NewError(fiber.StatusConflict, autoApplyEligibilityMismatchPrefix+b.Reason)
		}
	}

	// The allowance, LAST of the gates, so a request refused for an unsupported platform, a
	// missing CV or an application already sent spends nothing.
	//
	// Charged BEFORE anything is created and keyed by the POSTING. Consume is idempotent by
	// its reference, so a double-clicked button reads as already charged and costs one
	// attempt rather than two — and so does a retry after a database error. Charging after
	// the queue write instead would have to key on the queue row's id, which a repeat request
	// never gets, so the second click would charge again.
	if err := h.chargeAutoApply(ctx, userID, job.ID); err != nil {
		return err
	}

	h.ensureTrackedForAutoApply(ctx, userID, job)

	id, err := h.queries.EnqueueAutoApply(ctx, db.EnqueueAutoApplyParams{UserID: userID, JobID: job.ID})
	switch {
	case err == nil:
		// This request's own INSERT won the race — it is the only caller that may ever
		// publish for this entry (see design.md: publishing on every request, including
		// an idempotent replay that touched no row, would let the orchestrator's executor
		// see the same auto-apply/submit more than once for one entry).
		h.publishSubmit(ctx, id)
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		existing, ferr := h.queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		})
		if ferr != nil {
			return ferr
		}
		switch autoApplyEntryStatus(existing.ReviewDecision, existing.FailedAt, existing.BlockedAt) {
		case autoApplyReviewDeclined:
			return fiber.NewError(fiber.StatusConflict, autoApplyAlreadyDeclinedMessage)
		case autoApplyStatusFailed:
			if existing.SubmitUncertainAt.Valid {
				return fiber.NewError(fiber.StatusConflict, autoApplyUncertainMessage)
			}
			// cmd/auto-apply gave up on this entry (dead-lettered or parked on an
			// unresolvable form field) — reporting "queued" here would tell the
			// candidate their attempt is still in flight when it is stuck. It is
			// retryable, which is the way forward this names.
			return fiber.NewError(fiber.StatusConflict, autoApplyStuckMessage)
		default:
			// A live, undecided entry already exists (the common "page reload, a second
			// tab" case per design.md) — reporting success without a second row is the
			// intended idempotent behavior, not a fault to log.
			return nil
		}
	default:
		return err
	}
}

// autoApplyStuckMessage answers a start request for a pair whose attempt stopped. It names
// retry because that is now the way forward; before retry existed the same state was final.
const autoApplyStuckMessage = "this auto-apply attempt stopped — retry it rather than starting a new one"

// ensureTrackedForAutoApply puts job on the caller's tracker board at stage `preparing`
// when it is not there already (openspec/changes/auto-apply-review-tracking) — otherwise an
// auto-apply attempt is invisible on the board until it happens to succeed. Never moves a
// job that already carries a stage: GetApplicationStage's own NULL-or-no-row result is the
// only trigger, so a candidate further along (say, `interview`) is left untouched.
//
// Best-effort: a failure here must never cost the candidate their auto-apply attempt, the
// same convention every other side-effect write in this handler already follows
// (h.publishSubmit, notifyTailoredCVReady's own successor in cmd/auto-apply).
func (h *assistantHandlers) ensureTrackedForAutoApply(ctx context.Context, userID int64, job db.Job) {
	current, err := h.queries.GetApplicationStage(ctx, db.GetApplicationStageParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("auto-apply: reading current stage for user %d job %d: %v", userID, job.ID, err)
		return
	}
	if current.Valid {
		return
	}
	stage := "preparing"
	if _, err := h.tracking.tracking.Track(ctx, userID, job.PublicSlug, &stage, nil, "auto_apply"); err != nil {
		log.Printf("auto-apply: tracking job %d for user %d: %v", job.ID, userID, err)
	}
}

// chargeAutoApply spends one unit of the caller's daily auto-apply allowance for this
// posting. A spent allowance comes back as *autoApplyAllowanceRefusal.
//
// The reference is the POSTING's id, which makes the charge idempotent for the pair by
// construction: a second request for the same job reads as already charged and costs
// nothing, whether it came from a double click or a retry after a failure.
//
// Unlike the other metered features, a failure to CHARGE does not let the action through.
// Everywhere else the fallback is "record nothing and continue", because the action is a
// model call whose cost we absorb. Here the action drives a browser and submits a real
// application to a real employer under somebody's name, so an unmeasured one is not a
// rounding error — the honest answer to "we could not tell whether you may" is to say so.
func (h *assistantHandlers) chargeAutoApply(ctx context.Context, userID, jobID int64) error {
	if h.plans == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "auto-apply is not available")
	}

	ref := "job:" + strconv.FormatInt(jobID, 10)
	d, err := h.plans.Consume(ctx, userID, plan.FeatureAutoApply, ref)
	switch {
	case err == nil:
		return nil
	case isRefusal(err):
		return &autoApplyAllowanceRefusal{decision: d}
	default:
		log.Printf("plan: charging an auto-apply for user %d on job %d: %v", userID, jobID, err)
		return fiber.NewError(fiber.StatusServiceUnavailable, "auto-apply is not available")
	}
}
