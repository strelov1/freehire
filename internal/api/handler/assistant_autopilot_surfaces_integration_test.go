//go:build integration

package handler

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// seedTailoringSessionWithSurfaces creates a vacancy whose description prefers the long
// form of infrastructure-as-code, and a tailored CV whose skills chip still says IaC.
func seedTailoringSessionWithSurfaces(t *testing.T, pool *pgxpool.Pool, h *assistantHandlers, userID int64) (string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, description)
		 VALUES ('greenhouse', $1, 'https://example.test/j/'||$1, 'Platform Engineer', $1,
		         'We practice infrastructure as code and Terraform.')
		 RETURNING id`, "autopilot-align-"+uuid.NewString()[:8]).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	doc := `{"summary":"before the run","skills":[{"items":["IaC","Terraform"]}],` +
		`"experience":[{"company":"Acme","role":"Engineer","bullets":["Shipped platform tools"]}]}`
	var cvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'classic-ats', $2::jsonb, $3)
		 RETURNING id`, userID, doc, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	sess, err := h.store.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	return sess.ID.String(), cvID
}

func TestAutopilotAlignsSkillSurfacesBeforeTurn(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot-align@example.test", true)

	sessionID, cvID := seedTailoringSessionWithSurfaces(t, pool, h, userID)
	seedFitAnalysis(t, pool, userID, cvID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read the run's stream: %v", err)
	}

	rec, err := h.cv.cvStore.Get(context.Background(), cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills = %+v, want infrastructure as code before the turn", rec.Document.Skills)
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	feedResp1 := assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil)
	defer feedResp1.Body.Close()
	decodeJSON(t, feedResp1, &feed)
	var alignRevs int
	for _, rev := range feed.Data {
		if rev.Actor == string(cvedit.ActorSystem) && rev.Origin == string(cvedit.OriginImport) && rev.BatchID == "" {
			alignRevs++
		}
	}
	if alignRevs != 1 {
		t.Fatalf("system align revisions = %d, want 1; feed=%+v", alignRevs, feed.Data)
	}

	// Already aligned: a second start is a no-op on the document and adds no revision.
	resp2 := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("second autopilot: status %d", resp2.StatusCode)
	}
	if _, err := io.ReadAll(resp2.Body); err != nil {
		t.Fatalf("read the run's stream: %v", err)
	}
	feedResp2 := assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil)
	defer feedResp2.Body.Close()
	decodeJSON(t, feedResp2, &feed)
	alignRevs = 0
	for _, rev := range feed.Data {
		if rev.Actor == string(cvedit.ActorSystem) && rev.Origin == string(cvedit.OriginImport) && rev.BatchID == "" {
			alignRevs++
		}
	}
	if alignRevs != 1 {
		t.Fatalf("after second start system align revisions = %d, want still 1", alignRevs)
	}
}

func TestAutopilotUndoLeavesSurfaceAlign(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	ctx := context.Background()

	model := &scriptedTurnModel{}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot-undo-align@example.test", true)
	sessionID, cvID := seedTailoringSessionWithSurfaces(t, pool, h, userID)
	seedFitAnalysis(t, pool, userID, cvID)

	model.replies = []*llms.ContentChoice{
		callReplyChoice("cv_edit", `{"ops":[{"kind":"remove","path":"experience[0].bullets[0]"}]}`),
		{Content: "Removed a bullet."},
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read the run's stream: %v", err)
	}

	rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills after run = %+v, want aligned form", rec.Document.Skills)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 0 {
		t.Fatalf("experience = %+v, want the run's remove to have cleared the bullet", rec.Document.Experience)
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	feedResp3 := assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil)
	defer feedResp3.Body.Close()
	decodeJSON(t, feedResp3, &feed)
	var batchID string
	for _, rev := range feed.Data {
		if rev.Origin == string(cvedit.OriginTailorAgent) && rev.BatchID != "" {
			batchID = rev.BatchID
			break
		}
	}
	if batchID == "" {
		t.Fatalf("no agent batch in feed: %+v", feed.Data)
	}

	undo := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/me/cvs/"+cvID.String()+"/revisions/batch/"+batchID+"/undo", cookie, nil)
	if undo.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(undo.Body)
		t.Fatalf("undo batch = %d: %s", undo.StatusCode, body)
	}
	undo.Body.Close()

	rec, err = h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv after undo: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills after undo = %+v, want JD wording left by the align revision", rec.Document.Skills)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 1 {
		t.Fatalf("experience after undo = %+v, want the run's remove reverted", rec.Document.Experience)
	}
}
