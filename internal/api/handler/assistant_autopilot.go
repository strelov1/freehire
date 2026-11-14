package handler

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autopilotMaxSteps bounds an unattended tailoring run. A run reads the fit analysis and
// the CV, then spends roughly two rounds per requirement — a search and an edit — over a
// dozen or two requirements. The ordinary ceiling is written for a question and would cut
// such a run off halfway through the list.
const autopilotMaxSteps = 30

// tailorMaxSteps bounds an interactive tailoring turn. Evidence lookups plus per-skill
// edits burn rounds fast — the chat default of eight left a Salesforce edit dangling after
// WordPress and Mailchimp. Sixteen covers a short "bank then edit" pass; autopilot keeps
// its own higher ceiling.
const tailorMaxSteps = 16

// turnBounds picks the step ceiling for a session. Chat and the other presets keep the
// runner default; interactive tailor and base-CV editing (the same evidence-then-edit rounds)
// and a resumed autopilot raise theirs server-side so a client cannot.
func turnBounds(sess assistant.Session, lastUser string) assistant.TurnConfig {
	if sess.Preset == assistant.PresetSupport {
		return assistant.TurnConfig{MaxSteps: supportMaxSteps}
	}
	if sess.Preset != assistant.PresetTailor && sess.Preset != assistant.PresetBase {
		return assistant.TurnConfig{}
	}
	if lastUser == autopilotBrief {
		return assistant.TurnConfig{MaxSteps: autopilotMaxSteps}
	}
	return assistant.TurnConfig{MaxSteps: tailorMaxSteps}
}

// autopilotBrief opens an unattended run. It is deliberately short: the method — walk every
// requirement, search the bank, edit what the evidence supports, ask nothing until the end —
// lives in the tailoring system prompt, where it is stated once. This only says which of the
// two rhythms the candidate chose, because a turn does not start until a message arrives.
const autopilotBrief = "Tailor this CV for the vacancy yourself, working from my experience bank. " +
	"Go through every requirement without stopping to ask me, then tell me what is left. " +
	"Matching the vacancy's spelling of a skill is handled outside this run — do not spend an edit renaming skills for wording; spend them on evidence and substance."

// PostAssistantAutopilot runs the unattended tailoring pass on a tailoring session as one
// long streamed turn. Every edit of the turn is filed under one revision batch (streamSSE
// mints the id), so undoing the run is reverting that batch.
//
// Everything a client could otherwise dictate is fixed here — the brief and the ceiling.
// The undo handle in particular cannot be the client's job: a run whose edits were never
// batched is a run nobody can take back.
func (h *assistantHandlers) PostAssistantAutopilot(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil || h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// Both bindings, not just the CV: the CV tools are registered only when the session
	// carries a vacancy too, so a CV-only session would burn a thirty-round turn with no
	// cv_context, no cv_edit and no way to report.
	if sess.Preset != assistant.PresetTailor || sess.CVID == nil || sess.JobID == nil {
		return fiber.NewError(fiber.StatusConflict, "this conversation is not tailoring a CV")
	}
	// One read of the vacancy for the whole pre-run: the fit analysis and the surface
	// alignment both need it, and a run must not ask the database twice for the same row.
	// It stays in the request because the pre-run has nothing to prepare without it; an
	// unreadable vacancy leaves a nil analysis and an empty description, both of which the
	// steps below already treat as nothing to do.
	var analysis *autopilotAnalysis
	var jobDescription string
	if job, err := h.queries.GetJob(c.Context(), *sess.JobID); err != nil {
		log.Printf("assistant: loading job %d for the autopilot pre-run: %v", *sess.JobID, err)
	} else {
		if analysis, err = h.prepareAutopilotAnalysis(c, sess, job); err != nil {
			return err
		}
		jobDescription = job.Description
	}
	return h.streamSSE(c, sess, h.meterTurn, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		// The pre-run happens HERE rather than in the handler above, because a handler that
		// has not returned has written no byte and a cold-start chain takes minutes — see
		// autopilotAnalysis for what that cost.
		return h.runAutopilotToCompletion(ctx, analysis, sess, jobDescription, runner, reg, system, emit)
	})
}

// runAutopilotToCompletion runs the pre-run analysis fill, lays down the run plan, executes
// the autopilot pass to completion (or until the step cap or an error stops it), and
// refreshes the analysis afterward — the exact body PostAssistantAutopilot's stream callback
// runs, extracted so the queue-scoped auto-apply tailoring endpoint (PostAutoApplyTailor,
// openspec/changes/auto-apply-tailored-resume) shares it rather than duplicating it. Order
// is the order PostAssistantAutopilot always had: the analysis first, because the run plan
// is written from it and cv_context reads it on the run's first step; surface alignment gets
// its own system revision, not the run's edit batch, so undoing the run leaves the vacancy's
// wording in place.
//
// analysis may be nil (no match surface wired) — every method on it is a no-op then.
func (h *assistantHandlers) runAutopilotToCompletion(ctx context.Context, analysis *autopilotAnalysis, sess assistant.Session, jobDescription string, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
	// A run with no requirement list is not a cheaper run, it is a different one: the brief
	// is "go through every requirement", cv_context answers that there are none, and the
	// agent improvises a turn's worth of edits against nothing. Measured on production
	// 2026-08-25..09-08, 36% of runs went out that way and nothing said so.
	//
	// Refusing costs the candidate a run they asked for. Proceeding costs them their
	// document, edited against a vacancy nobody read — and undoing that is their work, not
	// ours. So this stops, and says why on the stream the client is already reading.
	if err := analysis.ensure(ctx); err != nil {
		log.Printf("assistant: autopilot has no requirement list, session %s: %v", sess.ID, err)
		return h.refuseAutopilotWithoutAPlan(emit)
	}
	h.cv.logSurfaceAlign(ctx, sess.UserID, *sess.CVID, jobDescription)
	h.layDownRunPlan(ctx, sess)

	err := runner.Run(ctx, sess, reg, system, autopilotBrief, assistant.TurnConfig{MaxSteps: autopilotMaxSteps}, emit)
	// The refresh is unconditional — it must run even when the caller cancelled the run
	// partway through (CancelAssistantTurn cancels this very ctx for the streamed case) —
	// so it runs on a detached copy, the same way boundRunner's credential-forget already
	// does.
	analysis.refresh(context.WithoutCancel(ctx))
	return err
}

// refuseAutopilotWithoutAPlan ends the run before it starts, on the stream the client is
// already reading.
//
// Both frames are load-bearing. The text is what the candidate sees — a run that stopped
// with no explanation is indistinguishable from one that ran and found nothing to do. The
// terminal result is what the CLIENT needs: the web app only calls endTurn() on a `result`,
// so a stream that simply closes leaves the session believing a turn is still in flight and
// queues every later message behind one that is over (see Runner.Run's own note on this).
//
// It returns an error so the caller logs and reports it, exactly as a failed turn does.
func (h *assistantHandlers) refuseAutopilotWithoutAPlan(emit func(assistant.Event)) error {
	emit(assistant.Event{
		Kind: assistant.EventAssistantText,
		Text: "I could not read this vacancy's requirements — the fit analysis did not " +
			"complete, so there was nothing for the run to work through. Your CV is " +
			"untouched. Try again in a moment.",
	})
	emit(assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopError, IsError: true})
	return errNoRequirementList
}

// errNoRequirementList marks a run refused for want of a fit analysis. Distinct from the
// chain's own error: what stopped the run is the ABSENCE of a plan, whatever produced it.
var errNoRequirementList = errors.New("autopilot: no requirement list to walk")

// prepareAutopilotAnalysis delegates to matchHandlers.prepareAutopilotRun, which assembles
// both halves of the run's fit analysis — the fill that gives cv_context something to read,
// and the refresh that follows the run — out of reads taken while c is still valid. A
// missing match surface degrades to a nil, whose ensure and refresh are no-ops: the same
// best-effort posture this step always had.
func (h *assistantHandlers) prepareAutopilotAnalysis(c *fiber.Ctx, sess assistant.Session, job db.Job) (*autopilotAnalysis, error) {
	if h.cv == nil || h.cv.match == nil {
		return nil, nil
	}
	return h.cv.match.prepareAutopilotRun(c, sess.UserID, job)
}

// layDownRunPlan writes the vacancy's requirements onto the CV as `not_reached` before the
// run starts, so a run that never reports still leaves one.
//
// It has to be the server's doing, because the agent cannot cover this case: on reaching the
// step cap the runner makes its final call with NO tools offered, so a run that spends its
// whole budget is exactly the run that cannot call `tailor_report`. Without the plan, the
// worst-case run — the one that edited the CV and stopped halfway — is also the one whose
// panel shows nothing, and therefore offers no way to undo it.
//
// Best-effort: a missing or unreadable analysis leaves the report as it was. The run itself
// is worth starting either way, and the agent replaces the whole report when it reports.
func (h *assistantHandlers) layDownRunPlan(ctx context.Context, sess assistant.Session) {
	if h.fit == nil {
		return
	}
	analysis, err := h.fit.Required(ctx, sess.UserID, *sess.JobID)
	if err != nil || analysis == nil {
		return
	}
	plan := make([]cv.AutopilotEntry, 0, len(analysis.RequirementMatch))
	for _, r := range analysis.RequirementMatch {
		if strings.TrimSpace(r.Text) == "" {
			continue
		}
		plan = append(plan, cv.AutopilotEntry{Requirement: r.Text, Status: cv.AutopilotNotReached})
	}
	if len(plan) == 0 {
		return
	}
	if err := h.cv.cvStore.SetAutopilotReport(ctx, *sess.CVID, sess.UserID, plan); err != nil {
		log.Printf("assistant: could not lay down the run plan cv=%s: %v", sess.CVID, err)
	}
}
