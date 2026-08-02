//go:build integration

// Integration tests for the follow-up suggestions endpoint against a real Postgres:
// what it answers for a conversation with nothing to follow up on, what it answers
// when the model fails, and that a suggestion never reports a problem the caller
// cannot act on.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llm"
)

// jsonModel answers every generation with one fixed body, or fails.
type jsonModel struct {
	body string
	err  error
}

func (m jsonModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.body}}}, nil
}

func (m jsonModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return m.body, m.err
}

// followUpsOf issues the request and returns the suggestions plus the status.
func followUpsOf(t *testing.T, app *fiber.App, id, cookie string) ([]string, int) {
	t.Helper()
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/followups", cookie, nil)
	defer resp.Body.Close()
	var out struct {
		Data struct {
			FollowUps []string `json:"followups"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Data.FollowUps, resp.StatusCode
}

// A conversation nobody has spoken in yet has nothing to follow up on. That is an
// empty list, not an error — and, importantly, not a model call either.
func TestFollowUpsOnAnEmptyConversationSpendNothing(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	setFollowUpModel(h, llm.NewWithModel(jsonModel{body: `{"follow_ups":["never asked?"]}`}))
	_, cookie := assistantUser(t, pool, iss, "quiet@example.test", true)

	got, status := followUpsOf(t, app, createSession(t, app, cookie), cookie)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(got) != 0 {
		t.Errorf("suggestions = %q, want none for a conversation with no answer in it", got)
	}
}

// The strip is decoration. A model that fails must not turn into an error the caller
// is shown, because there is nothing they could do about it and the answer they came
// for is already on screen.
func TestFollowUpsSurviveAFailingModel(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "here are three roles"}}})
	setFollowUpModel(h, llm.NewWithModel(jsonModel{err: errors.New("gateway down")}))
	_, cookie := assistantUser(t, pool, iss, "unlucky@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me jobs"})
	resp.Body.Close()

	got, status := followUpsOf(t, app, id, cookie)
	if status != fiber.StatusOK {
		t.Errorf("status = %d, want 200 — a failed suggestion is not a failed request", status)
	}
	if len(got) != 0 {
		t.Errorf("suggestions = %q, want none", got)
	}
}

// The happy path, end to end: a real answer in a real transcript, suggestions capped
// and trimmed on the way out.
func TestFollowUpsFollowTheAnswerThatWasGiven(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "here are three roles"}}})
	setFollowUpModel(h, llm.NewWithModel(jsonModel{
		body: `{"follow_ups":["compare the first two?","tailor my CV to the first?","  ","which pays most?","a fifth?"]}`,
	}))
	_, cookie := assistantUser(t, pool, iss, "curious@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me jobs"})
	resp.Body.Close()

	got, status := followUpsOf(t, app, id, cookie)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(got) != 3 {
		t.Fatalf("suggestions = %q, want three", got)
	}
	if got[0] != "compare the first two?" || got[2] != "which pays most?" {
		t.Errorf("suggestions = %q, want the blank dropped and the list capped at three", got)
	}
}

// An unconfigured model is the deployment with no suggestions at all: an empty list,
// and no branch needed at the call site.
func TestFollowUpsWithoutAModelAreEmpty(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "an answer"}}})
	setFollowUpModel(h, nil)
	_, cookie := assistantUser(t, pool, iss, "plain@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "hi"})
	resp.Body.Close()

	got, status := followUpsOf(t, app, id, cookie)
	if status != fiber.StatusOK || len(got) != 0 {
		t.Errorf("status = %d, suggestions = %q, want 200 and none", status, got)
	}
}

// Authentication is not optional, and a refusal must not reach the model.
func TestFollowUpsRequireACredential(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	setFollowUpModel(h, llm.NewWithModel(jsonModel{body: `{"follow_ups":["never?"]}`}))
	_, cookie := assistantUser(t, pool, iss, "owner@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/followups", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// LastExchange is the seam between the transcript and the suggester, so its rule --
// follow the last answer that actually said something -- is worth pinning against a
// transcript that a real turn wrote rather than one a test hand-built.
func TestFollowUpsReadTheStoredTranscript(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "the spoken answer"}}})
	userID, cookie := assistantUser(t, pool, iss, "reader@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "the question"})
	resp.Body.Close()

	sessionID, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	sess, err := h.store.Session(context.Background(), sessionID, userID)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	messages, err := h.store.Transcript(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	ex, ok := assistant.LastExchange(messages)
	if !ok {
		t.Fatal("LastExchange found nothing in a transcript a turn just wrote")
	}
	if ex.Prompt != "the question" || ex.Answer != "the spoken answer" {
		t.Errorf("exchange = %+v, want the turn that was just run", ex)
	}
}

// setFollowUpModel binds the suggestion model the way newAssistantHandlers does. It exists
// as ONE helper so a fixture cannot bind half of it — the client without the spend
// resolver — which is the shape of divergence a partial struct literal invites.
func setFollowUpModel(h *assistantHandlers, model *llm.Client) {
	h.followUps = assistant.NewFollowUps(model)
	h.followUpLLM = llmBinding{client: model, keys: h.keys}
}
