//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

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

// A read that FAILED is not a candidate with nothing to seed from. Collapsing the two
// answered 201 with an empty skeleton to someone whose CV we hold, and there is no way back
// from it: the client opens the editor on a blank document and the candidate's only clue is
// that their history is missing. Every neighbouring caller of this same seed propagates
// (cv_reseed.go), and so must this one.
func TestCVCreate_SeedFailureIsNotAnEmptyCV(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: experience.NewStore(experience.NewQueriesRepository(queries))}),
		seeder:  unreadableSeeder{}}
	app := buildCVApp(h, iss)

	user := seedAccount(t, pool, "seed-unreadable@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs", tok, createCVRequest{Title: "Seeded", Seed: true})
	defer resp.Body.Close()
	if resp.StatusCode == fiber.StatusCreated {
		t.Fatalf("create = 201 despite an unreadable seed source; the candidate is handed a "+
			"blank CV and told it worked (status %d)", resp.StatusCode)
	}
	var created int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM cvs WHERE user_id = $1`, user).Scan(&created); err != nil {
		t.Fatalf("count cvs: %v", err)
	}
	if created != 0 {
		t.Errorf("stored %d CVs, want none — nothing should be written on a failed seed", created)
	}
}
