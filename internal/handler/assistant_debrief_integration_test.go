//go:build integration

// Integration tests for the debrief: it is minted from an application the caller holds,
// whatever stage that application sits in, and the agent opens it itself. Run with:
// go test -tags=integration ./internal/handler/
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
)

// A debrief binds exactly as a rehearsal does: to the application's vacancy, to no CV.
func TestCreatingADebriefBindsItToTheApplicationsVacancy(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "debrief@example.test", true)

	const externalID = "debrief-1"
	jobID := seedApplication(t, pool, userID, externalID, "interview")

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=debrief&job=go-developer-"+externalID, cookie, nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create debrief: status %d", resp.StatusCode)
	}

	var got struct {
		Data struct {
			Preset string  `json:"preset"`
			JobID  *int64  `json:"job_id"`
			CVID   *string `json:"cv_id"`
			Label  string  `json:"label"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.Preset != "debrief" {
		t.Errorf("preset = %q, want debrief", got.Data.Preset)
	}
	if got.Data.JobID == nil || *got.Data.JobID != jobID {
		t.Errorf("job_id = %v, want %d — the debrief's context tool is bound to the vacancy", got.Data.JobID, jobID)
	}
	if got.Data.CVID != nil {
		t.Errorf("cv_id = %v, want none — a debrief edits no CV", got.Data.CVID)
	}
	// Named at creation, because a debrief's first message is the server's own brief:
	// identical every time, so naming from it would give every debrief the same string.
	if !strings.Contains(got.Data.Label, "Go Developer") {
		t.Errorf("label = %q, want the vacancy named in it", got.Data.Label)
	}
}

// The application row is the authorisation, exactly as it is for a rehearsal.
func TestCreatingADebriefNeedsAnApplication(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	_, cookie := assistantUser(t, pool, iss, "no-debrief@example.test", true)

	stranger, _ := assistantUser(t, pool, iss, "debrief-stranger@example.test", true)
	const externalID = "debrief-2"
	seedApplication(t, pool, stranger, externalID, "interview")

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=debrief&job=go-developer-"+externalID, cookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("debrief another candidate's application: status %d, want 404", resp.StatusCode)
	}
}

// The stage governs where the button appears, not what the endpoint accepts. Somebody
// who interviewed and never moved their application's stage is precisely who this is
// for, and refusing them would be a bug wearing a rule's clothes.
func TestADebriefIsCreatedWhateverTheStage(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "stale-stage@example.test", true)

	const externalID = "debrief-3"
	seedApplication(t, pool, userID, externalID, "screening")

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=debrief&job=go-developer-"+externalID, cookie, nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("debrief an application still in screening: status %d, want 201", resp.StatusCode)
	}
}

// An interview runs to several rounds, and each is its own conversation.
func TestEveryRoundGetsItsOwnDebrief(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "second-round@example.test", true)

	const externalID = "debrief-4"
	seedApplication(t, pool, userID, externalID, "interview")
	path := "/api/v1/assistant/sessions?preset=debrief&job=go-developer-" + externalID

	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		resp := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("debrief %d: status %d", i+1, resp.StatusCode)
		}
		var got struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids = append(ids, got.Data.ID)
	}
	if ids[0] == ids[1] {
		t.Error("the second round reopened the first round's debrief")
	}
}

// The candidate opened this from an application with nothing to type, so the agent
// speaks first — under the debrief's own brief, which asks what they were asked.
func TestTheDebriefOpensItself(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "What were you asked first?"}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "debrief-opening@example.test", true)

	jobID := seedApplication(t, pool, userID, "debrief-5", "interview")
	sess, err := h.store.CreateSession(context.Background(), userID, "debrief", nil, &jobID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/opening", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("opening: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	for _, want := range []string{"event: user_prompt", "event: result", "asked"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the opening stream lacks %q:\n%s", want, string(body))
		}
	}
	// The rehearsal's brief must not be what a debrief opens under: it would ask which
	// round to rehearse, for an interview already sat.
	if strings.Contains(string(body), "rehearse") {
		t.Error("the debrief opened under the rehearsal's brief")
	}
}

// A reload must not restart the review over whatever the candidate has already recalled.
func TestTheDebriefOpensOnlyOnce(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{
		{Content: "What were you asked first?"},
		{Content: "A second opening nobody asked for."},
	}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "debrief-twice@example.test", true)

	jobID := seedApplication(t, pool, userID, "debrief-6", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "debrief", nil, &jobID)
	path := "/api/v1/assistant/sessions/" + sess.ID.String() + "/opening"

	first := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if _, err := io.ReadAll(first.Body); err != nil {
		t.Fatalf("drain first opening: %v", err)
	}

	second := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if second.StatusCode != fiber.StatusConflict {
		t.Fatalf("second opening: status %d, want 409", second.StatusCode)
	}
}

// A debrief whose opening died upstream must be retryable, for the same reason a
// rehearsal's must: the brief is recorded before the model is called.
func TestAFailedDebriefOpeningCanBeRetried(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, failingModel{})
	userID, cookie := assistantUser(t, pool, iss, "debrief-failed@example.test", true)

	jobID := seedApplication(t, pool, userID, "debrief-7", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "debrief", nil, &jobID)
	path := "/api/v1/assistant/sessions/" + sess.ID.String() + "/opening"

	failed := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if _, err := io.ReadAll(failed.Body); err != nil {
		t.Fatalf("drain the failed opening: %v", err)
	}

	retry := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if retry.StatusCode != fiber.StatusOK {
		t.Fatalf("retry after a failed opening: status %d, want 200 — the debrief can never open", retry.StatusCode)
	}
}

// A debrief is a conversation the candidate returns to, so it belongs in the rail.
func TestADebriefIsListedInTheSessionRail(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "debrief-rail@example.test", true)

	jobID := seedApplication(t, pool, userID, "debrief-8", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "debrief", nil, &jobID)

	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/assistant/sessions", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("list sessions: status %d", resp.StatusCode)
	}
	var got struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range got.Data {
		if s.ID == sess.ID.String() {
			return
		}
	}
	t.Error("the debrief is absent from the session rail, so the candidate cannot return to it")
}
