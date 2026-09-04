//go:build integration

// Integration tests for the CV-builder HTTP surface (add-cv-builder): CRUD round-trip,
// owner isolation (foreign id → 404), open access to every signed-in user (no beta gate),
// the 501 gate when no renderer is configured, and seeding a new CV from the stored résumé
// structure. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// seedAccount inserts a user and optionally flags it as a beta tester.
func seedAccount(t *testing.T, pool *pgxpool.Pool, email string, beta bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		// Verified, like every real account these surfaces see: an unproven address
		// cannot mint the API key some of these fixtures need.
		`INSERT INTO users (email, beta_tester, email_verified) VALUES ($1, $2, true) RETURNING id`, email, beta).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// buildCVApp wires just the CV routes onto a fresh fiber app. The routes are open to every
// signed-in user (cookie auth only) — the beta gate was lifted when CV tailoring went public.
func buildCVApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/cv-templates", saved, h.ListCVTemplates)
	app.Get("/api/v1/me/cvs", saved, h.ListCVs)
	app.Post("/api/v1/me/cvs", saved, h.CreateCV)
	app.Get("/api/v1/me/cvs/:id", saved, h.GetCV)
	app.Put("/api/v1/me/cvs/:id", saved, h.UpdateCV)
	app.Put("/api/v1/me/cvs/:id/template", saved, h.SetCVTemplate)
	app.Delete("/api/v1/me/cvs/:id", saved, h.DeleteCV)
	app.Get("/api/v1/me/cvs/:id/pdf", saved, h.RenderCVPDF)
	app.Get("/api/v1/me/cvs/:id/revisions", saved, h.ListCVRevisions)
	app.Post("/api/v1/me/cvs/:id/revisions/:rid/undo", saved, h.UndoCVRevision)
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
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	plainTok, _ := iss.Issue(seedAccount(t, pool, "plain@example.test", false), testTokenVersion)

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
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	owner := seedAccount(t, pool, "owner@example.test", true)
	ownerTok, _ := iss.Issue(owner, testTokenVersion)
	otherTok, _ := iss.Issue(seedAccount(t, pool, "other2@example.test", true), testTokenVersion)

	// Create a CV to switch the template on.
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", ownerTok, createCVRequest{Title: "General"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	path := "/api/v1/me/cvs/" + created.Data.ID + "/template"

	// Valid registered template → 204, and it sticks on read.
	if resp := doCV(t, app, fiber.MethodPut, path, ownerTok, map[string]string{"template_id": "modern-sans"}); resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("set valid template = %d, want 204", resp.StatusCode)
	}
	resp = doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+created.Data.ID, ownerTok, nil)
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
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))} // storage disabled → seed no-ops
	app := buildCVApp(h, iss)

	beta := seedAccount(t, pool, "beta@example.test", true)
	other := seedAccount(t, pool, "other@example.test", true)
	plain := seedAccount(t, pool, "plain@example.test", false)
	betaTok, _ := iss.Issue(beta, testTokenVersion)
	otherTok, _ := iss.Issue(other, testTokenVersion)
	plainTok, _ := iss.Issue(plain, testTokenVersion)

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
	if created.Data.Title != "General" || id == "" {
		t.Fatalf("create returned %+v", created.Data)
	}

	// Update with a real document.
	doc := cv.Document{Header: cv.Header{FullName: "Ada Lovelace"}, Skills: []cv.SkillGroup{{Group: "Lang", Items: []string{"Go"}}}}
	upPath := "/api/v1/me/cvs/" + id
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
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}), resume: store}
	app := buildCVApp(h, iss)

	user := seedAccount(t, pool, "seed@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

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
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	// Name comes from the structure; the summary/tagline falls back to the headline line.
	if body.Data.Document.Header.FullName != "Seeded Ada" || body.Data.Document.Summary != "Backend Engineer" {
		t.Fatalf("seeded document = %+v / summary=%q, want name+summary from structure", body.Data.Document.Header, body.Data.Document.Summary)
	}
}

// A new CV's typography/margins come from the caller's saved appearance defaults, and its
// template falls back to the saved default's template when the request names none — see the
// add-cv-appearance-defaults change.
func TestCVCreate_UsesSavedAppearanceDefaults(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	cvStore := cv.NewStore(cv.NewQueriesRepository(queries))
	h := &cvHandlers{queries: queries, jobReader: queries, cvStore: cvStore,
		editor: cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))})}
	app := buildCVApp(h, iss)

	user := seedAccount(t, pool, "appearance-defaults@example.test", false)
	tok, _ := iss.Issue(user, testTokenVersion)

	saved := cv.AppearanceDefaults{
		TemplateID: "sidebar",
		Style:      cv.Style{FontFamily: "carlito", FontSize: 10, LineHeight: 0.65},
		Margins:    cv.Margins{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if _, err := cvStore.SetAppearanceDefaults(context.Background(), user, saved); err != nil {
		t.Fatalf("set appearance defaults: %v", err)
	}

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "New CV"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data cvResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.TemplateID != saved.TemplateID {
		t.Errorf("template = %q, want saved default %q", body.Data.TemplateID, saved.TemplateID)
	}
	if body.Data.Document.Style != saved.Style {
		t.Errorf("style = %+v, want saved default %+v", body.Data.Document.Style, saved.Style)
	}
	if body.Data.Document.Margins != saved.Margins {
		t.Errorf("margins = %+v, want saved default %+v", body.Data.Document.Margins, saved.Margins)
	}
}

// An explicit template_id on the create request still wins over a saved appearance default;
// typography/margins (which the request cannot express) still come from the saved default.
func TestCVCreate_ExplicitTemplateOverridesSavedDefault(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	cvStore := cv.NewStore(cv.NewQueriesRepository(queries))
	h := &cvHandlers{queries: queries, jobReader: queries, cvStore: cvStore,
		editor: cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))})}
	app := buildCVApp(h, iss)

	user := seedAccount(t, pool, "explicit-template@example.test", false)
	tok, _ := iss.Issue(user, testTokenVersion)

	saved := cv.AppearanceDefaults{
		TemplateID: "sidebar",
		Style:      cv.Style{FontSize: 10},
		Margins:    cv.Margins{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if _, err := cvStore.SetAppearanceDefaults(context.Background(), user, saved); err != nil {
		t.Fatalf("set appearance defaults: %v", err)
	}

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "New CV", TemplateID: "compact"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data cvResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.TemplateID != "compact" {
		t.Errorf("template = %q, want explicit request value %q", body.Data.TemplateID, "compact")
	}
	if body.Data.Document.Margins != saved.Margins {
		t.Errorf("margins = %+v, want saved default %+v (request cannot express margins)", body.Data.Document.Margins, saved.Margins)
	}
}

// Without saved appearance defaults, a new CV must still get the system's hardcoded
// defaults, exactly as before the add-cv-appearance-defaults change.
func TestCVCreate_NoSavedDefaultsUsesSystemDefaults(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))})}
	app := buildCVApp(h, iss)

	user := seedAccount(t, pool, "no-defaults@example.test", false)
	tok, _ := iss.Issue(user, testTokenVersion)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "New CV"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Data cvResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.TemplateID != cv.DefaultTemplateID {
		t.Errorf("template = %q, want system default %q", body.Data.TemplateID, cv.DefaultTemplateID)
	}
	if body.Data.Document.Margins != cv.DefaultMargins() {
		t.Errorf("margins = %+v, want system default %+v", body.Data.Document.Margins, cv.DefaultMargins())
	}
}

// The history feed and per-entry undo, end to end against a real database: the edits are
// recorded, undoing an older one leaves the newer in place, and the undo is itself an entry.
func TestCVRevisionHistoryAndUndo(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	tok, _ := iss.Issue(seedAccount(t, pool, "reviser@example.test", true), testTokenVersion)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "General"})
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, resp, &created)
	id := created.Data.ID

	// Two saves, each changing a different place, so neither coalesces into the other.
	for _, summary := range []string{"Backend engineer", "Platform engineer"} {
		body := updateCVRequest{Title: "General", TemplateID: "classic-ats",
			Document: cv.Document{Summary: summary, Header: cv.Header{FullName: "Ada"}}}
		if r := doCV(t, app, fiber.MethodPut, "/api/v1/me/cvs/"+id, tok, body); r.StatusCode != fiber.StatusOK {
			t.Fatalf("save %q = %d", summary, r.StatusCode)
		}
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/revisions", tok, nil), &feed)
	if len(feed.Data) < 2 {
		t.Fatalf("feed holds %d entries, want one per save (plus the seed): %+v", len(feed.Data), feed.Data)
	}
	newest := feed.Data[0]
	if newest.Title == "" || len(newest.Paths) == 0 {
		t.Fatalf("the newest entry says nothing about what it changed: %+v", newest)
	}

	// Undo the newest edit: the summary returns to what the previous save left.
	if r := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+id+"/revisions/"+newest.ID+"/undo", tok, nil); r.StatusCode != fiber.StatusOK {
		t.Fatalf("undo = %d", r.StatusCode)
	}
	var after struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+id, tok, nil), &after)
	if after.Data.Document.Summary != "Backend engineer" {
		t.Fatalf("summary = %q, want the text from before the undone edit", after.Data.Document.Summary)
	}

	// Undoing twice is refused rather than silently repeated.
	if r := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+id+"/revisions/"+newest.ID+"/undo", tok, nil); r.StatusCode != fiber.StatusConflict {
		t.Fatalf("second undo = %d, want 409", r.StatusCode)
	}

	// The undo is itself in the feed, and the entry it reversed is marked rather than removed.
	decodeJSON(t, doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/revisions", tok, nil), &feed)
	var sawUndo, sawReverted bool
	for _, r := range feed.Data {
		if r.RevertsID == newest.ID {
			sawUndo = true
		}
		if r.ID == newest.ID && r.Reverted {
			sawReverted = true
		}
	}
	if !sawUndo {
		t.Error("the undo did not appear in the feed")
	}
	if !sawReverted {
		t.Error("the reverted entry is not marked as undone")
	}
}

// Who made a change decides what the change is allowed to do: the agent is refused the
// candidate's own fields and must cite evidence for a claim, the candidate is refused
// nothing. So the actor has to come from the credential that authenticated the request and
// never from the request itself — otherwise the gate is one JSON field away from being
// optional.
//
// This test is the tripwire for that. It sends the actor in the body, by the name the wire
// shape uses, and asserts the recorded revision ignored it.
func TestTheActorIsNeverReadFromTheRequestBody(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	tok, _ := iss.Issue(seedAccount(t, pool, "actor@example.test", true), testTokenVersion)

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "General"}), &created)
	id := created.Data.ID

	// A whole-document save carrying an actor it is not entitled to. BodyParser ignores the
	// unknown field; what matters is that the recorded revision does too.
	body := map[string]any{
		"title":       "General",
		"template_id": "classic-ats",
		"actor":       "system",
		"origin":      "import",
		"document": map[string]any{
			"summary": "Ten years of Go",
			"header":  map[string]any{"full_name": "Ada"},
		},
	}
	if r := doCV(t, app, fiber.MethodPut, "/api/v1/me/cvs/"+id, tok, body); r.StatusCode != fiber.StatusOK {
		t.Fatalf("save = %d", r.StatusCode)
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/revisions", tok, nil), &feed)
	if len(feed.Data) == 0 {
		t.Fatal("the save left no revision")
	}
	for _, r := range feed.Data {
		if r.Actor != "candidate" {
			t.Errorf("revision recorded actor %q — the body was believed over the cookie", r.Actor)
		}
		if r.Origin != "editor" {
			t.Errorf("revision recorded origin %q — the body was believed over the entry point", r.Origin)
		}
	}
}

// Every entry point that changes a CV must leave an entry in its history: a change nobody
// recorded is a change nobody can undo, and the feed would be quietly incomplete.
func TestEveryEntryPointLeavesARevision(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)

	tok, _ := iss.Issue(seedAccount(t, pool, "entrypoints@example.test", true), testTokenVersion)

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "General"}), &created)
	id := created.Data.ID

	count := func() int {
		var feed struct {
			Data []cvedit.RevisionView `json:"data"`
		}
		decodeJSON(t, doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/revisions", tok, nil), &feed)
		return len(feed.Data)
	}

	before := count()
	if r := doCV(t, app, fiber.MethodPut, "/api/v1/me/cvs/"+id, tok, updateCVRequest{
		Title: "General", TemplateID: "classic-ats",
		Document: cv.Document{Summary: "Ten years of Go", Header: cv.Header{FullName: "Ada"}},
	}); r.StatusCode != fiber.StatusOK {
		t.Fatalf("whole-document save = %d", r.StatusCode)
	}
	if after := count(); after != before+1 {
		t.Errorf("a whole-document save left %d revisions, want one more than %d", after, before)
	}

	before = count()
	if r := doCV(t, app, fiber.MethodPut, "/api/v1/me/cvs/"+id+"/template", tok,
		setCVTemplateRequest{TemplateID: "centered"}); r.StatusCode != fiber.StatusNoContent {
		t.Fatalf("template pick = %d", r.StatusCode)
	}
	if after := count(); after != before+1 {
		t.Errorf("the template pick left %d revisions, want one more than %d", after, before)
	}
}

// UpdateCV (PUT /me/cvs/:id, the editor's autosave) goes through CommitDocument, which used
// to sanitize the incoming document before diffing — so an over-cap experience was silently
// truncated to cv.MaxBullets with no error, the exact loss cv_edit's ErrListCap refuses for
// an agent's insert. This pins that the whole-document save path refuses it too.
func TestUpdateCV_RefusesAWholeDocumentSaveOverTheBulletCap(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		resume:  resume.New(nil, resume.NewQueriesRepository(queries))}
	app := buildCVApp(h, iss)
	tok, _ := iss.Issue(seedAccount(t, pool, "bulletcap@example.test", true), testTokenVersion)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "General"})
	var created struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &created)
	id := created.Data.ID

	bullets := make([]string, cv.MaxBullets+1)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Pasted bullet %d", i+1)
	}
	overCap := cv.Document{
		Header:     cv.Header{FullName: "Ada Lovelace"},
		Experience: []cv.ExperienceItem{{Role: "Engineer", Company: "Acme", Bullets: bullets}},
	}
	upPath := "/api/v1/me/cvs/" + id
	putResp := doCV(t, app, fiber.MethodPut, upPath, tok, updateCVRequest{Title: "General", Document: overCap})
	if putResp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(putResp.Body)
		t.Fatalf("over-cap save = %d, want 409: %s", putResp.StatusCode, body)
	}

	var got struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, doCV(t, app, fiber.MethodGet, upPath, tok, nil), &got)
	if len(got.Data.Document.Experience) != 0 {
		t.Fatalf("a refused save must not have written anything: %+v", got.Data.Document.Experience)
	}
}
