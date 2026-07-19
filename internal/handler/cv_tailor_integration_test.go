//go:build integration

// Integration tests for the CV-tailoring HTTP surface (add-cv-tailoring): the tailoring
// bootstrap's preconditions (cached analysis + résumé) and success, field-level PATCH via a
// minted API key (apply / 422 bad addressing / 404 owner isolation), and the tailoring-context
// requirement split. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// newTailorAPI builds an API wired for the tailoring endpoints and truncates the tables.
func newTailorAPI(t *testing.T) (*API, *auth.Issuer) {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE cvs, users, jobs, user_job_analysis, api_keys RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss,
		cvStore:            cv.NewStore(cv.NewQueriesRepository(queries)),
		resume:             resume.New(nil, resume.NewQueriesRepository(queries)),
		matchAnalysisCache: queries,
		credits:            credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}),
	}
	return h, iss
}

// buildTailorApp wires the CV + tailoring routes with the real beta gate.
func buildTailorApp(h *API, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss)
	keyAuth := auth.RequireAuthOrKey(iss, h.queries)
	gate := auth.RequireModeratorOrBeta(h.queries, h.queries)
	app.Get("/api/v1/me/cvs/:id", keyAuth, gate, h.GetCV)
	app.Post("/api/v1/me/cvs/tailor", saved, gate, h.TailorCV)
	app.Patch("/api/v1/me/cvs/:id", keyAuth, gate, h.PatchCV)
	app.Get("/api/v1/me/cvs/:id/tailor-context", keyAuth, gate, h.TailorContext)
	return app
}

// doBearer issues a request authenticated by an API key (Authorization: Bearer).
func doBearer(t *testing.T, app *fiber.App, method, path, token string, body any) *http.Response {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func seedJobSlug(t *testing.T, h *API, slug string) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', $1, 'https://e.test/'||$1, 'Backend Engineer', $1) RETURNING id`,
		slug).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

func seedAnalysis(t *testing.T, h *API, userID, jobID int64) {
	t.Helper()
	blob, _ := json.Marshal(&matchanalysis.Analysis{
		Verdict: "Good Fit", OverallScore: 72, Recommendation: "Lead with Go depth.",
		Dimensions: []matchanalysis.Dimension{{Key: "skills", Label: "Skills", Score: 70, Comment: "solid"}},
		RequirementMatch: []matchanalysis.Requirement{
			{Text: "Kubernetes", Priority: "required", Status: matchanalysis.StatusMissingGap, Evidence: "absent"},
			{Text: "Go", Priority: "required", Status: matchanalysis.StatusMissingHave, Evidence: "in profile"},
			{Text: "REST", Priority: "preferred", Status: "covered", Evidence: "bullet 1"},
		},
		Strengths: []string{"Go depth"}, Gaps: []string{"K8s"},
	})
	if err := h.queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: blob, Model: "test-model",
	}); err != nil {
		t.Fatalf("seed analysis: %v", err)
	}
}

// seedFreshResume writes a structured résumé whose stamp matches the résumé upload time, so
// resume.Structured serves it (ok=true) and the base CV can be seeded from it.
func seedFreshResume(t *testing.T, h *API, userID int64) {
	t.Helper()
	st, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace", Summary: "Engineer", Skills: []string{"Go"}})
	at := time.Now().Truncate(time.Microsecond)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test-model'
		 WHERE id = $1`, userID, at, st); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}
}

func TestTailorCVBootstrap(t *testing.T) {
	h, iss := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedAccount(t, h, "tailor@example.test", true)
	tok, _ := iss.Issue(user)
	jobID := seedJobSlug(t, h, "backend-eng")

	// No cached analysis yet → 409.
	if resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"}); resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("no-analysis = %d, want 409", resp.StatusCode)
	}

	seedAnalysis(t, h, user, jobID)

	// Analysis present but no résumé to seed a base → 409.
	if resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"}); resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("no-résumé = %d, want 409", resp.StatusCode)
	}

	seedFreshResume(t, h, user)

	// Now bootstrap succeeds.
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Data.TailorCVID == 0 || got.Data.BaseCVID == 0 || got.Data.TailorCVID == got.Data.BaseCVID {
		t.Errorf("ids = %+v, want distinct non-zero", got.Data)
	}
	if got.Data.CLIToken == "" {
		t.Errorf("empty cli_token")
	}
	if got.Data.Analysis == nil || got.Data.Analysis.Verdict != "Good Fit" {
		t.Errorf("analysis not returned: %+v", got.Data.Analysis)
	}
	// Creating the tailored CV debited the tailor cost (3) from the monthly grant (20).
	var remaining int
	_ = h.pool.QueryRow(context.Background(), `SELECT remaining FROM credit_balances WHERE user_id=$1`, user).Scan(&remaining)
	if remaining != 17 {
		t.Errorf("remaining after tailor = %d, want 17 (20 - 3)", remaining)
	}
	var debits int
	_ = h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit' AND feature='tailor'`, user).Scan(&debits)
	if debits != 1 {
		t.Errorf("tailor debit rows = %d, want 1", debits)
	}
}

// TestTailorCVOutOfCredits: a caller with no points gets a 402 and no tailored CV or
// session is created, even with a cached analysis and a résumé present.
func TestTailorCVOutOfCredits(t *testing.T) {
	h, iss := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, h, "poor@example.test", true)
	tok, _ := iss.Issue(user)
	jobID := seedJobSlug(t, h, "backend-eng")
	seedAnalysis(t, h, user, jobID)
	seedFreshResume(t, h, user)

	// Force the current-period balance to zero so the tailor pre-check fails.
	period := time.Now().UTC().Format("2006-01")
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO credit_balances (user_id, period, remaining) VALUES ($1, $2, 0)`, user, period); err != nil {
		t.Fatalf("seed zero balance: %v", err)
	}

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("out-of-credits tailor = %d, want 402", resp.StatusCode)
	}
	resp.Body.Close()
	var cvs int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM cvs WHERE user_id=$1`, user).Scan(&cvs)
	if cvs != 0 {
		t.Errorf("cvs created on 402 = %d, want 0", cvs)
	}
}

func TestPatchCVViaKey(t *testing.T) {
	h, iss := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	owner := seedAccount(t, h, "owner@example.test", true)
	other := seedAccount(t, h, "other@example.test", true)

	base, err := h.cvStore.Create(ctx, owner, "General", cv.DefaultTemplateID, cv.Document{
		Experience: []cv.ExperienceItem{{Role: "Eng", Bullets: []string{"Shipped API"}}},
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}
	path := "/api/v1/me/cvs/" + strconv.FormatInt(base.ID, 10)

	ownerKey, err := mintTailoringKey(ctx, h.queries, owner, time.Now())
	if err != nil {
		t.Fatalf("mint owner key: %v", err)
	}
	otherKey, err := mintTailoringKey(ctx, h.queries, other, time.Now())
	if err != nil {
		t.Fatalf("mint other key: %v", err)
	}

	// A valid patch applies.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, cv.Patch{Op: cv.PatchAddBullet, Experience: 0, Value: "Cut latency"}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("patch = %d, want 200", resp.StatusCode)
	}
	rec, _ := h.cvStore.Get(ctx, base.ID, owner)
	if got := rec.Document.Experience[0].Bullets; len(got) != 2 || got[1] != "Cut latency" {
		t.Errorf("bullets after patch = %v", got)
	}

	// The minted key can also READ the CV back (GET /me/cvs/:id is keyAuth) — the agent
	// needs this to see the current document before patching.
	if resp := doBearer(t, app, fiber.MethodGet, path, ownerKey, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get via key = %d, want 200", resp.StatusCode)
	}

	// Bad addressing is a 422.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, cv.Patch{Op: cv.PatchReplaceBullet, Experience: 0, Bullet: 9, Value: "x"}); resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("bad-patch = %d, want 422", resp.StatusCode)
	}

	// Another user's key cannot touch this CV (owner isolation → 404, not 403).
	if resp := doBearer(t, app, fiber.MethodPatch, path, otherKey, cv.Patch{Op: cv.PatchSetSummary, Value: "hijack"}); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("foreign patch = %d, want 404", resp.StatusCode)
	}
}

func TestTailorContextSplit(t *testing.T) {
	h, iss := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, h, "ctx@example.test", true)
	jobID := seedJobSlug(t, h, "backend-eng")
	seedAnalysis(t, h, user, jobID)

	// A tailored CV bound to the vacancy.
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	key, _ := mintTailoringKey(ctx, h.queries, user, time.Now())

	resp := doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+strconv.FormatInt(tailored.ID, 10)+"/tailor-context", key, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("context = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data tailorContextResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if len(got.Data.MissingHave) != 1 || got.Data.MissingHave[0].Text != "Go" {
		t.Errorf("missing_have = %+v, want [Go]", got.Data.MissingHave)
	}
	if len(got.Data.MissingGap) != 1 || got.Data.MissingGap[0].Text != "Kubernetes" {
		t.Errorf("missing_gap = %+v, want [Kubernetes]", got.Data.MissingGap)
	}
	if got.Data.Verdict != "Good Fit" {
		t.Errorf("verdict = %q", got.Data.Verdict)
	}
	// The vacancy is bundled in so the agent sees the role it reframes toward.
	if got.Data.Job.Title != "Backend Engineer" || got.Data.Job.Slug != "backend-eng" {
		t.Errorf("job = %+v, want title=Backend Engineer slug=backend-eng", got.Data.Job)
	}

	// A base CV (no bound vacancy) is not tailorable-context → 409.
	base, _ := h.cvStore.Create(ctx, user, "Base", cv.DefaultTemplateID, cv.Document{})
	if resp := doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+strconv.FormatInt(base.ID, 10)+"/tailor-context", key, nil); resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("base-cv context = %d, want 409", resp.StatusCode)
	}
}

// TestCVSessionAndTailoredList covers the CV↔session link over real Postgres: SetSession
// round-trips on Get, ListTailored returns the vacancy slug + session (and excludes base CVs),
// and SetSession is owner-scoped.
func TestCVSessionAndTailoredList(t *testing.T) {
	h, _ := newTailorAPI(t)
	ctx := context.Background()
	user := seedAccount(t, h, "sess@example.test", true)
	jobID := seedJobSlug(t, h, "backend-eng")

	// A base CV (job_id NULL) must NOT appear in the tailored list.
	if _, err := h.cvStore.Create(ctx, user, "Base", cv.DefaultTemplateID, cv.Document{}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	if err := h.cvStore.SetSession(ctx, tailored.ID, user, "sess-xyz"); err != nil {
		t.Fatalf("set session: %v", err)
	}
	rec, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.AgentSessionID != "sess-xyz" {
		t.Errorf("session = %q, want sess-xyz", rec.AgentSessionID)
	}

	items, err := h.cvStore.ListTailored(ctx, user)
	if err != nil {
		t.Fatalf("list tailored: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("tailored list len = %d, want 1 (base excluded)", len(items))
	}
	if items[0].JobSlug != "backend-eng" || items[0].AgentSessionID != "sess-xyz" {
		t.Errorf("tailored item = %+v, want slug backend-eng + session sess-xyz", items[0])
	}

	other := seedAccount(t, h, "other-sess@example.test", true)
	if err := h.cvStore.SetSession(ctx, tailored.ID, other, "hijack"); !errors.Is(err, cv.ErrNotFound) {
		t.Errorf("foreign set-session err = %v, want ErrNotFound", err)
	}
}
