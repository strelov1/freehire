//go:build integration

package handler

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// The session routes answer through the generated server: a create is a 201 with the
// session, the read carries the (empty) transcript as a list, and a delete is a 204 after
// which the read is a 404.
func TestAssistantSessionLifecycleThroughTheContract(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "contract-session@example.test", true)

	id := createSession(t, app, cookie)

	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get: status %d (%s)", resp.StatusCode, raw)
	}
	var got apigen.GetAssistantSessionOK
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if got.Data.Session.ID != id || got.Data.Session.Preset != "chat" {
		t.Errorf("session = %+v, want chat %s", got.Data.Session, id)
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(got.Data.Messages, &messages); err != nil {
		t.Errorf("messages %s are not a list: %v", got.Data.Messages, err)
	}

	del := assistantRequest(t, app, fiber.MethodDelete, "/api/v1/assistant/sessions/"+id, cookie, nil)
	del.Body.Close()
	if del.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", del.StatusCode)
	}
	gone := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions/"+id, cookie, nil)
	gone.Body.Close()
	if gone.StatusCode != fiber.StatusNotFound {
		t.Errorf("get after delete: status %d, want 404", gone.StatusCode)
	}
}

// A session id that is not a UUID names nothing: it is the same 404 a session the caller
// does not own gets, on every route that takes one.
func TestAssistantMalformedSessionIDIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "contract-badid@example.test", true)

	for _, c := range []struct{ method, path string }{
		{fiber.MethodGet, "/api/v1/assistant/sessions/not-a-uuid"},
		{fiber.MethodDelete, "/api/v1/assistant/sessions/not-a-uuid"},
		{fiber.MethodPost, "/api/v1/assistant/sessions/not-a-uuid/cancel"},
		{fiber.MethodPost, "/api/v1/assistant/sessions/not-a-uuid/extend"},
	} {
		resp := assistantRequest(t, app, c.method, c.path, cookie, nil)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound || strings.TrimSpace(string(raw)) != `{"error":"session not found"}` {
			t.Errorf("%s %s: status %d body %s, want 404 session not found", c.method, c.path, resp.StatusCode, raw)
		}
	}
}

// A paid-AI route turns a free caller away with the 402 body before it does any work.
func TestAssistantVoiceTokenRefusesAFreeCaller(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := realGateAssistantApp(t, pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "contract-free@example.test", false)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/voice-token", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Errorf("status %d, want 402", resp.StatusCode)
	}
}
