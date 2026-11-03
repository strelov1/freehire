//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/platform/llm"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestShortlistSessionCreationRetriesWithoutDuplicates(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "shortlist-retry@example.test", false)
	id := uuid.New().String()
	body := map[string]any{"search_query": "skills=go&salary_min=80000", "request_id": id}
	for range 2 {
		resp := assistantRequest(t, app, http.MethodPost, "/api/v1/assistant/sessions", cookie, body)
		if resp.StatusCode != 200 {
			t.Fatalf("create: %d", resp.StatusCode)
		}
		var got struct {
			Data struct {
				ID            string `json:"id"`
				SearchContext struct {
					Query string `json:"query"`
				} `json:"search_context"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got.Data.ID != id || got.Data.SearchContext.Query != "salary_min=80000&skills=go" {
			t.Fatalf("response: %+v", got)
		}
	}
	resp := assistantRequest(t, app, http.MethodPost, "/api/v1/assistant/sessions", cookie, map[string]any{"search_query": "work_mode=remote", "request_id": id})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("rebound id: %d", resp.StatusCode)
	}
}

func TestShortlistPreferencesRemainLocalAndRestoreWithTheSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	user, cookie := assistantUser(t, pool, iss, "shortlist-brief@example.test", false)
	stranger, otherCookie := assistantUser(t, pool, iss, "shortlist-other@example.test", false)
	id := uuid.New()
	resp := assistantRequest(t, app, http.MethodPost, "/api/v1/assistant/sessions", cookie, map[string]any{"search_query": "skills=go", "request_id": id.String()})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	sess, err := h.store.Session(context.Background(), id, user)
	if err != nil {
		t.Fatal(err)
	}
	tool := h.searchPreferencesTool(sess)
	if _, err := tool.Run(context.Background(), stranger, json.RawMessage(`{"brief":"wrong owner"}`)); err == nil {
		t.Fatal("foreign preferences write succeeded")
	}
	if _, err := tool.Run(context.Background(), user, json.RawMessage(`{"brief":"Prefer hands-on roles; office is acceptable only for Acme"}`)); err != nil {
		t.Fatal(err)
	}
	restored, err := h.store.Session(context.Background(), id, user)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SearchContext.Query != "skills=go" || restored.SearchContext.Brief != "Prefer hands-on roles; office is acceptable only for Acme" {
		t.Fatalf("restored: %+v", restored.SearchContext)
	}
	resp = assistantRequest(t, app, http.MethodGet, "/api/v1/assistant/sessions/"+id.String(), otherCookie, nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("foreign session read: %d", resp.StatusCode)
	}
	resp = assistantRequest(t, app, http.MethodPost, "/api/v1/assistant/sessions", otherCookie, map[string]any{"search_query": "skills=go", "request_id": id.String()})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("foreign creation retry: %d", resp.StatusCode)
	}
}

type shortlistQueueModel struct {
	gate    *disconnectModel
	systems chan string
}

func (m *shortlistQueueModel) Chat(ctx context.Context, messages []llms.MessageContent, tools []llms.Tool, stream llm.ChatStream) (*llms.ContentChoice, error) {
	for _, msg := range messages {
		if msg.Role == llms.ChatMessageTypeSystem {
			for _, part := range msg.Parts {
				if text, ok := part.(llms.TextContent); ok {
					m.systems <- text.Text
				}
			}
		}
	}
	return m.gate.Chat(ctx, messages, tools, stream)
}

func TestShortlistQueuedTurnReadsTheUpdatedBrief(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	gate := newDisconnectModel(t)
	defer gate.letGo()
	model := &shortlistQueueModel{gate: gate, systems: make(chan string, 8)}
	app, h := newAssistantApp(pool, iss, model)
	user, cookie := assistantUser(t, pool, iss, "shortlist-queued@example.test", false)
	id := uuid.New()
	resp := assistantRequest(t, app, http.MethodPost, "/api/v1/assistant/sessions", cookie, map[string]any{"search_query": "skills=go", "request_id": id.String()})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	addr := serveOnSocket(t, app)
	running := startTurnInBackground(t, addr, id.String(), cookie)
	select {
	case <-gate.started:
	case <-time.After(10 * time.Second):
		t.Fatal("first turn did not start")
	}
	queued := dialTurn(t, addr, id.String(), cookie)
	defer func() { _ = queued.Close() }()
	awaitEvent(t, queued, "event: queued")
	sess, err := h.store.Session(context.Background(), id, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.searchPreferencesTool(sess).Run(context.Background(), user, json.RawMessage(`{"brief":"Prefer hands-on roles after this correction"}`)); err != nil {
		t.Fatal(err)
	}
	gate.letGo()
	<-running
	awaitEvent(t, queued, "event: result")
	var last string
	for {
		select {
		case last = <-model.systems:
		default:
			if !strings.Contains(last, "Prefer hands-on roles after this correction") {
				t.Fatal("queued turn read the old selection brief")
			}
			return
		}
	}
}
