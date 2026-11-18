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
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/tmc/langchaingo/llms"
)

func TestAssistantTurnStreamsAndPersists(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Here is an answer."}}}
	app, _ := newAssistantApp(pool, iss, model)
	_, cookie := assistantUser(t, pool, iss, "turn@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me go jobs"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("turn: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q, want an SSE stream", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	stream := string(body)
	for _, want := range []string{"event: user_prompt", "event: assistant_text", "event: result"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream is missing %q:\n%s", want, stream)
		}
	}
	if !strings.Contains(stream, assistant.StopEndTurn) {
		t.Errorf("stream does not end with a clean stop reason:\n%s", stream)
	}

	// The transcript is persisted and replays: prompt then answer.
	resp = assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil)
	defer resp.Body.Close()
	var read struct {
		Data struct {
			Session struct {
				Label string `json:"label"`
			} `json:"session"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&read); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	if len(read.Data.Messages) != 2 {
		t.Fatalf("transcript has %d messages, want the prompt and the answer", len(read.Data.Messages))
	}
	if read.Data.Messages[0].Role != assistant.RoleUser || read.Data.Messages[1].Role != assistant.RoleAssistant {
		t.Errorf("transcript roles = %+v, want user then assistant", read.Data.Messages)
	}
	// The rail names a session after its first message.
	if read.Data.Session.Label != "find me go jobs" {
		t.Errorf("label = %q, want the first user message", read.Data.Session.Label)
	}
}

func TestAssistantTurnRejectsAnEmptyMessage(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "empty@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "   "})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("blank message: status %d, want 400", resp.StatusCode)
	}
}

func TestAssistantTurnWithoutAModelIsUnavailable(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil) // no runner: LLM unconfigured
	_, cookie := assistantUser(t, pool, iss, "nollm@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "hi"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("turn with no model: status %d, want 503", resp.StatusCode)
	}
}

// failingTurnModel stands in for an upstream that is down — a 502 from the proxy is an
// ordinary event here, and the turn dies with it.
type failingTurnModel struct{}

func (failingTurnModel) Chat(context.Context, []llms.MessageContent, []llms.Tool, llm.ChatStream) (*llms.ContentChoice, error) {
	return nil, errors.New("upstream llm exploded")
}

// The assistant stream has the fit stream's shape: the handler returns nil before the body
// writer runs, so a turn that dies never reaches RenderError and the access log keeps the
// 200 the stream opened with. Logging it is not enough — nothing watches the journal, and
// it keeps ~12 hours.
func TestAssistantTurnFailureIsReported(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	tr := recordExceptions(t)
	app, _ := newAssistantApp(pool, iss, failingTurnModel{})
	_, cookie := assistantUser(t, pool, iss, "turnfail@example.test", true)

	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+id+"/messages", cookie,
		map[string]string{"text": "find me go jobs"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("turn: status %d, want the 200 the stream opens with", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if got := tr.count(); got != 1 {
		t.Errorf("reported exceptions = %d, want exactly 1 for a failed turn", got)
	}
}
