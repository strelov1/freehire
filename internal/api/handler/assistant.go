package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/browsertools"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// assistantHandlers is the in-app agent. It owns the conversation store and the
// turn runner, and reaches the other feature handlers for the services its tools
// call — the assistant is a facade over the same use cases the HTTP surface
// exposes, so it must not grow a second copy of any of them.
type assistantHandlers struct {
	store  *assistant.Store
	runner *assistant.Runner

	// llm is the agent's own client, kept beside the runner so a turn can be re-bound
	// to the caller's gateway credential. The runner holds it as an interface and
	// cannot be asked for it back.
	llm *llm.Client
	// keys resolves the credential a turn spends under. Nil in a deployment that does
	// not attribute spend, which every path treats as "spend on the service credential".
	keys *llmkey.Resolver
	// maxPrompt is the rune ceiling on one user message (ASSISTANT_MAX_PROMPT).
	maxPrompt int

	// turns are the turns running right now, so a cancel request can reach one that is
	// streaming on a goroutine no request owns any more, and so a session keeps to one turn
	// at a time. Its zero value works; see assistant_turns.go.
	turns turnRegistry

	queries *db.Queries
	// fit reads the cached fit analysis the tailoring and interview tools ground on. Held
	// directly rather than reached for through the CV handlers: these are use cases, and a
	// tool must not depend on which HTTP surface happens to assemble them.
	fit *fitanalysis.Service
	// jobs serves the vacancy a tool is about — one row by id, the same narrow read the CV
	// surface takes.
	jobs     jobReader
	search   *searchHandlers
	resume   *resumeHandlers
	tracking *trackingHandlers
	cv       *cvHandlers
	// profile backs the get_profile tool. Reusing the profile handlers rather than the
	// services underneath keeps the tool and GET /me/profile on one assembly, so the
	// agent cannot drift from what the profile page shows.
	profile *profileHandlers
	// letter serves cover_letter_draft. It is the SAME store, chain and bank the HTTP endpoint
	// holds, which is what stops the chat path and the button path from drifting into two
	// different letters for one pair. The bank is deliberately NOT reviewableResume's
	// file-only structure: a letter speaks for the candidate, where the ATS report judges
	// their document. Nil-safe.
	letter coverLetterDeps
	// experience backs the bank tools, which every preset offers.
	experience experienceBankTools
	// screeningAnswers backs screening_answers_set, which every preset offers for the
	// same reason the bank tools do: a candidate states their notice period or salary
	// expectation whenever it comes up, not on a schedule.
	screeningAnswers *screeninganswers.Store
	// mail backs the inbox tools, which only the general chat preset offers. The
	// tools reach through it to inbox.Service — the same use cases /me/inbox and
	// /me/emails render, so a rule cannot hold for one reader and not the other.
	mail *inboxHandlers
	// browserTools backs read_current_page in a browsing session. It is the same hub
	// the agentic autofill drives, so the assistant is a second in-process harness on
	// the user's channel rather than a second wire to their browser.
	browserTools *browsertools.Hub
	// stages and invitation back the context a rehearsal and a debrief share. They are
	// the two narrow reads the application-bound presets need and no other preset does:
	// which stage the application is at, and what the employer said when they invited
	// the candidate.
	stages     applicationReader
	invitation invitationReader

	// plans meters a turn against the caller's plan. Nil leaves every turn unmetered,
	// which is what a fixture exercising an adjacent surface gets — production always
	// wires one.
	plans *plan.Store

	// realtime mints voice-mode's short-lived OpenAI Realtime API credentials. Nil in
	// a deployment with no Realtime gateway configured, following internal/ai/speech's
	// convention: the mint endpoint answers 501 and the interview session view offers
	// no voice mode, rather than the composer discovering the absence on first use.
	realtime realtimeMinter

	// events best-effort publishes auto-apply/review.decided so a paused
	// cmd/auto-apply-orchestrate run resumes (openspec/changes/auto-apply-inngest-orchestration).
	// Nil is the unconfigured deployment: PostAutoApplyReview then publishes nothing,
	// exactly as it did before this existed.
	events autoApplyEventPublisher
}

// assistantModels names the model client the assistant runs on and the resolver that
// bills it.
type assistantModels struct {
	// Agent is the model the turn loop runs on. Nil leaves the runner nil: old
	// conversations stay readable and a new turn reports the assistant as unavailable,
	// rather than the whole surface disappearing.
	Agent *llm.Client
	// Keys names the caller on the gateway. Nil is ordinary rather than degraded: every
	// turn then spends on the service credential, exactly as it did before attribution.
	Keys      *llmkey.Resolver
	MaxSteps  int
	MaxPrompt int
}

// newAssistantHandlers wires the agent.
func newAssistantHandlers(queries *db.Queries, models assistantModels, store *assistant.Store,
	search *searchHandlers, resumeH *resumeHandlers, tracking *trackingHandlers, cvH *cvHandlers,
	profileH *profileHandlers, browserTools *browsertools.Hub, mail *inboxHandlers,
	bank experienceBankTools, screeningAnswers *screeninganswers.Store, plans *plan.Store, letter coverLetterDeps) *assistantHandlers {
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
		browserTools:     browserTools,
		mail:             mail,
		stages:           queries,
		plans:            plans,
	}
	// The fit reads come from the service, not from whichever handler happens to hold it:
	// a tool that grounds on the cached analysis must not break because the CV surface was
	// reshaped.
	if cvH != nil {
		h.fit = cvH.fit
	}
	// The rehearsal reads the invitation through the mail service, not through the store:
	// the guarantee that this read leaves read_at alone is inbox's, and reaching past it
	// would put a second reader outside that rule.
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

// register mounts the assistant. A browser drives it, but not always the web app:
// the extension's side panel holds conversations too, and it cannot send hire's
// httpOnly cookie across origins — so the gate is `key`, which resolves the cookie,
// the session JWT the connect flow minted, or a full-scope API key to one user.
// Authentication is the whole gate: every signed-in user reaches the assistant. The
// beta-tester restriction that used to sit here was written when the agent ran on the
// candidate's own machine, and did not survive the move in-process. A turn is both
// attributed — it spends on the caller's own gateway credential, tagged with its preset —
// and now BOUNDED: it draws on the caller's plan before the stream opens, and a spent
// allowance is a 402. Measured over three August weeks, the assistant was 99.4% of model
// spend and the only surface charging nothing for it.
func (h *assistantHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/assistant/sessions", mw.key, h.CreateAssistantSession)
	api.Get("/assistant/sessions", mw.key, h.ListAssistantSessions)
	api.Get("/assistant/sessions/:id", mw.key, h.GetAssistantSession)
	api.Delete("/assistant/sessions/:id", mw.key, h.DeleteAssistantSession)
	api.Post("/assistant/sessions/:id/messages", mw.key, h.PostAssistantMessage)
	api.Post("/assistant/sessions/:id/cancel", mw.key, h.CancelAssistantTurn)
	// Buy this tailoring session another ceiling's worth of turns. Separate from starting
	// a session because it is a distinct decision the candidate makes with what they know
	// now — they have seen the work so far and are choosing to spend more of the day on it.
	api.Post("/assistant/sessions/:id/extend", mw.key, h.PostAssistantExtend)
	api.Post("/assistant/sessions/:id/opening", mw.key, h.PostAssistantOpening)
	// Resume after a transport/model failure without appending another user message —
	// re-sending the prompt would duplicate it in the model's context.
	api.Post("/assistant/sessions/:id/retry", mw.key, h.PostAssistantRetry)
	// Voice mode: mint one call's credential, then append each completed spoken turn
	// as it finishes. The audio itself never transits here — see PostAssistantVoiceToken.
	// The limiter guards call STARTS, the same shape as speech's transcriptionsPerHour;
	// it is not a substitute for the client-side call-duration cap, which guards one
	// call's length.
	api.Post("/assistant/sessions/:id/voice-token", mw.key, perCallerLimiter(voiceTokensPerHour), h.PostAssistantVoiceToken)
	api.Post("/assistant/sessions/:id/voice-turns", mw.key, h.PostAssistantVoiceTurn)
	// Cookie-only: an unattended run rewrites a CV, and the browser is the only place
	// the candidate can watch it happen and undo it.
	api.Post("/assistant/sessions/:id/autopilot", mw.cookie, h.PostAssistantAutopilot)

	// The external auto-apply pipeline's own two endpoints (openspec/changes/
	// auto-apply-tailored-resume) — API key or cookie, never a session id from the caller.
	// See auto_apply_tailor.go's own doc comment for why these are not variants of
	// /autopilot above.
	api.Post("/me/auto-apply/:queueId/tailor", mw.autoApplyGate, h.PostAutoApplyTailor)
	api.Post("/me/auto-apply/:queueId/review", mw.autoApplyGate, h.PostAutoApplyReview)

	// The candidate-facing trigger for the sequence above (openspec/changes/
	// auto-apply-submit-trigger) — cookie-only, never mw.key or mw.autoApplyGate: see
	// auto_apply_enqueue.go's own doc comment for why.
	api.Post("/jobs/:slug/auto-apply", mw.cookie, h.PostJobAutoApply)
}

// defaultAssistantMaxPrompt bounds one user message when ASSISTANT_MAX_PROMPT is
// unset. The agent's context is finite and a pasted job board is not a question.
const defaultAssistantMaxPrompt = 8000

// sessionResponse is the wire shape of one conversation.
type sessionResponse struct {
	ID     string  `json:"id"`
	Preset string  `json:"preset"`
	Label  string  `json:"label"`
	CVID   *string `json:"cv_id,omitempty"`
	JobID  *int64  `json:"job_id,omitempty"`
}

// sessionView renders a session for the client. The id is a string because the
// client treats session ids as opaque and a CV stores its bound session id as text.
func sessionView(s assistant.Session) sessionResponse {
	return sessionResponse{
		ID:     s.ID.String(),
		Preset: s.Preset,
		Label:  s.Label,
		CVID:   cvIDString(s.CVID),
		JobID:  s.JobID,
	}
}

// assistantSessionID parses the :id route param. A session id is a UUID, so
// anything else cannot name a session at all — report it as missing rather than as
// a bad request, keeping "not yours" and "not a session" one indistinguishable
// answer.
// cvIDString renders a tailoring session's CV binding for the wire. A CV id is a
// UUID the client treats as opaque, so it travels as a string like the session's
// own id.
func cvIDString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func assistantSessionID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return id, nil
}

// CreateAssistantSession starts a new unbound conversation for the caller — a general
// chat, the experience interviewer with `?preset=profile`, or a browsing session held
// from the extension's side panel with `?preset=browse`.
//
// Only the presets that bind to NOTHING can be minted here. A tailoring session is
// created by the tailoring bootstrap, which knows the CV and the vacancy to bind; letting
// this endpoint name that preset would produce a session whose tools point at nothing.
func (h *assistantHandlers) CreateAssistantSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	preset, err := creatablePreset(c.Query("preset"))
	if err != nil {
		return err
	}
	// The creatable presets that bind to something all bind to the same thing: an
	// application. The client may name the vacancy because that binding is one the caller
	// already owns, and the server checks it rather than taking their word for it.
	var jobID *int64
	var vacancy db.Job
	if bindsToApplication(preset) {
		job, err := h.applicationVacancy(c, userID)
		if err != nil {
			return err
		}
		vacancy, jobID = job, &job.ID
	}
	sess, err := h.store.CreateSession(c.Context(), userID, preset, nil, jobID)
	if err != nil {
		return err
	}
	// Name it after its vacancy now, while we hold it. A session is otherwise named from
	// its first user message — which for these presets is the server's own brief,
	// identical every time, so every such session in the rail would carry the same string
	// and none would say which interview it was.
	if label := applicationSessionLabel(preset, vacancy); label != "" {
		if err := h.store.LabelSession(c.Context(), sess.ID, sess.UserID, label); err != nil {
			log.Printf("assistant: could not name %s session %s: %v", preset, sess.ID, err)
		} else {
			sess.Label = label
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": sessionView(sess)})
}

// creatablePreset resolves the preset a client asked for. Naming the value it rejected
// and the ones it accepts matters: this endpoint serves the web app and the extension
// both, and a bare 400 leaves a client author guessing.
//
// Tailoring is the one preset that cannot be minted here, because its binding is a CV
// that does not exist yet — the tailoring bootstrap creates both together. A rehearsal
// and a debrief bind to an application the caller already has, so naming one is safe.
//
// The accepted list is a slice rather than a switch's case list because the refusal has
// to recite it, and a preset added to one and forgotten in the other is a 400 that lies
// about what the client may ask for.
var creatablePresets = []string{
	assistant.PresetChat,
	assistant.PresetProfile,
	assistant.PresetBrowse,
	assistant.PresetInterview,
	assistant.PresetDebrief,
}

func creatablePreset(asked string) (string, error) {
	if asked == "" {
		return assistant.PresetChat, nil
	}
	if slices.Contains(creatablePresets, asked) {
		return asked, nil
	}
	return "", fiber.NewError(fiber.StatusBadRequest,
		fmt.Sprintf("preset %q cannot be created here; use one of %q — a tailoring session is created from its CV",
			asked, creatablePresets))
}

// bindsToApplication answers whether a preset is held against one application. Both
// such presets carry a vacancy and no CV, and both are minted by naming a slug the
// caller has applied to.
//
// The stage is deliberately not part of the answer. A debrief belongs after an
// interview, but a candidate who sat one and never moved their application's stage is
// exactly who it is for; where the button appears is the client's judgement, and
// refusing them here would be a bug wearing a rule's clothes.
func bindsToApplication(preset string) bool {
	return preset == assistant.PresetInterview || preset == assistant.PresetDebrief
}

// applicationSessionLabel names such a session in the rail: what it is, the role, and
// the company when the posting carries one. Empty for a vacancy with no title, which
// leaves the ordinary naming-from-the-first-message in place rather than writing a label
// that says nothing — and empty for any preset that is not bound to an application.
func applicationSessionLabel(preset string, job db.Job) string {
	var prefix string
	switch preset {
	case assistant.PresetInterview:
		prefix = "Interview: "
	case assistant.PresetDebrief:
		prefix = "Debrief: "
	default:
		return ""
	}
	title := strings.TrimSpace(job.Title)
	if title == "" {
		return ""
	}
	label := prefix + title
	if company := strings.TrimSpace(job.Company); company != "" {
		label += " · " + company
	}
	return label
}

// applicationVacancy resolves the `job_id` such a session is asked for, and refuses one
// the caller has no application against.
//
// The application row IS the authorisation: user_jobs holds one per (user, vacancy), so
// its absence answers "not yours" and "no such thing" with the same 404 — the same way a
// session the caller does not own is reported as missing.
func (h *assistantHandlers) applicationVacancy(c *fiber.Ctx, userID int64) (db.Job, error) {
	slug := strings.TrimSpace(c.Query("job"))
	if slug == "" {
		return db.Job{}, fiber.NewError(fiber.StatusBadRequest,
			"this conversation needs the `job` slug of an application you hold")
	}
	if h.stages == nil {
		return db.Job{}, fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// The vacancy is named by its public slug, like everywhere else on this API — the
	// numeric id is ours and never leaves the backend.
	job, err := h.stages.GetJobBySlug(c.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Job{}, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		return db.Job{}, err
	}
	if _, err := h.stages.GetUserJobStage(c.Context(),
		db.GetUserJobStageParams{UserID: userID, JobID: job.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Job{}, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		return db.Job{}, err
	}
	return job, nil
}

// ListAssistantSessions returns the caller's chat conversations, newest activity
// first. Tailoring conversations are not chats — each belongs to a CV — so they
// never appear here.
func (h *assistantHandlers) ListAssistantSessions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	sessions, err := h.store.ChatSessions(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionView(s))
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
}

// GetAssistantSession returns one owned conversation with its full transcript, so
// the client can repaint it through the same reducer live events fold through.
func (h *assistantHandlers) GetAssistantSession(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	messages, err := h.store.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"session":  sessionView(sess),
		"messages": messages,
	}})
}

// DeleteAssistantSession removes an owned conversation and its transcript.
func (h *assistantHandlers) DeleteAssistantSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := assistantSessionID(c)
	if err != nil {
		return err
	}
	if err := h.store.DeleteSession(c.Context(), id, userID); err != nil {
		return mapAssistantError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// CancelAssistantTurn stops the session's running turn.
//
// It exists because stopping used to be a side effect of the transport: the client aborted its
// read, the next write failed, and the turn was cancelled. That made a deliberate stop
// indistinguishable from a phone freezing its tab, and the turn paid for the confusion. Now the
// two are different things, and this is the one that means stop.
//
// A session with nothing running is not an error. A client cancels a turn whose end it cannot
// see; requiring it to first prove the turn is alive would ask it for the one fact it does not
// have. The ownership check comes first either way, so an id cannot be probed for existence.
func (h *assistantHandlers) CancelAssistantTurn(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := assistantSessionID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Session(c.Context(), id, userID); err != nil {
		return mapAssistantError(err)
	}
	h.turns.cancel(id)
	return c.SendStatus(fiber.StatusNoContent)
}

// assistantTurnRequest is the body of a turn: the user's message.
type assistantTurnRequest struct {
	Text string `json:"text"`
}

// PostAssistantMessage runs one turn and streams it as SSE. Every frame the loop
// produces — the recorded prompt, answer and reasoning deltas, each tool call and
// its result, the token usage, and exactly one terminal result — is written as a
// named event. A write failure means only that this reader stopped reading: the turn runs to
// its own end regardless, and stopping it is a separate request (see CancelAssistantTurn).
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
	prompt := strings.TrimSpace(in.Text)
	if prompt == "" {
		return fiber.NewError(fiber.StatusBadRequest, "text is required")
	}
	if len([]rune(prompt)) > h.maxPrompt {
		return fiber.NewError(fiber.StatusBadRequest, "message is too long")
	}
	return h.streamTurn(c, sess, prompt, turnBounds(sess, ""))
}

// PostAssistantRetry resumes a session after a model/transport failure without recording
// another user message. Re-posting the same prompt would leave two copies in the
// transcript and in the model's context; Continue keeps the history as it is (healing
// any dangling tool_use from the interrupted turn) and runs the loop again.
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
	if lastUserText(transcript) == "" {
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

// streamTurn runs one turn and writes it to the response as SSE. It is shared by every
// entry point that starts a turn, so the stream's shape — the headers, the cleared write
// deadline and the keepalive — is written once.
//
// The prompt and the turn's bounds are the caller's arguments rather than the request's
// body: an unattended run's brief and its raised ceiling are ours to choose, and a ceiling
// a client can set is not a bound.
func (h *assistantHandlers) streamTurn(c *fiber.Ctx, sess assistant.Session, prompt string, turn assistant.TurnConfig) error {
	return h.streamSSE(c, sess, h.meterTurn, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		return runner.Run(ctx, sess, reg, system, prompt, turn, emit)
	})
}

// streamContinue is streamTurn for a retry: same SSE machinery, no new prompt.
func (h *assistantHandlers) streamContinue(c *fiber.Ctx, sess assistant.Session, turn assistant.TurnConfig) error {
	return h.streamSSE(c, sess, h.meterRetry, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		return runner.Continue(ctx, sess, reg, system, turn, emit)
	})
}

// userLanguage reads the caller's preferred interface language for the turn's system
// prompt (see assistant.SystemPrompt). Best-effort: a lookup failure falls back to
// "en" rather than failing the turn — the language directive is worth getting right,
// not worth a 500 over.
func (h *assistantHandlers) userLanguage(ctx context.Context, userID int64) string {
	lang, err := h.queries.GetUserLanguage(ctx, userID)
	if err != nil {
		log.Printf("assistant: language for user %d: %v", userID, err)
		return "en"
	}
	return lang
}

// effectivePreset resolves the preset a turn actually runs under. A browse session's
// one distinguishing tool, read_current_page, only works over the extension's own
// connection (internal/ai/browsertools.Hub is keyed by user id, not session id, so ANY
// turn of the caller's could otherwise reach a browser they never opened on this
// surface) — reached any other way, the session must run as an ordinary chat.
//
// This is called ONCE, before either the prompt or the registry is built, and both
// are built from its answer rather than from sess.Preset directly. Deciding the same
// question twice — once for the prompt, once for the tools — is exactly what
// assistant.NormalizePreset's own doc comment warns against: the two could disagree,
// and a model told to always open by calling a tool it was never given just reports
// "unknown tool" instead of running the plain chat it degraded to.
func effectivePreset(preset string, asExtension bool) string {
	if assistant.NormalizePreset(preset) == assistant.PresetBrowse && !asExtension {
		return assistant.PresetChat
	}
	return preset
}

// streamSSE owns the SSE headers, turn claim/queue, keepalive and writer. start is the
// one difference between a fresh message and a retry, and meter is the difference between
// paying for a new turn and resuming one already paid for.
func (h *assistantHandlers) streamSSE(
	c *fiber.Ctx,
	sess assistant.Session,
	meter func(*fiber.Ctx, assistant.Session) (turnCharge, bool, error),
	start func(context.Context, *assistant.Runner, *assistant.Registry, string, func(assistant.Event)) error,
) error {
	asExtension := auth.IsExtensionBearer(c)
	turnSess := sess
	turnSess.Preset = effectivePreset(sess.Preset, asExtension)

	// One batch per turn. Every CV edit the agent makes in this turn is filed under it, so
	// the history can group them and "undo the run" is undoing a batch — which is what
	// retires the single pre-run snapshot and the edge two concurrent runs used to create.
	registry := h.registry(turnSess, uuid.New())
	system := assistant.SystemPrompt(turnSess.Preset, h.userLanguage(c.Context(), sess.UserID))

	// Tagged from turnSess, not sess: a demoted browse turn spends like a chat turn — no
	// page tool, the chat prompt — so it must be billed as one. Tagging it "browse" would
	// pollute that preset's cost bucket with turns that never touched its one distinguishing
	// tool, defeating the reason the tag exists (AGENTS.md: "the preset is what makes them
	// comparable").
	turnRunner := h.boundRunner(c.Context(), turnSess)

	// Metered BEFORE the headers go out. A refusal has to be a real status: an error frame
	// inside a stream that already answered 200 is invisible to anything checking status
	// codes, and the SPA would render an empty answer where an upgrade prompt belongs.
	//
	// Charged against turnSess, not sess, for the same reason the spend tag is: a browse
	// turn demoted to chat IS a chat turn, and it must draw on the allowance it actually
	// spends against.
	charge, refused, err := meter(c, turnSess)
	if refused {
		return err
	}

	sseHeaders(c)

	// The server's write timeout would kill this long-lived stream mid-turn, so the SSE
	// response carries its own, bounded deadline instead (captured while the request ctx
	// is valid; used inside the writer, which runs after this handler returns).
	conn := c.Context().Conn()

	// Same reason as conn: the sentryfiber hub is request-scoped and the ctx is released
	// once this handler returns, so take a clone now — the writer outlives the request
	// that owns its scope.
	var hub *sentry.Hub
	if reqHub := sentryfiber.GetHubFromContext(c); reqHub != nil {
		hub = reqHub.Clone()
	}

	// The turn runs on its own cancellable context: the request's own is released the moment
	// this handler returns, long before the turn ends. It is created HERE rather than inside
	// the writer so the session's slot is taken while the request can still be answered — a
	// slot taken after the handler returned could not refuse anything.
	ctx, cancel := context.WithCancel(context.Background())
	slot, waiter, err := h.turns.claim(sess.ID, cancel)
	if err != nil {
		cancel()
		// The charge was taken before the headers, which is the only place a refusal can
		// still be a status — but this turn never reaches the runner, so it owes the
		// allowance back. A 409 that also costs a message is the accounting refusing work
		// it declined to do.
		h.releaseTurn(turnSess, charge)
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// sseStream owns the write protocol both SSE endpoints need: it serializes the
		// heartbeat goroutine against the event callback (bufio.Writer is not safe for
		// concurrent use) and re-arms the write deadline before EVERY write. Per-write
		// rather than once up front because fasthttp runs this stream writer on its own
		// goroutine while the serving goroutine arms the server's WriteTimeout, so a single
		// set races with it and loses about half the time — the turn then dies at exactly
		// ten seconds, mid-answer, for no reason the user can see. Bounded rather than
		// cleared because a cleared deadline is forever: a reader that stopped reading
		// would block the write, and with it this goroutine, for the life of the process.
		stream := newSSEStream(w, conn, sseWriteTimeout)

		defer cancel()

		// One byte immediately, before anything this turn might wait on. Until something is
		// written, fasthttp holds the response line and the headers in its buffer, so "the
		// stream is open" is true here and not yet true on the wire — and a proxy times out
		// an upstream that has sent nothing, however busy it is. The heartbeat below cannot
		// cover this: its first comment is a whole interval away. A comment rather than an
		// event because EventSource ignores it — this says nothing to the client, it only
		// makes the connection real.
		stream.comment("open")

		// The heartbeat starts BEFORE the queue wait, not after it: a queued message can wait
		// up to a minute, and an idle connection is exactly what a proxy in front of us
		// collects. Waiting in silence would lose the connection at roughly the same moment
		// the wait was about to pay off.
		stopHeartbeat := stream.keepalive(sseKeepalive)
		defer stopHeartbeat()

		// A message that arrived while the session was busy waits here rather than running
		// beside the turn in flight — two turns of one tailoring session would edit one CV
		// from two conversations that cannot see each other. The client is told it is
		// waiting, because a stream that goes quiet for a minute is indistinguishable from
		// one that broke.
		if waiter != nil {
			stream.event(string(assistant.EventQueued), assistant.Event{Kind: assistant.EventQueued})

			waitCtx, giveUp := context.WithTimeout(ctx, turnQueueWait)
			slot, err = waiter.enter(waitCtx, cancel)
			giveUp()
			if err != nil {
				// The wait is over and this turn never started, so it gives back what it
				// took — a queued message that timed out behind another turn has produced
				// nothing, and that is exactly what the reservation exists to undo.
				h.releaseTurn(turnSess, charge)

				// Every stream owes its client one terminal event; without it the client
				// waits for a turn that is not coming.
				stream.event(string(assistant.EventResult),
					assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopCancelled})
				return
			}
		}
		defer h.turns.release(sess.ID, slot)

		err = start(ctx, turnRunner, registry, system, func(e assistant.Event) {
			// A write that fails means THIS reader is not listening: a phone froze its tab,
			// a tunnel dropped, a laptop slept. It does not mean the work should stop, and
			// treating it that way threw away live runs — a tailoring pass lost its report
			// after twenty-five committed edits because a tab went to the background.
			//
			// So the event is simply not delivered. The turn runs to its own end under the
			// bounds it already has (the step cap and the model timeout), the transcript is
			// persisted either way, and a client that comes back reads it from the session.
			// Stopping a turn is now something a caller ASKS for, through the cancel route.
			stream.event(string(e.Kind), e)
		})
		if err != nil {
			// The turn produced nothing the candidate can use, so it gives back what it
			// took. A model or transport failure is ours, not theirs, and charging for an
			// answer that never arrived is the one outcome the reservation exists to avoid.
			h.releaseTurn(turnSess, charge)

			// Nothing to continue is a client mistake (empty session), not a turn fault —
			// surface it as a conflict on the still-open stream rather than Sentry noise.
			if errors.Is(err, assistant.ErrNothingToContinue) {
				return
			}
			// The loop has already emitted its terminal error event; this is for us —
			// and for Sentry, which would otherwise never learn of it: this handler
			// returned nil long before the turn ran, so RenderError never sees the
			// failure and the access log keeps the 200 the stream opened with.
			log.Printf("assistant: turn failed session=%s: %v", sess.ID, err)
			reportStreamFault(hub, err)
		}
	}))
	return nil
}

// boundRunner returns the runner for one turn, spending under the session owner's own
// gateway credential.
//
// The turn is tagged with the preset as well as the feature: a rehearsal, an unattended
// tailoring pass and a question cost wildly different amounts, and the gateway files one
// spend row per tag, so the preset is what makes them comparable. It resolves before the
// stream opens — minting is a network call, and making it after the headers are out would
// stall a stream the client is already reading.
//
// The nil check is load-bearing and is not the same as letting Runner.With see a nil. A
// nil *llm.Client assigned into the Model INTERFACE is not a nil interface, so the runner
// would believe it has a model and dereference it on the first round. That is a panic in
// the stream goroutine, after the response has already begun.
func (h *assistantHandlers) boundRunner(ctx context.Context, sess assistant.Session) *assistant.Runner {
	bound := llmkey.Bind(ctx, h.keys, h.llm, sess.UserID, llm.Feature(tagAssistant), llm.Dimension{Name: "preset", Value: sess.Preset})
	if bound == nil {
		return h.runner
	}
	return h.runner.With(bound)
}

// The opening briefs. Like the autopilot's they are short on method and only say which
// conversation this is: how to open — read the context, name the vacancy, ask the first
// question — is in each preset's system prompt, stated once.
//
// They exist because a turn does not begin until a message arrives, and the candidate has
// nothing to type. They opened this from an application; asking them to introduce their
// own interview would be the questionnaire the assistant is supposed to save them.
const (
	rehearsalOpeningBrief = "Let's rehearse this interview. Read the context and tell me what you see, " +
		"then ask me which round to run."
	debriefOpeningBrief = "I've just had this interview. Read the context, then ask me what I was asked."
)

// openingBriefFor answers with the brief an application-bound preset opens under, and
// with the empty string for a preset that opens under none. The empty answer is what
// PostAssistantOpening refuses on, so the set of presets that can speak first is stated
// once rather than as a condition there and a constant here.
func openingBriefFor(preset string) string {
	switch preset {
	case assistant.PresetInterview:
		return rehearsalOpeningBrief
	case assistant.PresetDebrief:
		return debriefOpeningBrief
	default:
		return ""
	}
}

// PostAssistantOpening speaks first in a rehearsal or a debrief, streaming one ordinary
// turn under a server-side brief.
//
// It refuses a session whose opening has already been answered. The opening is the first
// turn of a conversation, and a reload that re-ran it would restart the interview over
// whatever the candidate had already said.
func (h *assistantHandlers) PostAssistantOpening(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if h.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// The vacancy as well as the preset: the context tool is registered only for a
	// session that carries one, so an unbound session would open on a tool it does not
	// have.
	brief := openingBriefFor(sess.Preset)
	if brief == "" || sess.JobID == nil {
		return fiber.NewError(fiber.StatusConflict, "this conversation does not open by itself")
	}
	transcript, err := h.store.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	// An ANSWERED opening, not any transcript at all. The runner records the brief before
	// it calls the model, so a turn that dies upstream — a 502 from the proxy is an
	// ordinary event — leaves exactly one message behind. Refusing on that would put the
	// session in a state it can never leave: a conversation holding one line the
	// candidate did not write, no opening, and no way to retry.
	for _, m := range transcript {
		if m.Role == assistant.RoleAssistant {
			return fiber.NewError(fiber.StatusConflict, "this conversation has already started")
		}
	}
	return h.streamTurn(c, sess, brief, assistant.TurnConfig{})
}

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
// runner default; interactive tailor and a resumed autopilot raise theirs server-side so
// a client cannot.
func turnBounds(sess assistant.Session, lastUser string) assistant.TurnConfig {
	if sess.Preset != assistant.PresetTailor {
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
		analysis, jobDescription = h.prepareAutopilotAnalysis(c, sess, job), job.Description
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
	analysis.ensure(ctx)
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

// prepareAutopilotAnalysis delegates to matchHandlers.prepareAutopilotRun, which assembles
// both halves of the run's fit analysis — the fill that gives cv_context something to read,
// and the refresh that follows the run — out of reads taken while c is still valid. A
// missing match surface degrades to a nil, whose ensure and refresh are no-ops: the same
// best-effort posture this step always had.
func (h *assistantHandlers) prepareAutopilotAnalysis(c *fiber.Ctx, sess assistant.Session, job db.Job) *autopilotAnalysis {
	if h.cv == nil || h.cv.match == nil {
		return nil
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

// ownedSession resolves the :id route param to a session the caller owns.
func (h *assistantHandlers) ownedSession(c *fiber.Ctx) (assistant.Session, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	id, err := assistantSessionID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	sess, err := h.store.Session(c.Context(), id, userID)
	if err != nil {
		return assistant.Session{}, mapAssistantError(err)
	}
	return sess, nil
}

// mapAssistantError renders a store failure as HTTP. A session the caller does not
// own is a 404, exactly like one that never existed — the two must stay
// indistinguishable or the ids become probeable.
func mapAssistantError(err error) error {
	if errors.Is(err, assistant.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return err
}
