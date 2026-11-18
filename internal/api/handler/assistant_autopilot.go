package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autopilotMaxSteps bounds an unattended tailoring run: about two rounds per requirement over a
// dozen or two requirements, which the ordinary ceiling would cut off halfway.
const autopilotMaxSteps = 30

// tailorMaxSteps bounds an interactive tailoring turn. Sixteen covers a short "bank then edit" pass;
// the chat default of eight left edits dangling. Autopilot keeps its own higher ceiling.
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

// autopilotBrief opens an unattended run. Deliberately short: the method lives in the tailoring
// system prompt; this only says which rhythm the candidate chose, since a turn needs a message to start.
const autopilotBrief = "Tailor this CV for the vacancy yourself, working from my experience bank. " +
	"Go through every requirement without stopping to ask me, then tell me what is left. " +
	"Matching the vacancy's spelling of a skill is handled outside this run — do not spend an edit renaming skills for wording; spend them on evidence and substance."

// PostAssistantAutopilot runs the unattended tailoring pass as one long streamed turn. Every edit is
// filed under one revision batch (streamSSE mints the id), so undoing the run is reverting that batch.
// The brief and the ceiling are fixed here: the undo handle cannot be the client's job.
func (h *assistantHandlers) PostAssistantAutopilot(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil || h.cv == nil || h.cv.cvStore == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// Both bindings: the CV tools are registered only when the session carries a vacancy too, so a
	// CV-only session would burn a thirty-round turn with no cv_context, cv_edit or way to report.
	if sess.Preset != assistant.PresetTailor || sess.CVID == nil || sess.JobID == nil {
		return fiber.NewError(fiber.StatusConflict, "this conversation is not tailoring a CV")
	}
	// One read of the vacancy for the whole pre-run (fit analysis and surface alignment both need it).
	// An unreadable vacancy leaves a nil analysis and empty description, which the steps below skip.
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
		// The pre-run happens here, not in the handler above: a handler that has not returned has
		// written no byte, and a cold-start chain takes minutes (see autopilotAnalysis).
		return h.runAutopilotToCompletion(ctx, analysis, sess, jobDescription, runner, reg, system, emit)
	})
}

// runAutopilotToCompletion runs the pre-run analysis fill, lays down the run plan, executes the
// autopilot pass and refreshes the analysis afterward. Shared with PostAutoApplyTailor.
// Order matters: the run plan is written from the analysis and cv_context reads it; surface alignment
// gets its own system revision, so undoing the run leaves the vacancy's wording in place.
//
// analysis may be nil (no match surface wired) — every method on it is a no-op then.
func (h *assistantHandlers) runAutopilotToCompletion(ctx context.Context, analysis *autopilotAnalysis, sess assistant.Session, jobDescription string, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
	// A run with no requirement list is a different run: cv_context says there are none and the agent
	// edits against nothing. Refusing costs the candidate a run; proceeding edits their document against
	// a vacancy nobody read. So stop, and say why on the stream the client is already reading.
	if err := analysis.ensure(ctx); err != nil {
		log.Printf("assistant: autopilot has no requirement list, session %s: %v", sess.ID, err)
		return h.refuseAutopilotWithoutAPlan(emit)
	}
	h.cv.logSurfaceAlign(ctx, sess.UserID, *sess.CVID, jobDescription)
	h.layDownRunPlan(ctx, sess)

	err := runner.Run(ctx, sess, reg, system, autopilotBrief, assistant.TurnConfig{MaxSteps: autopilotMaxSteps}, emit)
	// The refresh must run even when the caller cancelled the run (CancelAssistantTurn cancels this
	// ctx), so it runs on a detached copy, like boundRunner's credential-forget.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	analysis.refresh(rctx)
	return err
}

// refuseAutopilotWithoutAPlan ends the run before it starts, on the stream the client is already
// reading. Both frames are load-bearing: the text is what the candidate sees, and the terminal result
// is what the web app needs to call endTurn(); a stream that just closes leaves the session believing
// a turn is in flight (see Runner.Run). Returns an error so the caller logs it, as a failed turn does.
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

// errNoRequirementList marks a run refused for want of a fit analysis, whatever produced the absence.
var errNoRequirementList = errors.New("autopilot: no requirement list to walk")

// prepareAutopilotAnalysis delegates to matchHandlers.prepareAutopilotRun, which builds the pre-run
// fill and post-run refresh from reads taken while c is valid. A missing match surface yields a nil
// whose ensure and refresh are no-ops.
func (h *assistantHandlers) prepareAutopilotAnalysis(c *fiber.Ctx, sess assistant.Session, job db.Job) (*autopilotAnalysis, error) {
	if h.cv == nil || h.cv.match == nil {
		return nil, nil
	}
	return h.cv.match.prepareAutopilotRun(c, sess.UserID, job)
}

// layDownRunPlan writes the vacancy's requirements onto the CV as `not_reached` before the run, so a
// run that never reports still leaves a plan. The agent cannot cover this: at the step cap the runner's
// final call offers no tools, so the run that spends its budget cannot call tailor_report.
// Best-effort: a missing analysis leaves the report as it was.
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
