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
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/assistant"
)

// assistantMaxPrompt bounds one user message. The agent's context is finite and a
// pasted job board is not a question; the limit is generous for prose.
const assistantMaxPrompt = 8000

// assistantKeepalive is how often a silent turn emits an SSE comment. A model
// thinking for a minute would otherwise let nginx's read timeout sever the
// stream, and the client would show a bare "connection lost" mid-answer.
const assistantKeepalive = 15 * time.Second

// sessionResponse is the wire shape of one conversation.
type sessionResponse struct {
	ID        string `json:"id"`
	Preset    string `json:"preset"`
	Label     string `json:"label"`
	CVID      *int64 `json:"cv_id,omitempty"`
	JobID     *int64 `json:"job_id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// sessionView renders a session for the client. The id is a string because the
// client treats session ids as opaque and a CV stores its bound session id as text.
func sessionView(s assistant.Session) sessionResponse {
	return sessionResponse{
		ID:     assistantSessionID(s.ID),
		Preset: s.Preset,
		Label:  s.Label,
		CVID:   s.CVID,
		JobID:  s.JobID,
	}
}

func assistantSessionID(id int64) string { return fmt.Sprintf("%d", id) }

// CreateAssistantSession starts a new chat conversation for the caller. Tailoring
// sessions are created by the tailoring bootstrap, which knows the CV and vacancy
// to bind; this endpoint deliberately cannot mint one.
func (a *API) CreateAssistantSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	sess, err := a.assistant.CreateSession(c.Context(), userID, assistant.PresetChat, nil, nil)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": sessionView(sess)})
}

// ListAssistantSessions returns the caller's conversations, newest activity first.
func (a *API) ListAssistantSessions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	sessions, err := a.assistant.Sessions(c.Context(), userID)
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
func (a *API) GetAssistantSession(c *fiber.Ctx) error {
	sess, err := a.ownedSession(c)
	if err != nil {
		return err
	}
	messages, err := a.assistant.Transcript(c.Context(), sess.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"session":  sessionView(sess),
		"messages": messages,
	}})
}

// DeleteAssistantSession removes an owned conversation and its transcript.
func (a *API) DeleteAssistantSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := a.assistant.DeleteSession(c.Context(), id, userID); err != nil {
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
func (a *API) PostAssistantMessage(c *fiber.Ctx) error {
	sess, err := a.ownedSession(c)
	if err != nil {
		return err
	}
	if a.assistantRunner == nil {
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

	registry := a.assistantRegistry(sess)
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

		err := a.assistantRunner.Run(ctx, sess, registry, system, prompt, func(e assistant.Event) {
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
			log.Printf("assistant: turn failed session=%d: %v", sess.ID, err)
		}
	}))
	return nil
}

// ownedSession resolves the :id route param to a session the caller owns.
func (a *API) ownedSession(c *fiber.Ctx) (assistant.Session, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	id, err := pathID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	sess, err := a.assistant.Session(c.Context(), id, userID)
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
