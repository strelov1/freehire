//go:build integration

// Integration tests for the CV-builder HTTP surface (add-cv-builder): CRUD round-trip,
// owner isolation (foreign id → 404), open access to every signed-in user (no beta gate),
// the 501 gate when no renderer is configured, and seeding a new CV from the stored résumé
// structure. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// seedAccount inserts a user and optionally flags it as a beta tester.
func seedAccount(t *testing.T, h *API, email string, beta bool) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (email, beta_tester) VALUES ($1, $2) RETURNING id`, email, beta).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// buildCVApp wires just the CV routes onto a fresh fiber app. The routes are open to every
// signed-in user (cookie auth only) — the beta gate was lifted when CV tailoring went public.
func buildCVApp(h *API, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss)
	app.Get("/api/v1/cv-templates", saved, h.ListCVTemplates)
	app.Get("/api/v1/me/cvs", saved, h.ListCVs)
	app.Post("/api/v1/me/cvs", saved, h.CreateCV)
	app.Get("/api/v1/me/cvs/:id", saved, h.GetCV)
	app.Put("/api/v1/me/cvs/:id", saved, h.UpdateCV)
	app.Put("/api/v1/me/cvs/:id/template", saved, h.SetCVTemplate)
	app.Delete("/api/v1/me/cvs/:id", saved, h.DeleteCV)
	app.Get("/api/v1/me/cvs/:id/pdf", saved, h.RenderCVPDF)
	return app
}

func doCV(t *testing.T, app *fiber.App, method, path, token string, body any) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// TestCVTemplatesEndpoint_OpenToAuthed checks the static templates list is open to every
// signed-in user: an unauthenticated request is 401, while a plain (non-beta) user gets every
// registered template.
func TestCVTemplatesEndpoint_OpenToAuthed(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	plainTok, _ := iss.Issue(seedAccount(t, h, "plain@example.test", false))

	if resp := doCV(t, app, fiber.MethodGet, "/api/v1/cv-templates", "", nil); resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("unauthenticated templates = %d, want 401", resp.StatusCode)
	}

	resp := doCV(t, app, fiber.MethodGet, "/api/v1/cv-templates", plainTok, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("non-beta templates = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data []cv.TemplateInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != len(cv.Templates()) {
		t.Fatalf("returned %d templates, want %d", len(body.Data), len(cv.Templates()))
	}
}

// TestSetCVTemplateEndpoint checks the set-template endpoint: a valid registered id updates
// the template while leaving title/document intact; an unknown id is a 400; a foreign id 404.
func TestSetCVTemplateEndpoint(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	owner := seedAccount(t, h, "owner@example.test", true)
	ownerTok, _ := iss.Issue(owner)
	otherTok, _ := iss.Issue(seedAccount(t, h, "other2@example.test", true))

	// Create a CV to switch the template on.
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", ownerTok, createCVRequest{Title: "General"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	path := "/api/v1/me/cvs/" + strconv.FormatInt(created.Data.ID, 10) + "/template"

	// Valid registered template → 204, and it sticks on read.
	if resp := doCV(t, app, fiber.MethodPut, path, ownerTok, map[string]string{"template_id": "modern-sans"}); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("set valid template = %d, want 204", resp.StatusCode)
	}
	resp = doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+strconv.FormatInt(created.Data.ID, 10), ownerTok, nil)
	var got struct {
		Data struct {
			TemplateID string `json:"template_id"`
			Title      string `json:"title"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Data.TemplateID != "modern-sans" {
		t.Errorf("template = %q, want modern-sans", got.Data.TemplateID)
	}
	if got.Data.Title != "General" {
		t.Errorf("title changed to %q, want General", got.Data.Title)
	}

	// Unknown template → 400.
	if resp := doCV(t, app, fiber.MethodPut, path, ownerTok, map[string]string{"template_id": "nope"}); resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("unknown template = %d, want 400", resp.StatusCode)
	}

	// Foreign owner → 404.
	if resp := doCV(t, app, fiber.MethodPut, path, otherTok, map[string]string{"template_id": "centered"}); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("foreign set = %d, want 404", resp.StatusCode)
	}
}

func TestCVEndpoints_CRUDAndIsolation(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))} // storage disabled → seed no-ops
	app := buildCVApp(h, iss)

	beta := seedAccount(t, h, "beta@example.test", true)
	other := seedAccount(t, h, "other@example.test", true)
	plain := seedAccount(t, h, "plain@example.test", false)
	betaTok, _ := iss.Issue(beta)
	otherTok, _ := iss.Issue(other)
	plainTok, _ := iss.Issue(plain)

	// A plain (non-beta) user now has full access — the CV builder is public.
	if resp := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs", plainTok, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("non-beta list = %d, want 200", resp.StatusCode)
	}

	// Create (no seed → empty document).
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", betaTok, createCVRequest{Title: "General"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Data cvResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	id := created.Data.ID
	if created.Data.Title != "General" || id == 0 {
		t.Fatalf("create returned %+v", created.Data)
	}

	// Update with a real document.
	doc := cv.Document{Header: cv.Header{FullName: "Ada Lovelace"}, Skills: []cv.SkillGroup{{Group: "Lang", Items: []string{"Go"}}}}
	upPath := "/api/v1/me/cvs/" + strconv.FormatInt(id, 10)
	if resp := doCV(t, app, fiber.MethodPut, upPath, betaTok, updateCVRequest{Title: "Tailored", Document: doc}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("update = %d, want 200", resp.StatusCode)
	}

	// Get reflects the update.
	resp = doCV(t, app, fiber.MethodGet, upPath, betaTok, nil)
	var got struct {
		Data cvResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Data.Title != "Tailored" || got.Data.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("get after update = %+v", got.Data)
	}

	// Owner isolation: another beta user cannot read it.
	if resp := doCV(t, app, fiber.MethodGet, upPath, otherTok, nil); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("foreign get = %d, want 404", resp.StatusCode)
	}

	// PDF gate: renderer is nil → 501.
	if resp := doCV(t, app, fiber.MethodGet, upPath+"/pdf", betaTok, nil); resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("pdf without renderer = %d, want 501", resp.StatusCode)
	}

	// With a renderer configured, the PDF streams (when typst is installed).
	if bin, err := exec.LookPath("typst"); err == nil {
		h.cvRenderer = cv.NewTypstRenderer(bin)
		resp := doCV(t, app, fiber.MethodGet, upPath+"/pdf", betaTok, nil)
		if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" {
			t.Fatalf("pdf render = %d ct=%q, want 200 application/pdf", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		resp.Body.Close()
		h.cvRenderer = nil
	}

	// Delete then 404.
	if resp := doCV(t, app, fiber.MethodDelete, upPath, betaTok, nil); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete = %d, want 204", resp.StatusCode)
	}
	if resp := doCV(t, app, fiber.MethodGet, upPath, betaTok, nil); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", resp.StatusCode)
	}
}

func TestCVCreate_SeedsFromStructuredResume(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	ctx := context.Background()
	iss := auth.NewIssuer("test-secret", time.Hour)
	// S3 storage is DISABLED (nil blobs): seeding reads the structured résumé from
	// Postgres, so it must work independently of object storage.
	store := resume.New(nil, resume.NewQueriesRepository(queries))
	h := &API{pool: pool, queries: queries, issuer: iss,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)), resume: store}
	app := buildCVApp(h, iss)

	user := seedAccount(t, h, "seed@example.test", true)
	tok, _ := iss.Issue(user)

	// Seed a structured résumé directly. Both stamps take the same statement-time now(),
	// so the structure reads as current (the store's freshness gate requires them equal).
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Seeded Ada", Headline: "Backend Engineer"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_structured = $2, resume_structured_uploaded_at = now(), resume_uploaded_at = now() WHERE id = $1`,
		user, blob); err != nil {
		t.Fatalf("seed structured résumé: %v", err)
	}

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "Seeded", Seed: true})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create seeded = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data cvResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	// Name comes from the structure; the summary/tagline falls back to the headline line.
	if body.Data.Document.Header.FullName != "Seeded Ada" || body.Data.Document.Summary != "Backend Engineer" {
		t.Fatalf("seeded document = %+v / summary=%q, want name+summary from structure", body.Data.Document.Header, body.Data.Document.Summary)
	}
}
