//go:build integration

// Integration tests for the autopilot run endpoint (see the tailor-autopilot change): it
// runs only on a tailoring session bound to a CV, it takes the pre-run snapshot itself, and
// the brief and the turn's ceiling are the server's rather than the caller's.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
)

// seedTailoringSession creates a CV bound to a vacancy plus the tailoring session that
// addresses it, and returns the session id and the CV id.
func seedTailoringSession(t *testing.T, pool *pgxpool.Pool, h *assistantHandlers, userID int64) (string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'autopilot-1', 'https://example.test/j/1', 'Go Developer', 'go-developer-autopilot')
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	var cvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'classic-ats', '{"summary":"before the run"}'::jsonb, $2)
		 RETURNING id`, userID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	sess, err := h.store.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	return sess.ID.String(), cvID
}

func TestAutopilotRunsOnATailoringSessionAndSnapshotsFirst(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot@example.test", true)

	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	stream := string(body)
	for _, want := range []string{"event: user_prompt", "event: result"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream is missing %q:\n%s", want, stream)
		}
	}

	// The snapshot is the server's job, not the client's: it must exist after a run even
	// though nobody asked for it.
	var revertable bool
	if err := pool.QueryRow(context.Background(),
		`SELECT autopilot_undo IS NOT NULL FROM cvs WHERE id = $1`, cvID).Scan(&revertable); err != nil {
		t.Fatalf("read snapshot flag: %v", err)
	}
	if !revertable {
		t.Error("no pre-run snapshot was taken; the run would be unrevertable")
	}

	// The brief is the server's too: the recorded prompt is one we wrote, and the caller
	// sent no text at all.
	msgs, err := h.store.Transcript(context.Background(), mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != "user" || len(msgs[0].Content) == 0 {
		t.Fatalf("transcript = %+v, want it to open with the server's brief", msgs)
	}
}

func TestAutopilotIsRefusedOnANonTailoringSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "autopilot-chat@example.test", true)

	// A plain chat session: no CV, no vacancy, no tailoring tools.
	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+id+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("autopilot on a chat session: status %d, want 409", resp.StatusCode)
	}
}

func TestAutopilotOnAForeignSessionIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	owner, _ := assistantUser(t, pool, iss, "autopilot-owner@example.test", true)
	_, strangerCookie := assistantUser(t, pool, iss, "autopilot-stranger@example.test", true)

	sessionID, _ := seedTailoringSession(t, pool, h, owner)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", strangerCookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("foreign session: status %d, want 404 — ownership never leaks as 403", resp.StatusCode)
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

// The revert is the other half of the run: it restores the document the run started from
// and clears the log that described the run, since that log now names edits that are gone.
func TestAutopilotUndoRestoresAndClears(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	userID, cookie := assistantUser(t, pool, iss, "autopilot-undo@example.test", true)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)
	_ = sessionID

	ctx := context.Background()
	// A run happened: the snapshot was taken, the document changed, a report was written.
	if err := h.cv.cvStore.SnapshotForAutopilot(ctx, cvID, userID); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE cvs SET data = '{"summary":"after the run"}'::jsonb WHERE id = $1`, cvID); err != nil {
		t.Fatalf("simulate the run's edit: %v", err)
	}
	if err := h.cv.cvStore.SetAutopilotReport(ctx, cvID, userID, []cv.AutopilotEntry{
		{Requirement: "Kafka", Status: cv.AutopilotClosedBank, Note: "reframed"},
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/me/cvs/"+cvID.String()+"/autopilot/undo", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("undo: status %d", resp.StatusCode)
	}

	rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Document.Summary != "before the run" {
		t.Errorf("summary = %q, want the pre-run document", rec.Document.Summary)
	}
	if rec.AutopilotRevertable || len(rec.AutopilotReport) != 0 {
		t.Errorf("after undo: revertable=%v report=%d, want both cleared", rec.AutopilotRevertable, len(rec.AutopilotReport))
	}

	// A second undo has nothing to restore, and says so rather than blanking the document.
	again := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/me/cvs/"+cvID.String()+"/autopilot/undo", cookie, nil)
	if again.StatusCode != fiber.StatusConflict {
		t.Errorf("undo without a run: status %d, want 409", again.StatusCode)
	}
}

// The CV read carries the run's log and whether it can still be undone, so the workspace
// panel renders from the CV it already re-reads after every turn.
func TestCVReadCarriesTheRunReport(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	userID, cookie := assistantUser(t, pool, iss, "autopilot-read@example.test", true)
	_, cvID := seedTailoringSession(t, pool, h, userID)

	ctx := context.Background()
	if err := h.cv.cvStore.SnapshotForAutopilot(ctx, cvID, userID); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := h.cv.cvStore.SetAutopilotReport(ctx, cvID, userID, []cv.AutopilotEntry{
		{Requirement: "Kafka in production", Status: cv.AutopilotClosedBank, Note: "reframed"},
		{Requirement: "Team leadership", Status: cv.AutopilotOpen},
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String(), cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get cv: status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`"autopilot_report"`, "Kafka in production", `"closed_bank"`, `"autopilot_revertable":true`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("CV read is missing %q:\n%s", want, body)
		}
	}
}
