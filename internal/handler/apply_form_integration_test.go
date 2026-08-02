//go:build integration

// End-to-end HTTP verification of the apply-form read: GET /api/v1/jobs/:slug/apply-form
// drives the real path — GetJobIDBySlug → GetApplyFormByJobID → applyform.ForDisplay →
// JSON — so what a client receives is what these tests assert.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/db"
)

func seedApplyFormJob(t *testing.T, q *db.Queries, extID, slug string) int64 {
	t.Helper()
	saved, err := q.UpsertJob(context.Background(), db.UpsertJobParams{
		Source: "greenhouse", ExternalID: extID, URL: "https://ex.test/" + extID,
		Title: "Staff Engineer", Company: "Acme", CompanySlug: "acme",
		PublicSlug: slug, Location: "Remote", Description: "We need a staff engineer.",
	})
	if err != nil {
		t.Fatalf("seed %s: %v", extID, err)
	}
	return saved.Job.ID
}

func seedApplyForm(t *testing.T, q *db.Queries, jobID int64, form applyform.Form) {
	t.Helper()
	payload, err := json.Marshal(form)
	if err != nil {
		t.Fatalf("encode form: %v", err)
	}
	if err := q.UpsertApplyForm(context.Background(), db.UpsertApplyFormParams{
		JobID: jobID, Provider: form.Provider, Payload: payload,
	}); err != nil {
		t.Fatalf("seed form: %v", err)
	}
}

func applyFormApp(q *db.Queries) *fiber.App {
	h := &jobsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug/apply-form", h.JobApplyForm)
	return app
}

func getApplyForm(t *testing.T, app *fiber.App, slug string) (int, []byte) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/jobs/"+slug+"/apply-form", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func truncateApplyFormJobs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func TestJobApplyForm_ServesTheStoredQuestions(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	truncateApplyFormJobs(t, pool)

	jobID := seedApplyFormJob(t, q, "acme:1", "staff-engineer-acme-1")
	seedApplyForm(t, q, jobID, applyform.Form{
		Provider: "greenhouse",
		Fields: []applyform.Field{
			{ID: "first_name", Label: "First Name", Type: applyform.TypeText, Required: true},
			{ID: "resume", Label: "Resume/CV", Type: applyform.TypeFile, Required: true},
			{ID: "question_1", Label: "Why did you apply?", Type: applyform.TypeTextarea, Required: true},
			{ID: "race", Label: "Race", Type: applyform.TypeSelect, Demographic: true},
		},
	})

	status, body := getApplyForm(t, applyFormApp(q), "staff-engineer-acme-1")
	if status != fiber.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	var out struct {
		Data applyform.Display `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if out.Data.Provider != "greenhouse" {
		t.Errorf("provider = %q, want %q", out.Data.Provider, "greenhouse")
	}
	if len(out.Data.Questions) != 1 || out.Data.Questions[0].Text != "Why did you apply?" {
		t.Errorf("questions = %+v, want only the employer's own", out.Data.Questions)
	}
	if out.Data.Questions[0].Answer != "written answer" {
		t.Errorf("answer = %q, want the written-answer hint", out.Data.Questions[0].Answer)
	}
	if len(out.Data.Basics) != 2 {
		t.Errorf("basics = %v, want the two standard fields collapsed", out.Data.Basics)
	}
}

// "This employer asks nothing" and "we could not read this platform" are different
// statements, and only one of them is true — so a posting with no stored form must not
// answer with an empty form.
func TestJobApplyForm_PostingWithoutAStoredForm(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	truncateApplyFormJobs(t, pool)

	seedApplyFormJob(t, q, "acme:2", "staff-engineer-acme-2")

	status, body := getApplyForm(t, applyFormApp(q), "staff-engineer-acme-2")
	if status != fiber.StatusNotFound {
		t.Fatalf("status %d: %s, want 404", status, body)
	}
}

func TestJobApplyForm_UnknownSlug(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	truncateApplyFormJobs(t, pool)

	status, body := getApplyForm(t, applyFormApp(q), "no-such-job")
	if status != fiber.StatusNotFound {
		t.Fatalf("status %d: %s, want 404", status, body)
	}
}

// A form asking nothing beyond a CV is the one-click apply a candidate is hunting for,
// so it is served rather than treated as an absent form.
func TestJobApplyForm_FormWithNoEmployerQuestions(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	truncateApplyFormJobs(t, pool)

	jobID := seedApplyFormJob(t, q, "acme:3", "staff-engineer-acme-3")
	seedApplyForm(t, q, jobID, applyform.Form{
		Provider: "recruitee",
		Fields: []applyform.Field{
			{ID: "name", Label: "Full name", Type: applyform.TypeText, Required: true},
			{ID: "cv", Label: "CV", Type: applyform.TypeFile, Required: true},
		},
	})

	status, body := getApplyForm(t, applyFormApp(q), "staff-engineer-acme-3")
	if status != fiber.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	var out struct {
		Data applyform.Display `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(out.Data.Questions) != 0 {
		t.Errorf("questions = %+v, want none", out.Data.Questions)
	}
	if len(out.Data.Basics) != 2 {
		t.Errorf("basics = %v, want both standard fields", out.Data.Basics)
	}
}
