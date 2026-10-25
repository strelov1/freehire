//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

type applySessionView struct {
	Data struct {
		ID     string `json:"id"`
		Preset string `json:"preset"`
		Label  string `json:"label"`
		JobID  *int64 `json:"job_id"`
	} `json:"data"`
}

// The conversation about one automatic application is opened from that application, and
// opening it again from the same drawer returns the same conversation — a second one would
// split the numbered questions from the answers given to them.
func TestApplySessionIsOnePerApplication(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	userID, cookie := assistantUser(t, pool, iss, "apply-chat@example.test", true)
	jobID := seedApplication(t, pool, userID, "apply-1", "applied")

	open := func() (int, applySessionView) {
		resp := assistantRequest(t, app, fiber.MethodPost,
			"/api/v1/assistant/sessions?preset=apply&job=go-developer-apply-1", cookie, nil)
		defer resp.Body.Close()
		var got applySessionView
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.StatusCode, got
	}

	status, first := open()
	if status != fiber.StatusCreated {
		t.Fatalf("first open: status %d, want 201", status)
	}
	if first.Data.Preset != "apply" || first.Data.JobID == nil || *first.Data.JobID != jobID {
		t.Fatalf("first open = %+v, want an apply session bound to job %d", first.Data, jobID)
	}
	if first.Data.Label == "" {
		t.Error("the apply session is not named after its vacancy")
	}

	status, again := open()
	if status != fiber.StatusOK {
		t.Fatalf("second open: status %d, want 200 for the existing session", status)
	}
	if again.Data.ID != first.Data.ID {
		t.Errorf("second open returned session %s, want the existing %s", again.Data.ID, first.Data.ID)
	}
}

// The application row is the authorisation, as for a rehearsal.
func TestApplySessionNeedsTheCallersApplication(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "apply-chat-2@example.test", true)
	stranger, _ := assistantUser(t, pool, iss, "apply-chat-3@example.test", true)
	seedApplication(t, pool, stranger, "apply-2", "applied")

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=apply&job=go-developer-apply-2", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("apply session on another candidate's application: status %d, want 404", resp.StatusCode)
	}
}

// narrationModel answers with one line and counts how often it was asked; fail makes every
// call an upstream error.
type narrationModel struct {
	calls int
	fail  bool
}

func (m *narrationModel) Chat(_ context.Context, _ []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.calls++
	if m.fail {
		return nil, errors.New("upstream is down")
	}
	if s.OnText != nil {
		s.OnText("Your application is waiting for two answers.")
	}
	return &llms.ContentChoice{Content: "Your application is waiting for two answers."}, nil
}

func narratedCursor(t *testing.T, pool *pgxpool.Pool, sessionID string) *int64 {
	t.Helper()
	var cur *int64
	if err := pool.QueryRow(context.Background(),
		`SELECT narrated_event_id FROM assistant_sessions WHERE id = $1`, sessionID).Scan(&cur); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	return cur
}

// The chat catches up on what happened to the application while nobody was looking, once:
// a sync with nothing new costs nothing, and narrating is never charged to the candidate's
// allowance — they did not ask for it.
func TestApplySyncNarratesNewEventsOnce(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &narrationModel{}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "apply-sync@example.test", true)
	jobID := seedApplication(t, pool, userID, "apply-sync-1", "applied")
	sess, err := h.store.CreateSession(context.Background(), userID, "apply", nil, &jobID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sync := func() (int, string) {
		resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sess.ID.String()+"/sync", cookie, nil)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if status, _ := sync(); status != fiber.StatusNoContent {
		t.Fatalf("sync with no attempt: status %d, want 204", status)
	}
	if model.calls != 0 {
		t.Fatalf("nothing to narrate cost %d model calls", model.calls)
	}

	// Queueing the attempt records its first event (the 0196 trigger).
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2)`, userID, jobID); err != nil {
		t.Fatalf("queue attempt: %v", err)
	}

	status, body := sync()
	if status != fiber.StatusOK {
		t.Fatalf("sync with a new event: status %d, want 200", status)
	}
	if !strings.Contains(body, "waiting for two answers") {
		t.Errorf("the narration did not reach the stream: %s", body)
	}
	if model.calls != 1 {
		t.Errorf("model calls = %d, want 1", model.calls)
	}
	if narratedCursor(t, pool, sess.ID.String()) == nil {
		t.Error("the cursor did not advance past the narrated event")
	}

	if status, _ := sync(); status != fiber.StatusNoContent {
		t.Errorf("second sync: status %d, want 204 — the event was already narrated", status)
	}
	if model.calls != 1 {
		t.Errorf("the same event was narrated twice (model calls = %d)", model.calls)
	}

	var charges int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM usage_ledger WHERE user_id = $1 AND kind = 'consume'`, userID).Scan(&charges); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if charges != 0 {
		t.Errorf("narration was charged %d times to the candidate's allowance", charges)
	}
}

// A narration that never reached the candidate is narrated again on the next sync.
func TestApplySyncKeepsTheCursorWhenTheTurnFails(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &narrationModel{fail: true}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "apply-sync-2@example.test", true)
	jobID := seedApplication(t, pool, userID, "apply-sync-2", "applied")
	sess, err := h.store.CreateSession(context.Background(), userID, "apply", nil, &jobID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2)`, userID, jobID); err != nil {
		t.Fatalf("queue attempt: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sess.ID.String()+"/sync", cookie, nil)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if cur := narratedCursor(t, pool, sess.ID.String()); cur != nil {
		t.Errorf("the cursor advanced to %d past a narration that failed", *cur)
	}
}

// Only an apply conversation has an application to catch up on.
func TestSyncRefusesOtherPresets(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &narrationModel{})
	userID, cookie := assistantUser(t, pool, iss, "apply-sync-3@example.test", true)
	sess, err := h.store.CreateSession(context.Background(), userID, "chat", nil, nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sess.ID.String()+"/sync", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("sync on a chat session: status %d, want 409", resp.StatusCode)
	}
}
