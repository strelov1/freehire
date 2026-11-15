package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyEnqueueSources are the ATS providers auto-apply can queue an attempt against.
// Only Greenhouse can currently SUBMIT one (fillProviders in internal/api/atsapply); the
// others still tailor and get reviewed, then park with a named reason in cmd/auto-apply.
// Widening this list is a product decision: each attempt spends a daily tailoring turn.
var autoApplyEnqueueSources = map[string]bool{
	"greenhouse": true,
	"ashby":      true,
	"workable":   true,
	"lever":      true,
	"recruitee":  true,
}

// autoApplyAlreadyDeclinedMessage answers a permanently-declined pair. Distinct from
// autoApplyDeclineReason (auto_apply_tailor.go), the stored last_error.
const autoApplyAlreadyDeclinedMessage = "this auto-apply attempt was already declined"

// autoApplyStatusQueued is the wire status for a live, undecided auto-apply entry,
// shared with GetJob's status overlay (jobs.go).
const autoApplyStatusQueued = "queued"

// autoApplyStatusFailed is the wire status for an entry cmd/auto-apply gave up on:
// dead-lettered (failed_at) or parked for missing form data (blocked_at). Both leave
// review_decision 'approved', so without checking those columns a stuck attempt reads
// as "queued" forever.
const autoApplyStatusFailed = "failed"

// autoApplyEntryStatus derives the wire status from the columns
// GetAutoApplyQueueEntryForJob reads. Declined is checked first: DeclineAutoApplyReview
// also sets blocked_at, so checking blocked first would report a decline as a failure.
func autoApplyEntryStatus(reviewDecision pgtype.Text, failedAt, blockedAt pgtype.Timestamptz) string {
	if reviewDecision.Valid && reviewDecision.String == autoApplyReviewDeclined {
		return autoApplyReviewDeclined
	}
	if failedAt.Valid || blockedAt.Valid {
		return autoApplyStatusFailed
	}
	return autoApplyStatusQueued
}

// autoApplyEligibilityEnforce gates whether an eligibility mismatch (see
// eligibilityBlocker) refuses enqueueing or only logs. OFF (shadow-only) by default like
// PLAN_ENFORCE: a false positive blocks a paying candidate from a job they are eligible for.
//
// Reads os.Getenv on every call: memoizing would freeze the first value for the process,
// so t.Setenv could never change it in a later test.
func autoApplyEligibilityEnforce() bool {
	return os.Getenv("AUTO_APPLY_ELIGIBILITY_ENFORCE") == "1"
}

// autoApplyEligibilityMismatchPrefix starts the refusal message, so the candidate does
// not read it as a transient error worth retrying.
const autoApplyEligibilityMismatchPrefix = "this job's requirements do not match your profile: "

// eligibilityBlocker reports the first unmet work-authorization or location-and-work-mode
// blocker for the (userID, job) pair, using the evaluator match analysis uses
// (h.cv.match.jobBlockers, internal/candidate/hardconstraint). Other categories
// (experience, education, ...) bear on fit, not on misrepresenting the candidate.
//
// Degrades to "no blocker" when evaluation cannot run (no service, profile or structured
// résumé): absence of evidence is never evidence of ineligibility.
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

// firstEligibilityBlocker picks the first unmet work-authorization or location-and-work-mode
// entry among the computed blockers. Split out so the policy is testable without stores.
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

// PostJobAutoApply creates one durable auto-apply attempt for the caller and the named
// job, starting the tailor-then-review sequence. Cookie-only (mw.cookie, never mw.key):
// the browser is the only place the candidate can watch and undo it, and an entry can end
// in a REAL submitted application. The one other door is the signed-in MCP server, only
// for an account that switched agents_may_apply on from this browser session.
func (h *assistantHandlers) PostJobAutoApply(ctx context.Context, params apigen.PostJobAutoApplyParams) (apigen.PostJobAutoApplyRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var refused *autoApplyAllowanceRefusal
	switch err := h.startAutoApply(ctx, userID, params.Slug); {
	case errors.As(err, &refused):
		return decisionRefusal(refused.decision).assistant()
	case err != nil:
		return nil, err
	}
	return &apigen.PostJobAutoApplyOK{Data: apigen.AutoApplyQueued{Status: autoApplyStatusQueued}}, nil
}

// autoApplyAllowanceRefusal is startAutoApply's answer when the day's allowance is spent.
// Its own type because the browser renders the shared 402 body while an agent reads only the sentence.
type autoApplyAllowanceRefusal struct{ decision plan.Decision }

func (r *autoApplyAllowanceRefusal) Error() string {
	return refusalMessage(r.decision.Feature, r.decision.FairUse, planIncludes(viewDecision(r.decision)))
}

// startAutoApply creates one auto-apply attempt for userID and the job behind slug, or
// reports why it may not. The route and the MCP tool both call it, so the gates cannot
// differ between them.
//
// Gates, in order: the job must be on a queueable ATS, the caller must have a base CV or
// a résumé to seed one, the pair must not already be applied to, and allowance must be
// left today. The allowance is LAST so every other refusal costs nothing. A repeat for a
// pair with a live, undecided entry succeeds without a second row; a repeat for a pair
// the candidate declined is refused permanently.
//
// An unknown slug comes back as pgx.ErrNoRows, which the HTTP path renders as 404.
func (h *assistantHandlers) startAutoApply(ctx context.Context, userID int64, slug string) error {
	job, err := jobBySlugOrCode(ctx, h.queries, slug)
	if err != nil {
		return err
	}
	if !autoApplyEnqueueSources[job.Source] {
		return assistant.WithLogReason(fiber.NewError(fiber.StatusBadRequest, "auto-apply cannot queue an attempt against this posting's ATS"), "ats not supported for auto-apply")
	}

	if h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// The base CV need not exist yet: cvStore.Tailor seeds one from the extracted résumé,
	// so this asks Tailor's own question. Requiring the cvs row refused candidates who
	// uploaded a résumé but never opened the CV builder.
	if _, ok, err := h.cv.cvStore.BaseCV(ctx, userID); err != nil {
		return err
	} else if !ok {
		if _, hasResume, err := h.cv.seedSource().Structured(ctx, userID); err != nil {
			return err
		} else if !hasResume {
			return assistant.WithLogReason(fiber.NewError(fiber.StatusConflict, "add a résumé before using auto-apply"), "no resume for auto-apply")
		}
	}

	// cmd/auto-apply/store.go's Submit deletes the queue row in the same transaction it marks
	// the job applied, so ON CONFLICT cannot catch a re-click after a real submission; this check does.
	if applied, err := h.queries.GetUserJobApplied(ctx, db.GetUserJobAppliedParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	}); err != nil {
		return err
	} else if applied {
		return assistant.WithLogReason(fiber.NewError(fiber.StatusConflict, "already applied to this job"), "already applied")
	}

	// Eligibility gate: refuse (shadow mode: only log) a pair the candidate's known
	// work-authorization or location evidence conflicts with. Runs before the charge.
	if b, mismatched := h.eligibilityBlocker(ctx, userID, job); mismatched {
		enforced := autoApplyEligibilityEnforce()
		// Logged in both modes, tagged by category, so the false-positive rate stays observable.
		verb := "would refuse"
		if enforced {
			verb = "refused"
		}
		log.Printf("auto-apply: eligibility gate %s user %d job %d (%s): %s", verb, userID, job.ID, b.Category, b.Reason)
		if enforced {
			return fiber.NewError(fiber.StatusConflict, autoApplyEligibilityMismatchPrefix+b.Reason)
		}
	}

	// Charged BEFORE anything is created and keyed by the POSTING: Consume is idempotent by
	// reference, so a double click or a retry after a database error costs one attempt.
	// Keying on the queue row's id would charge again, since a repeat request never gets one.
	if err := h.chargeAutoApply(ctx, userID, job.ID); err != nil {
		return err
	}

	h.ensureTrackedForAutoApply(ctx, userID, job)

	id, err := h.queries.EnqueueAutoApply(ctx, db.EnqueueAutoApplyParams{UserID: userID, JobID: job.ID})
	switch {
	case err == nil:
		// Only the request whose INSERT won may publish: an idempotent replay publishing
		// too would let the orchestrator see the same auto-apply/submit more than once.
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
			// cmd/auto-apply gave up on this entry; "queued" would hide that it is stuck. It is retryable.
			return fiber.NewError(fiber.StatusConflict, autoApplyStuckMessage)
		default:
			// A live, undecided entry exists (page reload, second tab): idempotent success, not a fault.
			return nil
		}
	default:
		return err
	}
}

// autoApplyStuckMessage answers a start request for a pair whose attempt stopped; it names retry.
const autoApplyStuckMessage = "this auto-apply attempt stopped — retry it rather than starting a new one"

// ensureTrackedForAutoApply puts job on the caller's tracker board at stage `preparing`
// when it is not there, so an attempt is visible before it succeeds. Never moves a job
// that already has a stage (GetApplicationStage NULL or no row is the only trigger).
//
// Best-effort: a failure must never cost the candidate their attempt.
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
// The reference is the POSTING's id, so a second request for the same job reads as
// already charged (double click or retry).
//
// Unlike other metered features, a failure to CHARGE does not let the action through:
// this submits a real application to a real employer under the candidate's name.
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
		// Wrapped, not a 503: classify reports only a 500, and this fault must reach the exception log.
		return fmt.Errorf("charge auto-apply for job %d: %w", jobID, err)
	}
}
