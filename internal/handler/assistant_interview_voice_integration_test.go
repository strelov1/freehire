//go:build integration

// Integration tests for voice mode's two HTTP endpoints against a real Postgres:
// minting a session-scoped Realtime credential, and appending completed voice turns
// to the session transcript. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llm"
)

// realtimeStub stands in for internal/realtime.Client. It records the instructions it
// was asked to mint a secret for, so a test can prove the rehearsal context actually
// reached the request rather than only that the endpoint answered 200.
type realtimeStub struct {
	value        string
	err          error
	instructions string
	calls        int
}

func (s *realtimeStub) MintClientSecret(_ context.Context, instructions string) (string, error) {
	s.calls++
	s.instructions = instructions
	return s.value, s.err
}

func (s *realtimeStub) Model() string { return "gpt-realtime-2.1" }

func TestPostAssistantVoiceTokenReportsUnconfigured(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "voice-unconfigured@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-1", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-token", cookie, nil)
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("voice-token with no realtime client: status %d, want 501", resp.StatusCode)
	}
}

func TestPostAssistantVoiceTokenRefusesANonInterviewSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	h.realtime = &realtimeStub{value: "ek_should_not_be_used"}
	userID, cookie := assistantUser(t, pool, iss, "voice-wrong-preset@example.test", true)

	sess, _ := h.store.CreateSession(context.Background(), userID, "chat", nil, nil)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-token", cookie, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("voice-token on a chat session: status %d, want 409", resp.StatusCode)
	}
}

func TestPostAssistantVoiceTokenRefusesAnotherCandidatesSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	h.realtime = &realtimeStub{value: "ek_should_not_be_used"}
	owner, _ := assistantUser(t, pool, iss, "voice-owner@example.test", true)
	_, strangerCookie := assistantUser(t, pool, iss, "voice-stranger@example.test", true)

	jobID := seedApplication(t, pool, owner, "voice-2", "interview")
	sess, _ := h.store.CreateSession(context.Background(), owner, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-token", strangerCookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("voice-token for another candidate's session: status %d, want 404", resp.StatusCode)
	}
}

func TestPostAssistantVoiceTokenMintsAScopedSecret(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	stub := &realtimeStub{value: "ek_abc123"}
	h.realtime = stub
	userID, cookie := assistantUser(t, pool, iss, "voice-mint@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-3", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-token", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("voice-token: status %d", resp.StatusCode)
	}
	var got struct {
		Data struct {
			Value string `json:"value"`
			Model string `json:"model"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.Value != "ek_abc123" {
		t.Errorf("value = %q, want ek_abc123", got.Data.Value)
	}
	// The WebRTC SDP exchange names the model on its URL — the browser has to learn it
	// from here, not from a string hardcoded in the frontend that could drift from
	// REALTIME_MODEL.
	if got.Data.Model != "gpt-realtime-2.1" {
		t.Errorf("model = %q, want gpt-realtime-2.1", got.Data.Model)
	}
	if stub.calls != 1 {
		t.Fatalf("MintClientSecret called %d times, want 1", stub.calls)
	}
	// The rehearsal context has to have actually reached the mint call, not just the
	// endpoint answering — this is what proves task 3.1's instructions builder ran.
	if !containsAll(stub.instructions, "Go Developer", "Acme") {
		t.Errorf("minted instructions do not carry the vacancy:\n%s", stub.instructions)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestPostAssistantVoiceTurnAppendsToTheTranscript(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "voice-turn@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-4", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)
	path := "/api/v1/assistant/sessions/" + sess.ID.String() + "/voice-turns"

	first := assistantRequest(t, app, fiber.MethodPost, path, cookie,
		map[string]any{"role": "assistant", "content": "Which round would you like to rehearse?"})
	if first.StatusCode != fiber.StatusNoContent {
		t.Fatalf("append assistant turn: status %d", first.StatusCode)
	}
	second := assistantRequest(t, app, fiber.MethodPost, path, cookie,
		map[string]any{"role": "user", "content": "Let's do the technical round."})
	if second.StatusCode != fiber.StatusNoContent {
		t.Fatalf("append user turn: status %d", second.StatusCode)
	}

	transcript, err := h.store.Transcript(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(transcript) != 2 {
		t.Fatalf("transcript has %d messages, want 2", len(transcript))
	}
	if transcript[0].Role != "assistant" || transcript[1].Role != "user" {
		t.Errorf("transcript roles = [%s, %s], want [assistant, user] in order",
			transcript[0].Role, transcript[1].Role)
	}
}

func TestPostAssistantVoiceTurnRefusesAnotherCandidatesSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	owner, _ := assistantUser(t, pool, iss, "voice-turn-owner@example.test", true)
	_, strangerCookie := assistantUser(t, pool, iss, "voice-turn-stranger@example.test", true)

	jobID := seedApplication(t, pool, owner, "voice-5", "interview")
	sess, _ := h.store.CreateSession(context.Background(), owner, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-turns", strangerCookie,
		map[string]any{"role": "user", "content": "hello"})
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("append turn to another candidate's session: status %d, want 404", resp.StatusCode)
	}
}

func TestPostAssistantVoiceTurnRejectsAnUnknownRole(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "voice-turn-badrole@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-6", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-turns", cookie,
		map[string]any{"role": "system", "content": "hello"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("append turn with role=system: status %d, want 400", resp.StatusCode)
	}
}

func TestPostAssistantVoiceTurnRefusesANonInterviewSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "voice-turn-wrong-preset@example.test", true)

	sess, _ := h.store.CreateSession(context.Background(), userID, "chat", nil, nil)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-turns", cookie,
		map[string]any{"role": "user", "content": "hello"})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("append turn to a chat session: status %d, want 409", resp.StatusCode)
	}
}

// A voice turn is replayed into every later turn of the session, so an unbounded one
// costs every turn after it — the same reasoning PostAssistantMessage already applies
// to a typed message.
func TestPostAssistantVoiceTurnRejectsContentOverTheLengthCeiling(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	h.maxPrompt = 10
	userID, cookie := assistantUser(t, pool, iss, "voice-turn-toolong@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-8", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-turns", cookie,
		map[string]any{"role": "user", "content": "this content is much longer than ten runes"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("append an over-length turn: status %d, want 400", resp.StatusCode)
	}
}

// recordingModel captures the history it was called with, so a test can confirm what
// the model actually saw rather than only what the endpoint answered.
type recordingModel struct {
	reply   *llms.ContentChoice
	history []llms.MessageContent
}

func (m *recordingModel) Chat(_ context.Context, history []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.history = history
	if s.OnText != nil && m.reply.Content != "" {
		s.OnText(m.reply.Content)
	}
	return m.reply, nil
}

// A candidate who started a call and finished it by typing must not have the spoken
// half of the conversation vanish from what the model reasons from next.
func TestASessionContinuesInTextAfterVoiceTurns(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &recordingModel{reply: &llms.ContentChoice{Content: "Good, let's continue."}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "voice-continue@example.test", true)

	jobID := seedApplication(t, pool, userID, "voice-7", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)

	appendResp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/voice-turns", cookie,
		map[string]any{"role": "assistant", "content": "SPOKEN_TURN_MARKER: which round would you like?"})
	if appendResp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("append voice turn: status %d", appendResp.StatusCode)
	}

	msgResp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/messages", cookie,
		map[string]any{"text": "Let's do behavioural."})
	if msgResp.StatusCode != fiber.StatusOK {
		t.Fatalf("post message: status %d", msgResp.StatusCode)
	}
	if _, err := io.ReadAll(msgResp.Body); err != nil {
		t.Fatalf("drain turn: %v", err)
	}

	found := false
	for _, m := range model.history {
		for _, part := range m.Parts {
			if text, ok := part.(llms.TextContent); ok && strings.Contains(text.Text, "SPOKEN_TURN_MARKER") {
				found = true
			}
		}
	}
	if !found {
		t.Error("the model's context did not include the earlier voice-mode turn")
	}
}
