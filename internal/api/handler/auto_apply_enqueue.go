package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyAlreadyDeclinedMessage answers a permanently-declined pair. Distinct from
// autoApplyDeclineReason (auto_apply_tailor.go), the stored last_error.
const autoApplyAlreadyDeclinedMessage = "this auto-apply attempt was already declined"

// autoApplyStuckMessage answers a start request for a pair whose attempt stopped; it names retry.
const autoApplyStuckMessage = "this auto-apply attempt stopped — retry it rather than starting a new one"

// autoApplyEligibilityMismatchPrefix starts the refusal message, so the candidate does
// not read it as a transient error worth retrying.
const autoApplyEligibilityMismatchPrefix = "this job's requirements do not match your profile: "

// autoApplyEnqueuer assembles the start use case over this handler's wiring, so the route and
// the MCP tool run one sequence of gates. Assembled per call from the fields below for the
// same reason cvHandlers.seedSource is: tests build the handlers as literals, and a nil there
// means what it means in production — a deployment without that dependency.
func (h *assistantHandlers) autoApplyEnqueuer() *autoapply.Enqueuer {
	e := &autoapply.Enqueuer{
		FindJob: func(ctx context.Context, slug string) (db.Job, error) {
			return jobBySlugOrCode(ctx, h.queries, slug)
		},
		Queue:         h.queries,
		Plans:         h.plans,
		PublishSubmit: h.publishSubmit,
	}
	if h.cv != nil {
		if h.cv.cvStore != nil {
			e.BaseCV = func(ctx context.Context, userID int64) (bool, error) {
				_, ok, err := h.cv.cvStore.BaseCV(ctx, userID)
				return ok, err
			}
		}
		e.Seeder = h.cv.seedSource()
	}
	// The eligibility evaluation needs the match surface's inputs and the caller's profile;
	// without either it cannot run, and the use case then sees no blocker.
	if h.cv != nil && h.cv.match != nil && h.profile != nil && h.profile.userProfile != nil {
		e.Blockers = func(ctx context.Context, userID int64, job db.Job) []hardconstraint.Blocker {
			profile, err := h.profile.userProfile.Get(ctx, userID)
			if err != nil {
				return nil
			}
			return h.cv.match.jobBlockers(ctx, userID, job, profile)
		}
	}
	if h.tracking != nil && h.tracking.tracking != nil {
		e.Tracker = boardTracker{h.tracking.tracking.Track}
	}
	return e
}

// boardTracker adapts jobtracking.Service.Track to the use case's Tracker port: the
// interaction a best-effort board write returns has no reader here.
type boardTracker struct {
	track func(ctx context.Context, userID int64, slug string, stage, notes *string, source string) (jobtracking.Interaction, error)
}

func (b boardTracker) Track(ctx context.Context, userID int64, slug string, stage, notes *string, source string) error {
	_, err := b.track(ctx, userID, slug, stage, notes, source)
	return err
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
	if err := h.startAutoApply(ctx, userID, params.Slug); err != nil {
		var refused *autoapply.AllowanceRefusal
		if errors.As(err, &refused) {
			// The shared 402 body the browser renders; an agent reads only the sentence (toolError).
			return decisionRefusal(refused.Decision).assistant()
		}
		return nil, autoApplyEnqueueError(err)
	}
	return &apigen.PostJobAutoApplyOK{Data: apigen.AutoApplyQueued{Status: string(autoapply.EntryQueued)}}, nil
}

// startAutoApply runs the enqueue use case for the caller and the job behind slug. The route
// and the MCP tool both call it, so the gates cannot differ between them.
func (h *assistantHandlers) startAutoApply(ctx context.Context, userID int64, slug string) error {
	return h.autoApplyEnqueuer().Start(ctx, userID, slug)
}

// autoApplyEnqueueError maps the enqueue use case's typed answers onto HTTP statuses and
// sentences, the way employerError and recoveryError do for theirs. Both transports run it:
// the route through RenderError, the MCP tool through toolError, so a refusal carries the
// same status (which the tool's metrics classify by) and the same sentence to an agent.
// Anything else — pgx.ErrNoRows from the job lookup, a charge fault — is returned as it is.
func autoApplyEnqueueError(err error) error {
	var refused *autoapply.Refusal
	if !errors.As(err, &refused) {
		return err
	}
	switch refused.Reason {
	case autoapply.RefusalUnsupportedSource:
		return assistant.WithLogReason(fiber.NewError(fiber.StatusBadRequest, "auto-apply cannot queue an attempt against this posting's ATS"), "ats not supported for auto-apply")
	case autoapply.RefusalNoCVBuilder:
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	case autoapply.RefusalNoResume:
		return assistant.WithLogReason(fiber.NewError(fiber.StatusConflict, "add a résumé before using auto-apply"), "no resume for auto-apply")
	case autoapply.RefusalAlreadyApplied:
		return assistant.WithLogReason(fiber.NewError(fiber.StatusConflict, "already applied to this job"), "already applied")
	case autoapply.RefusalEligibilityMismatch:
		return fiber.NewError(fiber.StatusConflict, autoApplyEligibilityMismatchPrefix+refused.Detail)
	case autoapply.RefusalAlreadyDeclined:
		return fiber.NewError(fiber.StatusConflict, autoApplyAlreadyDeclinedMessage)
	case autoapply.RefusalSubmitUncertain:
		return fiber.NewError(fiber.StatusConflict, autoApplyUncertainMessage)
	case autoapply.RefusalStuck:
		return fiber.NewError(fiber.StatusConflict, autoApplyStuckMessage)
	case autoapply.RefusalNoPlanMeter:
		return fiber.NewError(fiber.StatusServiceUnavailable, "auto-apply is not available")
	default:
		return err
	}
}
