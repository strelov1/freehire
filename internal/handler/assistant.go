package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/browsertools"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/llm"
)

// assistantHandlers is the in-app agent. It owns the conversation store and the
// turn runner, and reaches the other feature handlers for the services its tools
// call — the assistant is a facade over the same use cases the HTTP surface
// exposes, so it must not grow a second copy of any of them.
type assistantHandlers struct {
	store  *assistant.Store
	runner *assistant.Runner

	queries  *db.Queries
	search   *searchHandlers
	resume   *resumeHandlers
	tracking *trackingHandlers
	cv       *cvHandlers
	// profile backs the get_profile tool. Reusing the profile handlers rather than the
	// services underneath keeps the tool and GET /me/profile on one assembly, so the
	// agent cannot drift from what the profile page shows.
	profile *profileHandlers
	// experience backs the bank tools, which every preset offers.
	experience experienceBankTools
	// browserTools backs read_current_page in a browsing session. It is the same hub
	// the agentic autofill drives, so the assistant is a second in-process harness on
	// the user's channel rather than a second wire to their browser.
	browserTools *browsertools.Hub
}

// newAssistantHandlers wires the agent. A nil LLM client leaves the runner nil:
// old conversations stay readable and a new turn reports the assistant as
// unavailable, rather than the whole surface disappearing.
func newAssistantHandlers(queries *db.Queries, model *llm.Client, maxSteps int,
	search *searchHandlers, resumeH *resumeHandlers, tracking *trackingHandlers, cvH *cvHandlers,
	profileH *profileHandlers, browserTools *browsertools.Hub) *assistantHandlers {
	h := &assistantHandlers{
		experience:   experience.NewStore(experience.NewQueriesRepository(queries)),
		store:        assistant.NewStore(queries),
		queries:      queries,
		search:       search,
		resume:       resumeH,
		tracking:     tracking,
		cv:           cvH,
		profile:      profileH,
		browserTools: browserTools,
	}
	if model != nil {
		h.runner = assistant.NewRunner(model, h.store, assistant.RunnerConfig{MaxSteps: maxSteps})
	}
	return h
}

// register mounts the assistant. A browser drives it, but not always the web app:
// the extension's side panel holds conversations too, and it cannot send hire's
// httpOnly cookie across origins — so the gate is `key`, which resolves the cookie,
// the session JWT the connect flow minted, or a full-scope API key to one user.
// Every route stays behind the restricted rollout: inference is billed to us, so it
// is not open to everyone while it is free.
func (h *assistantHandlers) register(api fiber.Router, mw middleware) {
	gate := h.requireRollout
	api.Post("/assistant/sessions", mw.key, gate, h.CreateAssistantSession)
	api.Get("/assistant/sessions", mw.key, gate, h.ListAssistantSessions)
	api.Get("/assistant/sessions/:id", mw.key, gate, h.GetAssistantSession)
	api.Delete("/assistant/sessions/:id", mw.key, gate, h.DeleteAssistantSession)
	api.Post("/assistant/sessions/:id/messages", mw.key, gate, h.PostAssistantMessage)
}

// requireRollout admits moderators and beta testers. Membership is read fresh per
// request rather than from the token, so revoking it takes effect immediately, and
// a read failure fails closed — the assistant spends our money, so an unresolvable
// caller is refused rather than let through.
func (h *assistantHandlers) requireRollout(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	u, err := h.queries.GetUserByID(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
	if u.Role != "moderator" && !u.BetaTester {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}
	return c.Next()
}

// assistantMaxPrompt bounds one user message. The agent's context is finite and a
// pasted job board is not a question; the limit is generous for prose.
const assistantMaxPrompt = 8000

// assistantKeepalive is how often a silent turn emits an SSE comment. A model
// thinking for a minute would otherwise let nginx's read timeout sever the
// stream, and the client would show a bare "connection lost" mid-answer.
const assistantKeepalive = 15 * time.Second

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
	sess, err := h.store.CreateSession(c.Context(), userID, preset, nil, nil)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": sessionView(sess)})
}

// creatablePreset resolves the preset a client asked for, admitting only those that
// bind to nothing. Naming the value it rejected and the ones it accepts matters:
// this endpoint serves the web app and the extension both, and a bare 400 leaves a
// client author guessing.
func creatablePreset(asked string) (string, error) {
	switch asked {
	case "":
		return assistant.PresetChat, nil
	case assistant.PresetChat, assistant.PresetProfile, assistant.PresetBrowse:
		return asked, nil
	default:
		return "", fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("preset %q cannot be created here; use %q, %q or %q — a tailoring session is created from its CV",
				asked, assistant.PresetChat, assistant.PresetProfile, assistant.PresetBrowse))
	}
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

// assistantTurnRequest is the body of a turn: the user's message.
type assistantTurnRequest struct {
	Text string `json:"text"`
}

// PostAssistantMessage runs one turn and streams it as SSE. Every frame the loop
// produces — the recorded prompt, answer and reasoning deltas, each tool call and
// its result, the token usage, and exactly one terminal result — is written as a
// named event. A write failure means the client is gone, which cancels the turn's
// context so the loop stops before spending another model call.
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
	if len([]rune(prompt)) > assistantMaxPrompt {
		return fiber.NewError(fiber.StatusBadRequest, "message is too long")
	}

	registry := h.registry(sess)
	system := assistant.SystemPrompt(sess.Preset)

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no") // stop nginx buffering so events reach the browser promptly

	// The server's write timeout would kill this long-lived stream mid-turn, so the
	// deadline is cleared for the SSE response only (captured while the request ctx
	// is valid; used inside the writer, which runs after this handler returns).
	conn := c.Context().Conn()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// Clear the connection's write deadline before EVERY write, not once up
		// front: fasthttp runs this stream writer on its own goroutine while the
		// serving goroutine arms the server's WriteTimeout, so a single clear races
		// with it and loses about half the time — the turn then dies at exactly ten
		// seconds, mid-answer, for no reason the user can see.
		write := func(event string, data any) bool {
			if conn != nil {
				_ = conn.SetWriteDeadline(time.Time{})
			}
			return writeEvent(w, event, data)
		}
		// The request context is gone once the handler returns, so the turn runs on
		// its own cancellable context. Cancellation comes from the client going away,
		// which shows up as a failed write below.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The heartbeat goroutine and the event callback both write to w, which is not
		// safe for concurrent use.
		var mu sync.Mutex
		stop := make(chan struct{})
		var heartbeat sync.WaitGroup
		heartbeat.Add(1)
		go func() {
			defer heartbeat.Done()
			t := time.NewTicker(assistantKeepalive)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					mu.Lock()
					if conn != nil {
						_ = conn.SetWriteDeadline(time.Time{})
					}
					writeComment(w, "keepalive")
					mu.Unlock()
				}
			}
		}()

		err := h.runner.Run(ctx, sess, registry, system, prompt, assistant.TurnConfig{}, func(e assistant.Event) {
			mu.Lock()
			defer mu.Unlock()
			if !write(string(e.Kind), e) {
				// The client is gone. Stop the loop at its next boundary rather than
				// finishing a turn nobody is reading.
				cancel()
			}
		})
		close(stop)
		heartbeat.Wait()
		if err != nil {
			// The loop has already emitted its terminal error event; this is for us.
			log.Printf("assistant: turn failed session=%s: %v", sess.ID, err)
		}
	}))
	return nil
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

// writeEvent writes one named SSE event, reporting whether the write reached the
// client. Unlike writeSSE it does not swallow the failure: a dead connection is
// how a streamed turn learns to stop.
func writeEvent(w *bufio.Writer, event string, data any) bool {
	blob, err := json.Marshal(data)
	if err != nil {
		return true // an unencodable frame is our bug, not a dead client
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, blob); err != nil {
		return false
	}
	return w.Flush() == nil
}
