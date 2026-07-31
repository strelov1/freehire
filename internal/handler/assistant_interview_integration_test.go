//go:build integration

// Integration tests for starting a rehearsal: the session is minted from an application
// the caller actually holds, and the agent opens the conversation itself. Run with:
// go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llm"
)

// seedApplication records an application the caller holds, at the given stage, and
// returns the vacancy id.
func seedApplication(t *testing.T, pool *pgxpool.Pool, userID int64, externalID, stage string) int64 {
	t.Helper()
	ctx := context.Background()
	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', $1, 'https://example.test/j/' || $1, 'Go Developer', 'Acme', 'go-developer-' || $1)
		 RETURNING id`, externalID).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`WITH mark AS (
		     INSERT INTO user_jobs (user_id, job_id) VALUES ($1, $2)
		     ON CONFLICT (user_id, job_id) DO NOTHING
		 )
		 INSERT INTO applications (user_id, job_id, company_slug, role_title, applied_at, stage)
		 SELECT $1, $2, j.company_slug, j.title, now(), $3 FROM jobs j WHERE j.id = $2`,
		userID, jobID, stage); err != nil {
		t.Fatalf("seed application: %v", err)
	}
	return jobID
}

// A rehearsal is minted from an application: bound to its vacancy, bound to no CV.
func TestCreatingARehearsalBindsItToTheApplicationsVacancy(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "rehearse@example.test", true)

	const externalID = "rehearsal-1"
	jobID := seedApplication(t, pool, userID, externalID, "interview")

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=interview&job=go-developer-"+externalID, cookie, nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create rehearsal: status %d", resp.StatusCode)
	}

	var got struct {
		Data struct {
			Preset string  `json:"preset"`
			JobID  *int64  `json:"job_id"`
			CVID   *string `json:"cv_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.Preset != "interview" {
		t.Errorf("preset = %q, want interview", got.Data.Preset)
	}
	if got.Data.JobID == nil || *got.Data.JobID != jobID {
		t.Errorf("job_id = %v, want %d", got.Data.JobID, jobID)
	}
	if got.Data.CVID != nil {
		t.Errorf("cv_id = %v, want none — a rehearsal edits no CV", got.Data.CVID)
	}
}

// Rehearsing an interview for a vacancy you never applied to is not a rehearsal, and
// the application row is also what proves the vacancy is the caller's to rehearse.
func TestCreatingARehearsalNeedsAnApplication(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, nil)
	userID, cookie := assistantUser(t, pool, iss, "no-application@example.test", true)

	// An application belonging to somebody else entirely.
	stranger, _ := assistantUser(t, pool, iss, "stranger@example.test", true)
	const externalID = "rehearsal-2"
	seedApplication(t, pool, stranger, externalID, "interview")
	_ = userID

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions?preset=interview&job=go-developer-"+externalID, cookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("create rehearsal for another candidate's application: status %d, want 404", resp.StatusCode)
	}
}

// The candidate opened this from an application, not from an empty chat box — so the
// agent speaks first, under a brief the server chose.
func TestTheRehearsalOpensItself(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Which round would you like to rehearse?"}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "opening@example.test", true)

	jobID := seedApplication(t, pool, userID, "rehearsal-3", "interview")
	sess, err := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)
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
	for _, want := range []string{"event: user_prompt", "event: result", "rehearse"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the opening stream lacks %q:\n%s", want, string(body))
		}
	}
}

// Reloading the page must not restart the interview. The opening is the first turn of a
// conversation, and a conversation that already has one is simply underway.
func TestTheRehearsalOpensOnlyOnce(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{
		{Content: "Which round would you like to rehearse?"},
		{Content: "A second opening nobody asked for."},
	}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "opened-twice@example.test", true)

	jobID := seedApplication(t, pool, userID, "rehearsal-4", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)
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

// failingModel stands in for the LLM proxy being down — a 502 from it is an ordinary
// event, not a corner case.
type failingModel struct{}

func (failingModel) Chat(context.Context, []llms.MessageContent, []llms.Tool, llm.ChatStream) (*llms.ContentChoice, error) {
	return nil, errors.New("upstream 502")
}

// An opening that died upstream must be retryable. The runner records the brief BEFORE
// it calls the model, so a failed turn leaves one message behind — and a guard that
// refuses on any transcript would strand the rehearsal in a state it can never leave:
// one line the candidate did not write, no opening, and no way back.
func TestAFailedOpeningCanBeRetried(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, failingModel{})
	userID, cookie := assistantUser(t, pool, iss, "failed-opening@example.test", true)

	jobID := seedApplication(t, pool, userID, "rehearsal-5", "interview")
	sess, _ := h.store.CreateSession(context.Background(), userID, "interview", nil, &jobID)
	path := "/api/v1/assistant/sessions/" + sess.ID.String() + "/opening"

	failed := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if _, err := io.ReadAll(failed.Body); err != nil {
		t.Fatalf("drain the failed opening: %v", err)
	}

	retry := assistantRequest(t, app, fiber.MethodPost, path, cookie, nil)
	if retry.StatusCode != fiber.StatusOK {
		t.Fatalf("retry after a failed opening: status %d, want 200 — the rehearsal can never open", retry.StatusCode)
	}
}

// The opening belongs to a rehearsal. Any other session already has a way to start:
// the candidate types.
func TestOnlyARehearsalHasAnOpening(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Hello."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "plain-chat@example.test", true)

	sess, _ := h.store.CreateSession(context.Background(), userID, "chat", nil, nil)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sess.ID.String()+"/opening", cookie, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("opening on a chat session: status %d, want 409", resp.StatusCode)
	}
}
