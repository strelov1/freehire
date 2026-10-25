//go:build integration

package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
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
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("apply session on another candidate's application: status %d, want 404", resp.StatusCode)
	}
}
