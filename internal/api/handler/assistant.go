package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/problemreport"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// assistantHandlers is the in-app agent. It owns the conversation store and the turn runner and
// reaches the other feature handlers for the services its tools call. It is a facade over the
// same use cases the HTTP surface exposes, so it must not grow a second copy of any of them.
type assistantHandlers struct {
	store  *assistant.Store
	runner *assistant.Runner

	// llm is the agent's own client, kept beside the runner so a turn can be re-bound to the
	// caller's gateway credential (the runner holds it as an interface).
	llm *llm.Client
	// keys resolves the credential a turn spends under. Nil means spend on the service credential.
	keys *llmkey.Resolver
	// maxPrompt is the rune ceiling on one user message (ASSISTANT_MAX_PROMPT).
	maxPrompt int

	// turns are the turns running now, so a cancel can reach a turn streaming on a goroutine no
	// request owns, and a session keeps to one turn at a time. Zero value works; see assistant_turns.go.
	turns turnRegistry

	queries *db.Queries
	// fit reads the cached fit analysis the tailoring and interview tools ground on. Held directly,
	// not via the CV handlers: tools must not depend on which HTTP surface assembles it.
	fit *fitanalysis.Service
	// jobs serves the vacancy a tool is about: one row by id, the same narrow read the CV surface takes.
	jobs     jobReader
	search   *searchHandlers
	resume   *resumeHandlers
	tracking *trackingHandlers
	// reports files a job report for report_job. Set by Register to the SAME handler that serves
	// POST /jobs/:slug/reports, so the paths cannot drift. Nil-safe: the tool refuses.
	reports    *reportHandlers
	dictionary *dictionaryHandlers
	problems   *problemreport.Service
	// buddies serves the interview-buddy and mock-interview tools of the signed-in MCP
	// server (buddy_mcp_tools.go). Set by Register to the handler behind the cookie routes.
	buddies *buddyHandlers
	cv      *cvHandlers
	// profile backs the get_profile tool. Reusing the profile handlers keeps the tool and
	// GET /me/profile on one assembly.
	profile *profileHandlers
	// letter serves cover_letter_draft. It is the SAME store, chain and bank the HTTP endpoint
	// holds, so chat and button cannot draft different letters for one pair. The bank is
	// deliberately NOT reviewableResume's file-only structure: a letter speaks for the candidate,
	// where the ATS report judges their document. Nil-safe.
	letter coverLetterDeps
	// experience backs the bank tools, which every preset offers.
	experience experienceBankTools
	// screeningAnswers backs screening_answers_set, offered by every preset: a candidate states
	// their notice period or salary expectation whenever it comes up.
	screeningAnswers *screeninganswers.Store
	// answerBank backs the signed-in MCP server's answer-bank tools: the store /me/answer-bank
	// serves, so an answer an agent saves is the one the worker sends.
	answerBank *answerbank.Store
	// mail backs the inbox tools, offered only by the general chat preset. They reach inbox.Service,
	// the use cases /me/inbox and /me/emails render, so a rule holds for both readers.
	mail *inboxHandlers
	// browserTools backs read_current_page in a browsing session: the hub the agentic autofill
	// drives, so the assistant is a second in-process harness on the user's channel.
	browserTools *browsertools.Hub
	// stages and invitation back the context a rehearsal and a debrief share: the application's
	// stage, and what the employer said when they invited the candidate.
	stages     applicationReader
	invitation invitationReader

	// plans meters a turn against the caller's plan. Nil leaves turns unmetered (test fixtures);
	// production always wires one.
	plans *plan.Store

	// support is the support agent's own wiring: its model, its runner and the mail a
	// handover goes out on. The zero value is the unconfigured deployment.
	support supportDeps

	// realtime mints voice-mode's short-lived OpenAI Realtime credentials. Nil without a Realtime
	// gateway (internal/ai/speech's convention): the mint endpoint answers 501 and the interview
	// view offers no voice mode.
	realtime realtimeMinter

	// events best-effort publishes auto-apply/review.decided so a paused cmd/auto-apply-orchestrate
	// run resumes. Nil publishes nothing.
	events autoApplyEventPublisher

	// extractPDFText reads an attached PDF's text (the résumé upload's extractor), a function so a
	// test need not shell out to pdftotext.
	extractPDFText func([]byte) (string, error)
}

// assistantModels names the model client the assistant runs on and the resolver that bills it.
type assistantModels struct {
	// Agent is the model the turn loop runs on. Nil leaves the runner nil: old conversations stay
	// readable and a new turn reports the assistant as unavailable.
	Agent *llm.Client
	// Keys names the caller on the gateway. Nil is ordinary: turns spend on the service credential.
	Keys      *llmkey.Resolver
	MaxSteps  int
	MaxPrompt int
}

// newAssistantHandlers wires the agent.
func newAssistantHandlers(
	queries *db.Queries,
	models assistantModels,
	store *assistant.Store,
	search *searchHandlers,
	resumeH *resumeHandlers,
	tracking *trackingHandlers,
	cvH *cvHandlers,
	profileH *profileHandlers,
	dictionary *dictionaryHandlers,
	problems *problemreport.Service,
	browserTools *browsertools.Hub,
	mail *inboxHandlers,
	bank experienceBankTools,
	screeningAnswers *screeninganswers.Store,
	plans *plan.Store,
	letter coverLetterDeps,
	prices func(context.Context) []billing.PublicPrice,
) *assistantHandlers {
	h := &assistantHandlers{
		experience:       bank,
		letter:           letter,
		screeningAnswers: screeningAnswers,
		store:            store,
		queries:          queries,
		jobs:             queries,
		search:           search,
		resume:           resumeH,
		tracking:         tracking,
		cv:               cvH,
		profile:          profileH,
		dictionary:       dictionary,
		problems:         problems,
		browserTools:     browserTools,
		mail:             mail,
		stages:           queries,
		plans:            plans,
		extractPDFText:   resume.ExtractPDFText,
		support:          supportDeps{prices: prices},
	}
	// The fit reads come from the service, not from whichever handler holds it, so reshaping the
	// CV surface cannot break a tool that grounds on the cached analysis.
	if cvH != nil {
		h.fit = cvH.fit
	}
	// The rehearsal reads the invitation through the mail service: that read leaving read_at alone
	// is inbox's guarantee, and bypassing it would put a second reader outside the rule.
	if mail != nil {
		h.invitation = mail.inbox
	}
	if models.Agent != nil {
		h.llm = models.Agent
		h.runner = assistant.NewRunner(models.Agent, h.store, assistant.RunnerConfig{MaxSteps: models.MaxSteps})
	}
	h.keys = models.Keys
	h.maxPrompt = models.MaxPrompt
	if h.maxPrompt < 1 {
		h.maxPrompt = defaultAssistantMaxPrompt
	}
	return h
}

// register mounts the assistant. The extension's side panel holds conversations too and cannot
// send the httpOnly cookie across origins, so the gate is `key`, which resolves the cookie, the
// connect-flow session JWT, or a full-scope API key to one user. Every signed-in user reaches
// the assistant. A turn is attributed (the caller's own gateway credential, tagged with its
// preset) and bounded: it draws on the caller's plan before the stream opens, and a spent
// allowance is a 402.
func (h *assistantHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Post("/assistant/sessions", mw.key, contract)
	api.Get("/assistant/sessions", mw.key, contract)
	api.Get("/assistant/sessions/:id", mw.key, contract)
	api.Delete("/assistant/sessions/:id", mw.key, contract)
	api.Post("/assistant/sessions/:id/messages", mw.key, mw.paidAI(plan.FeatureAssistant), h.PostAssistantMessage)
	// Read a PDF to attach to the next message. No model call, so no paidAI gate: the turn that
	// carries the text is what is metered.
	api.Post("/assistant/attachments", mw.key, h.PostAssistantAttachment)
	api.Post("/assistant/sessions/:id/cancel", mw.key, contract)
	// Buy this tailoring session another ceiling's worth of turns. Separate from starting a
	// session: a distinct decision the candidate makes after seeing the work so far.
	api.Post("/assistant/sessions/:id/extend", mw.key, contract)
	api.Post("/assistant/sessions/:id/opening", mw.key, mw.paidAI(plan.FeatureAssistant), h.PostAssistantOpening)
	api.Post("/assistant/sessions/:id/sync", mw.key, mw.paidAI(plan.FeatureAssistant), h.PostAssistantSync)
	// Resume after a transport/model failure without appending another user message —
	// re-sending the prompt would duplicate it in the model's context.
	api.Post("/assistant/sessions/:id/retry", mw.key, mw.paidAI(plan.FeatureAssistant), h.PostAssistantRetry)
	// Voice mode: mint one call's credential, then append each completed spoken turn
	// as it finishes. The audio itself never transits here — see PostAssistantVoiceToken.
	api.Post("/assistant/sessions/:id/voice-token", mw.key, mw.paidAI(plan.FeatureAssistant), contract)
	api.Post("/assistant/sessions/:id/voice-turns", mw.key, contract)
	// Cookie-only: an unattended run rewrites a CV, and the browser is the only place
	// the candidate can watch it happen and undo it.
	api.Post("/assistant/sessions/:id/autopilot", mw.cookie, mw.paidAI(plan.FeatureTailor), h.PostAssistantAutopilot)

	// The external auto-apply pipeline's own two endpoints: API key or cookie, never a session id
	// from the caller. See auto_apply_tailor.go for why these are not variants of /autopilot.
	api.Get("/me/auto-apply", mw.key, contract)
	api.Post("/me/auto-apply/:queueId/tailor", mw.autoApplyGate, h.PostAutoApplyTailor)
	api.Post("/me/auto-apply/:queueId/review", mw.autoApplyGate, contract)

	// The candidate-facing trigger for the sequence above: cookie-only, never mw.key or
	// mw.autoApplyGate (see auto_apply_enqueue.go).
	api.Post("/jobs/:slug/auto-apply", mw.cookie, mw.paidAI(plan.FeatureAutoApply), contract)
	// Retry and cancel an existing attempt. Cookie-only like the start route: the one other door
	// is the signed-in MCP server behind agents_may_apply, and an API key would skip it.
	api.Post("/jobs/:slug/auto-apply/retry", mw.cookie, contract)
	api.Delete("/jobs/:slug/auto-apply", mw.cookie, contract)
}

// defaultAssistantMaxPrompt bounds one user message when ASSISTANT_MAX_PROMPT is unset.
const defaultAssistantMaxPrompt = 8000

// PostAssistantMessage runs one turn and streams it as SSE, each loop frame (prompt, answer and
// reasoning deltas, tool calls and results, token usage, one terminal result) as a named event.
// A write failure only means this reader stopped: the turn runs to its own end, and stopping it
// is a separate request (see CancelAssistantTurn).
func (h *assistantHandlers) PostAssistantMessage(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	var in assistantTurnRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	prompt, docs, err := parseTurnRequest(in, h.maxPrompt)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return h.streamTurn(c, sess, prompt, docs, turnBounds(sess, ""))
}

// PostAssistantRetry resumes a session after a model/transport failure without recording
// another user message: re-posting would duplicate the prompt in the transcript and the model's
// context. Continue keeps the history (healing a dangling tool_use) and runs the loop again.
func (h *assistantHandlers) PostAssistantRetry(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	transcript, err := h.store.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	// Asked of the role, not the text: a message that was only an attached document has no text
	// and is still something to retry.
	if !slices.ContainsFunc(transcript, func(m assistant.Message) bool { return m.Role == assistant.RoleUser }) {
		return fiber.NewError(fiber.StatusConflict, "nothing to retry — send a message first")
	}
	turn := turnBounds(sess, lastUserText(transcript))
	return h.streamContinue(c, sess, turn)
}

// lastUserText returns the text of the most recent user message, or "" when none.
func lastUserText(transcript []assistant.Message) string {
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role != assistant.RoleUser {
			continue
		}
		var c struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(transcript[i].Content, &c); err != nil {
			return ""
		}
		return c.Text
	}
	return ""
}

// userLanguage reads the caller's preferred interface language for the turn's system prompt
// (see assistant.SystemPrompt). Best-effort: a lookup failure falls back to "en".
func (h *assistantHandlers) userLanguage(ctx context.Context, userID int64) string {
	lang, err := h.queries.GetUserLanguage(ctx, userID)
	if err != nil {
		log.Printf("assistant: language for user %d: %v", userID, err)
		return "en"
	}
	return lang
}

// effectivePreset resolves the preset a turn actually runs under. A browse session's
// read_current_page only works over the extension's own connection (browsertools.Hub is keyed
// by user id, not session id), so reached any other way the session must run as an ordinary chat.
//
// Called ONCE, before the prompt and the registry are built, and both use its answer: deciding
// twice could disagree (assistant.NormalizePreset's doc), and a model told to open with a tool it
// was never given just reports "unknown tool".
func effectivePreset(preset string, asExtension bool) string {
	if assistant.NormalizePreset(preset) == assistant.PresetBrowse && !asExtension {
		return assistant.PresetChat
	}
	return preset
}

// boundRunner returns the runner for one turn, spending under the session owner's own gateway
// credential.
//
// The turn is tagged with the preset as well as the feature: rehearsal, unattended tailoring and
// a question cost wildly different amounts, and the gateway files one spend row per tag. It
// resolves before the stream opens, since minting is a network call that would stall a stream
// already being read.
//
// The nil check is load-bearing: a nil *llm.Client assigned into the Model INTERFACE is not a
// nil interface, so the runner would dereference it on the first round, panicking in the stream
// goroutine after the response began.
//
// It goes through the user-call binding, so it fails closed on a context the paid-AI gate did
// not mark, before any stream opens.
func (h *assistantHandlers) boundRunner(ctx context.Context, sess assistant.Session) (*assistant.Runner, error) {
	if sess.Preset == assistant.PresetSupport {
		return h.boundSupportRunner(ctx, sess)
	}
	bound, err := h.binding().bind(ctx, sess.UserID, llm.Feature(tagAssistant), llm.Dimension{Name: "preset", Value: sess.Preset})
	if err != nil {
		return nil, err
	}
	if bound == nil {
		return h.runner, nil
	}
	return h.runner.With(bound), nil
}

// binding is this surface's model client and credential resolver as the one user-call binding
// every handler spends through (user_llm.go). Two fields because the assistant also needs the
// bare client; assembling it where used keeps the assistant from reaching llmkey around the paid check.
func (h *assistantHandlers) binding() llmBinding { return llmBinding{client: h.llm, keys: h.keys} }

// ownedSession resolves the :id route param to a session the caller owns.
func (h *assistantHandlers) ownedSession(c *fiber.Ctx) (assistant.Session, error) {
	return h.ownedSessionByID(c.Context(), c.Params("id"))
}

// ownedSessionByID is ownedSession for a contract handler, which holds the raw id.
func (h *assistantHandlers) ownedSessionByID(ctx context.Context, rawID string) (assistant.Session, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return assistant.Session{}, err
	}
	id, err := parseAssistantSessionID(rawID)
	if err != nil {
		return assistant.Session{}, err
	}
	sess, err := h.store.Session(ctx, id, userID)
	if err != nil {
		return assistant.Session{}, mapAssistantError(err)
	}
	if sess.Preset == assistant.PresetSupport {
		return assistant.Session{}, errSessionNotFound
	}
	return sess, nil
}

// mapAssistantError renders a store failure as HTTP. A session the caller does not own is a
// 404, indistinguishable from one that never existed, or the ids become probeable.
func mapAssistantError(err error) error {
	if errors.Is(err, assistant.ErrNotFound) {
		return errSessionNotFound
	}
	return err
}

var errSessionNotFound = fiber.NewError(fiber.StatusNotFound, "session not found")
