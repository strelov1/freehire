//go:build integration

// Integration tests for the CV-tailoring HTTP surface (add-cv-tailoring): the tailoring
// bootstrap's one remaining precondition (a résumé to seed a base CV from) and success,
// field-level PATCH via a minted API key (apply / 422 bad addressing / 404 owner isolation), and
// the tailoring-context requirement split. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// newTailorAPI builds a handler wired for the tailoring endpoints and truncates the tables.
func newTailorAPI(t *testing.T) (*cvHandlers, *auth.Issuer, *pgxpool.Pool) {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE cvs, users, jobs, user_jobs, applications, user_job_analysis, api_keys, assistant_sessions, experience_employments, experience_atoms RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	plans := plan.NewStore(queries, pool, plan.DefaultConfig().Enforcing())
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	resumeStore := resume.New(nil, resume.NewQueriesRepository(queries))
	h := &cvHandlers{queries: queries, jobReader: queries,
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		// The REAL gate, built exactly as Register builds it. A nil gate here would be a
		// configuration production does not have: newCVHandlers always receives
		// bankGate{bank}, so an API-key PATCH edits as the agent and an uncited claim is
		// refused. A fixture asserting otherwise tests nothing that ships.
		editor: cvedit.NewEditor(cvedit.NewRepository(pool, queries),
			bankGate{bank: bank}),
		resume: resumeStore,
		seeder: bankedSeeder{resume: resumeStore, bank: bank},
		fit:    fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		plans:  plans,
		match:  &matchHandlers{fit: fitanalysis.New(queries, plans, matchanalysis.NewAnalyzer(nil))},
		// Same adapter Register wires: bootstrap must place the vacancy on the board.
		jobs: trackingBoarder{repo: jobtracking.NewQueriesRepository(queries, pool)},
		// The tailoring bootstrap mints its conversation through the assistant's store,
		// exactly as Register wires it.
		assistantSessions: assistant.NewStore(queries),
	}
	return h, iss, pool
}

// buildTailorApp wires the CV + tailoring routes. They are open to every signed-in user (the
// beta gate was lifted when CV tailoring went public); credits meter the LLM spend instead.
func buildTailorApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	// The CV surface mounts the SCOPED middleware in production, because the key the
	// tailoring bootstrap mints is narrow (auth.ScopeCV). Mirroring that here is the
	// point of the test: a harness on the full-scope-only middleware would reject the
	// very credential the flow issues.
	keyAuth := auth.RequireAuthOrScopedKey(iss, testVersions, apiKeys{h.queries}, auth.ScopeCV)
	app.Get("/api/v1/me/cvs/:id", keyAuth, h.GetCV)
	app.Post("/api/v1/me/cvs/tailor", saved, h.TailorCV)
	app.Patch("/api/v1/me/cvs/:id", keyAuth, h.PatchCV)
	app.Get("/api/v1/me/cvs/:id/tailor-context", keyAuth, h.TailorContext)
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

func seedJobSlug(t *testing.T, pool *pgxpool.Pool, slug string) int64 {
	t.Helper()
	return seedJobSlugDesc(t, pool, slug, "")
}

func seedJobSlugDesc(t *testing.T, pool *pgxpool.Pool, slug, description string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, description)
		 VALUES ('test', $1, 'https://e.test/'||$1, 'Backend Engineer', $1, $2) RETURNING id`,
		slug, description).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

func seedAnalysis(t *testing.T, h *cvHandlers, userID, jobID int64) {
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
func seedFreshResume(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	seedFreshResumeSkills(t, pool, userID, []string{"Go"})
}

func seedFreshResumeSkills(t *testing.T, pool *pgxpool.Pool, userID int64, skills []string) {
	t.Helper()
	st, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace", Summary: "Engineer", Skills: skills})
	at := time.Now().Truncate(time.Microsecond)
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test-model'
		 WHERE id = $1`, userID, at, st); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}
}

func TestTailorCVBootstrap(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedAccount(t, pool, "tailor@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "backend-eng")

	// No résumé to seed a base → 409. No cached analysis exists either, and that no longer
	// gates the bootstrap — the résumé gate is the only precondition left.
	if resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"}); resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("no-résumé = %d, want 409", resp.StatusCode)
	}

	seedFreshResume(t, pool, user)

	// Bootstrap succeeds with no cached analysis: the tailored CV is created, and the
	// response's analysis is nil since none has been computed yet.
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Data.TailorCVID == "" || got.Data.BaseCVID == "" || got.Data.TailorCVID == got.Data.BaseCVID {
		t.Errorf("ids = %+v, want two distinct ids", got.Data)
	}
	// The bootstrap mints the tailoring conversation itself: a tailor-preset session
	// bound to this CV and vacancy, already stored on the CV so reopening the
	// workspace resumes it instead of starting over.
	if got.Data.SessionID == "" {
		t.Fatalf("bootstrap returned no session id")
	}
	var boundPreset, boundSession, boundCV string
	var boundJob int64
	if err := pool.QueryRow(context.Background(),
		`SELECT s.preset, s.cv_id, s.job_id, c.agent_session_id
		   FROM assistant_sessions s JOIN cvs c ON c.id = s.cv_id
		  WHERE s.id = $1`, got.Data.SessionID).Scan(&boundPreset, &boundCV, &boundJob, &boundSession); err != nil {
		t.Fatalf("read the minted session: %v", err)
	}
	if boundPreset != assistant.PresetTailor || boundCV != got.Data.TailorCVID || boundJob != jobID {
		t.Errorf("session = preset %q cv %s job %d, want a tailor session bound to cv %s / job %d",
			boundPreset, boundCV, boundJob, got.Data.TailorCVID, jobID)
	}
	if boundSession != got.Data.SessionID {
		t.Errorf("cv.agent_session_id = %q, want the minted session %q", boundSession, got.Data.SessionID)
	}
	if got.Data.Analysis != nil {
		t.Errorf("analysis = %+v, want nil (none cached yet)", got.Data.Analysis)
	}
	if !got.Data.ColdStartRunning {
		t.Errorf("cold_start_running = false, want true on a bootstrap that just created the CV")
	}
	// Creating the tailored CV consumed one of the day's tailoring sessions.
	var used int
	_ = pool.QueryRow(context.Background(),
		`SELECT used FROM usage_daily WHERE user_id=$1 AND feature='tailor' AND day=(now() AT TIME ZONE 'utc')::date`, user).Scan(&used)
	if used != 1 {
		t.Errorf("tailoring sessions used = %d, want 1", used)
	}
	var charges int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM usage_ledger WHERE user_id=$1 AND kind='consume' AND feature='tailor'`, user).Scan(&charges)
	if charges != 1 {
		t.Errorf("tailoring charges = %d, want 1", charges)
	}
	assertVacancyOnKanban(t, pool, user, jobID)
}

// assertVacancyOnKanban checks the tailor bootstrap's tracking side-effect: the vacancy
// is bookmarked AND staged as preparing (so it lands in the Preparing Kanban column),
// without applied_at (preparing a CV is not submitting an application), and the ledger
// event that set the stage is attributed to the platform, not the candidate.
func assertVacancyOnKanban(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) {
	t.Helper()
	var saved bool
	var appliedAt pgtype.Timestamptz
	var stage pgtype.Text
	err := pool.QueryRow(context.Background(),
		`SELECT uj.saved_at IS NOT NULL, a.applied_at, a.stage
		   FROM user_jobs uj
		   LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
		  WHERE uj.user_id = $1 AND uj.job_id = $2`, userID, jobID).
		Scan(&saved, &appliedAt, &stage)
	if err != nil {
		t.Fatalf("read tracking row: %v", err)
	}
	if !saved {
		t.Errorf("saved_at unset — tailored vacancy should stay bookmarked")
	}
	if !stage.Valid || stage.String != "preparing" {
		t.Errorf("stage = %q, want preparing — the Kanban's own column, not applied", stage.String)
	}
	if appliedAt.Valid {
		t.Errorf("applied_at = %v, want null — tailor is not an application submit", appliedAt.Time)
	}
	rows, err := db.New(pool).ListUserJobs(context.Background(), db.ListUserJobsParams{
		UserID: userID, Limit: 50, Offset: 0, Filter: "board",
	})
	if err != nil {
		t.Fatalf("ListUserJobs board: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.ID == jobID {
			found = true
			if !r.Stage.Valid || r.Stage.String != "preparing" {
				t.Errorf("board row stage = %q, want preparing", r.Stage.String)
			}
			break
		}
	}
	if !found {
		t.Errorf("job %d missing from filter=board listing after tailor", jobID)
	}

	var source string
	if err := pool.QueryRow(context.Background(),
		`SELECT source FROM application_events
		  WHERE user_id = $1 AND job_id = $2 AND kind = 'stage_set'
		  ORDER BY id DESC LIMIT 1`, userID, jobID).Scan(&source); err != nil {
		t.Fatalf("read stage_set event source: %v", err)
	}
	if source != "system" {
		t.Errorf("stage_set event source = %q, want system — the platform placed this, not the candidate", source)
	}
}

// TestTailorCVBootstrap_HealsMissingSave: an existing tailored CV whose vacancy was
// never (or no longer) on the board gets staged on the next bootstrap resume.
func TestTailorCVBootstrap_HealsMissingSave(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "tailor-heal@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "heal-eng")
	seedFreshResume(t, pool, user)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "heal-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("first bootstrap = %d, want 201", resp.StatusCode)
	}
	var first struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()
	assertVacancyOnKanban(t, pool, user, jobID)

	if _, err := pool.Exec(ctx,
		`UPDATE applications SET stage = NULL WHERE user_id = $1 AND job_id = $2`, user, jobID); err != nil {
		t.Fatalf("clear stage: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE user_jobs SET saved_at = NULL WHERE user_id = $1 AND job_id = $2`, user, jobID); err != nil {
		t.Fatalf("clear save: %v", err)
	}

	resp = doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "heal-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("resume bootstrap = %d, want 201", resp.StatusCode)
	}
	var second struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&second)
	resp.Body.Close()
	if second.Data.TailorCVID != first.Data.TailorCVID {
		t.Fatalf("resume made a new CV (%s vs %s)", second.Data.TailorCVID, first.Data.TailorCVID)
	}
	assertVacancyOnKanban(t, pool, user, jobID)

	// An advanced stage must not be pulled back to applied on resume.
	if _, err := pool.Exec(ctx,
		`UPDATE applications SET stage = 'interview' WHERE user_id = $1 AND job_id = $2`, user, jobID); err != nil {
		t.Fatalf("advance stage: %v", err)
	}
	resp = doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "heal-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("third bootstrap = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
	var stage string
	if err := pool.QueryRow(ctx,
		`SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2`, user, jobID).Scan(&stage); err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if stage != "interview" {
		t.Errorf("stage = %q, want interview — tailor must not overwrite progress", stage)
	}

	// Clearing the board marks must not delete the tailored CV.
	if _, err := pool.Exec(ctx,
		`UPDATE applications SET stage = NULL WHERE user_id = $1 AND job_id = $2`, user, jobID); err != nil {
		t.Fatalf("clear stage again: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE user_jobs SET saved_at = NULL WHERE user_id = $1 AND job_id = $2`, user, jobID); err != nil {
		t.Fatalf("clear save again: %v", err)
	}
	var cvs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM cvs WHERE user_id = $1 AND job_id = $2`, user, jobID).Scan(&cvs); err != nil {
		t.Fatalf("count cvs: %v", err)
	}
	if cvs != 1 {
		t.Errorf("cvs after clear = %d, want 1", cvs)
	}
}

type boomBoarder struct{}

func (boomBoarder) EnsureOnBoard(context.Context, int64, int64) error {
	return errors.New("board boom")
}

// TestTailorCVBootstrap_SaveFailureStillSucceeds: once the CV and session exist, a
// tracking write error must not turn the bootstrap into a failure.
func TestTailorCVBootstrap_SaveFailureStillSucceeds(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	h.jobs = boomBoarder{}
	app := buildTailorApp(h, iss)

	user := seedAccount(t, pool, "tailor-save-fail@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "fail-eng")
	seedFreshResume(t, pool, user)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "fail-eng"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap with board failure = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Data.TailorCVID == "" || got.Data.SessionID == "" {
		t.Fatalf("response = %+v, want CV and session ids despite board failure", got.Data)
	}
	var cvs int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cvs WHERE user_id = $1 AND job_id = $2`, user, jobID).Scan(&cvs); err != nil {
		t.Fatalf("count cvs: %v", err)
	}
	if cvs != 1 {
		t.Errorf("cvs = %d, want 1", cvs)
	}
}

// TestTailorCVOutOfCredits: a caller with no points gets a 402 and no tailored CV or
// session is created, even with a cached analysis and a résumé present.
func TestTailorCVOutOfCredits(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "poor@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "backend-eng")
	seedAnalysis(t, h, user, jobID)
	seedFreshResume(t, pool, user)

	// Spend today's whole tailoring allowance so the bootstrap pre-check refuses. The
	// counter is what the pre-check reads; seeding it directly is the state a candidate
	// who already opened today's sessions would be in.
	if _, err := pool.Exec(ctx,
		`INSERT INTO usage_daily (user_id, feature, day, used) VALUES ($1, 'tailor', CURRENT_DATE, $2)`,
		user, plan.DefaultConfig().FreeDaily(plan.FeatureTailor)); err != nil {
		t.Fatalf("seed a spent allowance: %v", err)
	}

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("out-of-credits tailor = %d, want 402", resp.StatusCode)
	}
	resp.Body.Close()
	var cvs int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM cvs WHERE user_id=$1`, user).Scan(&cvs)
	if cvs != 0 {
		t.Errorf("cvs created on 402 = %d, want 0", cvs)
	}
}

func TestPatchCVViaKey(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	owner := seedAccount(t, pool, "owner@example.test", true)
	other := seedAccount(t, pool, "other@example.test", true)

	base, err := h.cvStore.Create(ctx, owner, "General", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Ada Lovelace", Email: "ada@x.com", Phone: "+1 415 555 0000"},
		Experience: []cv.ExperienceItem{{Role: "Eng", Bullets: []string{"Shipped API"}}},
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}
	path := "/api/v1/me/cvs/" + base.ID.String()

	// The agent (API key) must never see the contact block; the owner's cookie read does.
	decodeHeader := func(resp *http.Response) cv.Header {
		var w struct {
			Data struct {
				Document cv.Document `json:"document"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&w)
		return w.Data.Document.Header
	}

	ownerKey := cvScopedKey(t, h, owner)
	otherKey := cvScopedKey(t, h, other)

	// The key edits as the AGENT, so a bullet stating something about the candidate has to
	// cite banked evidence. Uncited is refused — this is the rule the tailoring capability
	// exists to enforce, and it must hold for an API-key caller with no assistant in sight.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "insert", "path": "experience[0].bullets[1]", "value": "Cut latency"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("uncited patch = %d, want 403", resp.StatusCode)
	}

	// Cited by an atom the candidate asserted, the same batch applies — so the gate is a
	// wall with a door, not a blanket refusal.
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	atom, err := bank.AddAtom(ctx, owner, experience.Atom{
		Claim: "Cut latency", Provenance: experience.ProvenanceStatedInChat,
	},
		experience.AuthorQuoted,
	)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{
			"kind": "insert", "path": "experience[0].bullets[1]",
			"value": "Cut latency", "evidence_id": atom.ID.String(),
		}},
	}); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("cited patch = %d, want 200", resp.StatusCode)
	}
	rec, _ := h.cvStore.Get(ctx, base.ID, owner)
	if got := rec.Document.Experience[0].Bullets; len(got) != 2 || got[1] != "Cut latency" {
		t.Errorf("bullets after patch = %v", got)
	}

	// The minted key can also READ the CV back (GET /me/cvs/:id is keyAuth) — the agent
	// needs this to see the current document before patching — but the contact block is stripped.
	if resp := doBearer(t, app, fiber.MethodGet, path, ownerKey, nil); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get via key = %d, want 200", resp.StatusCode)
	} else if h := decodeHeader(resp); h.FullName != "" || h.Email != "" || h.Phone != "" {
		t.Errorf("agent read leaked the contact block: %+v", h)
	}

	// The owner's own cookie read sees the full contact block.
	ownerCookie, _ := iss.Issue(owner, testTokenVersion)
	greq := httptest.NewRequest(fiber.MethodGet, path, nil)
	greq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: ownerCookie})
	if resp, err := app.Test(greq); err != nil {
		t.Fatalf("owner get: %v", err)
	} else if h := decodeHeader(resp); h.FullName != "Ada Lovelace" || h.Email != "ada@x.com" {
		t.Errorf("owner read should keep contacts, got %+v", h)
	}

	// The agent cannot WRITE a contact field either.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "header.full_name", "value": "Fake Name"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("patch full_name via key = %d, want 403", resp.StatusCode)
	}

	// Bad addressing is a 422 — but only once the evidence gate has let the op through. The
	// gate is the OUTER check: an uncited claim is 403 whatever its path addresses, which the
	// old nil-gate fixture could never show. Citing the banked atom is what makes this case
	// about addressing again.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{
			"kind": "set", "path": "experience[0].bullets[9]",
			"value": "x", "evidence_id": atom.ID.String(),
		}},
	}); resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("bad-patch = %d, want 422", resp.StatusCode)
	}

	// And the ordering itself, pinned: the same malformed op WITHOUT evidence is refused by
	// the gate rather than reaching the addressing check.
	if resp := doBearer(t, app, fiber.MethodPatch, path, ownerKey, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "experience[0].bullets[9]", "value": "x"}},
	}); resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("uncited bad-patch = %d, want 403 — the gate runs before addressing", resp.StatusCode)
	}

	// Another user's key cannot touch this CV (owner isolation → 404, not 403). The op is a
	// REMOVE because the evidence gate runs first and only gates claim-bearing set/insert:
	// with a claim-bearing op the foreign caller is refused at the gate (403) and never
	// reaches the ownership check. Isolation is intact either way — 403 is what an uncited
	// claim gets whether or not the CV exists, so neither answer discloses one — but only a
	// non-claiming op actually exercises the 404.
	if resp := doBearer(t, app, fiber.MethodPatch, path, otherKey, map[string]any{
		"ops": []map[string]any{{"kind": "remove", "path": "experience[0].bullets[0]"}},
	}); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("foreign patch = %d, want 404", resp.StatusCode)
	}
}

func TestTailorContextSplit(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "ctx@example.test", true)
	jobID := seedJobSlug(t, pool, "backend-eng")
	seedAnalysis(t, h, user, jobID)

	// A tailored CV bound to the vacancy.
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	key := cvScopedKey(t, h, user)

	resp := doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String()+"/tailor-context", key, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("context = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data fitanalysis.TailoringContext `json:"data"`
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
	if resp := doBearer(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+base.ID.String()+"/tailor-context", key, nil); resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("base-cv context = %d, want 409", resp.StatusCode)
	}
}

// TestCVSessionAndTailoredList covers the CV↔session link over real Postgres: SetSession
// round-trips on Get, ListTailored returns the vacancy slug + session (and excludes base CVs),
// and SetSession is owner-scoped.
func TestCVSessionAndTailoredList(t *testing.T) {
	h, _, pool := newTailorAPI(t)
	ctx := context.Background()
	user := seedAccount(t, pool, "sess@example.test", true)
	jobID := seedJobSlug(t, pool, "backend-eng")

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

	other := seedAccount(t, pool, "other-sess@example.test", true)
	if err := h.cvStore.SetSession(ctx, tailored.ID, other, "hijack"); !errors.Is(err, cv.ErrNotFound) {
		t.Errorf("foreign set-session err = %v, want ErrNotFound", err)
	}
}

// cvScopedKey issues a user-created `cv`-scoped API key — the credential the public
// CLI authenticates its CV commands with. The in-app agent no longer uses one (it
// runs as the caller), but the scope and its owner checks still guard this surface.
func cvScopedKey(t *testing.T, h *cvHandlers, userID int64) string {
	t.Helper()
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := h.queries.CreateAPIKey(context.Background(), db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        "cv scope",
		TokenHash:   hash,
		TokenPrefix: prefix,
		Scope:       auth.ScopeCV,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}
	return token
}

// Reloading /tailor/<slug> re-runs the bootstrap: the address carries no CV reference, so the
// page asks for a tailored CV again. That must reach the SAME CV and the SAME conversation —
// three CVs appeared on one vacancy in half an hour on production, each with an empty chat,
// because every refresh minted another copy and rebound the session.
func TestTailorCVBootstrapIsIdempotentPerVacancy(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedAccount(t, pool, "tailor-reload@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	jobID := seedJobSlug(t, pool, "backend-eng")
	seedAnalysis(t, h, user, jobID)
	seedFreshResume(t, pool, user)

	bootstrap := func() tailorCVResponse {
		t.Helper()
		resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "backend-eng"})
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
		}
		var got struct {
			Data tailorCVResponse `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		return got.Data
	}

	first := bootstrap()
	if !first.ColdStartRunning {
		t.Errorf("first bootstrap: cold_start_running = false, want true (it just created the CV)")
	}
	// Something was said in that conversation, so losing it would be losing real work.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO assistant_messages (session_id, seq, role, content)
		 VALUES ($1, 1, 'user', '{"text":"tailor it for me"}'::jsonb)`, first.SessionID); err != nil {
		t.Fatalf("seed a message: %v", err)
	}

	second := bootstrap()

	if second.TailorCVID != first.TailorCVID {
		t.Errorf("reload made a new CV (%s vs %s)", second.TailorCVID, first.TailorCVID)
	}
	if second.SessionID != first.SessionID {
		t.Errorf("reload rebound the conversation (%s vs %s) — the candidate's messages are on the old one", second.SessionID, first.SessionID)
	}
	if second.ColdStartRunning {
		t.Errorf("reload: cold_start_running = true, want false — the CV already existed")
	}

	var cvs, sessions int
	if err := pool.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM cvs WHERE user_id = $1 AND job_id = $2),
		        (SELECT count(*) FROM assistant_sessions WHERE user_id = $1 AND job_id = $2)`,
		user, jobID).Scan(&cvs, &sessions); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cvs != 1 || sessions != 1 {
		t.Errorf("after two bootstraps: %d CVs and %d sessions, want one of each", cvs, sessions)
	}

	// And the charge happened once, not twice.
	var charges int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM usage_ledger WHERE user_id = $1 AND kind = 'consume' AND feature = 'tailor'`,
		user).Scan(&charges); err != nil {
		t.Fatalf("count charges: %v", err)
	}
	if charges != 1 {
		t.Errorf("tailoring charges = %d, want 1 — a reload is not a second purchase", charges)
	}
}

// TestTailorCVBootstrap_ReseedsStaleBaseAfterUpload: base predates a newer résumé
// upload → first tailor for a new vacancy refreshes base + tailored from the seed.
func TestTailorCVBootstrap_ReseedsStaleBaseAfterUpload(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "stale-base@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	// Old seed stamp + base content that predates the later upload.
	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	oldBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Old Name", Summary: "before upload", Skills: []string{"Go"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test'
		 WHERE id = $1`, user, oldAt, oldBlob); err != nil {
		t.Fatalf("seed old résumé: %v", err)
	}
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Old Name"},
		Summary: "before upload",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	// Newer upload + structure (seed source moves; cvs intentionally untouched).
	newAt := time.Now().Truncate(time.Microsecond)
	newBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace", Summary: "after upload", Skills: []string{"Go", "Kafka"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, newAt, newBlob); err != nil {
		t.Fatalf("seed new résumé: %v", err)
	}

	jobID := seedJobSlug(t, pool, "stale-base-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "stale-base-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Summary != "after upload" || tailored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("tailored = %+v / %q, want after-upload seed", tailored.Document.Header, tailored.Document.Summary)
	}
	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.ID != base.ID {
		t.Fatalf("base id changed: %s → %s", base.ID, refreshed.ID)
	}
	if refreshed.Document.Summary != "after upload" {
		t.Fatalf("base summary = %q, want after upload", refreshed.Document.Summary)
	}

	// Idempotent reload keeps the same tailored id and does not wipe content.
	resp2 := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "stale-base-job"})
	if resp2.StatusCode != fiber.StatusCreated {
		t.Fatalf("reload = %d", resp2.StatusCode)
	}
	var got2 struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp2.Body).Decode(&got2)
	resp2.Body.Close()
	if got2.Data.TailorCVID != got.Data.TailorCVID {
		t.Fatalf("reload minted new tailored CV")
	}
}

// Pending extraction after a re-upload must not blank a filled base header: the bank may
// have roles, but without a current structured résumé the seed is not usable for replace.
func TestTailorCVBootstrap_PendingStructureDoesNotBlankHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "pending-struct@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{
			FullName: "Ada Lovelace",
			Email:    "ada@example.com",
			Phone:    "+351 900 000 000",
			Location: "Lisbon, PT",
			Links:    []string{"github.com/ada"},
		},
		Summary: "keep me",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	// Newer upload, but no current structure (extraction pending / stamp mismatch).
	newAt := time.Now().Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = NULL, resume_structured_uploaded_at = NULL WHERE id = $1`,
		user, newAt); err != nil {
		t.Fatalf("seed pending résumé: %v", err)
	}
	seedBankedCareer(t, h.queries, user)

	jobID := seedJobSlug(t, pool, "pending-struct-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "pending-struct-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	want := cv.Header{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+351 900 000 000",
		Location: "Lisbon, PT", Links: []string{"github.com/ada"},
	}
	if refreshed.Document.Header.FullName != want.FullName ||
		refreshed.Document.Header.Email != want.Email ||
		refreshed.Document.Header.Phone != want.Phone ||
		refreshed.Document.Header.Location != want.Location ||
		len(refreshed.Document.Header.Links) != 1 || refreshed.Document.Header.Links[0] != want.Links[0] {
		t.Fatalf("base header blanked: %+v, want %+v", refreshed.Document.Header, want)
	}
	if refreshed.Document.Summary != "keep me" {
		t.Fatalf("base summary = %q, want keep me", refreshed.Document.Summary)
	}
}

// A newer, CURRENT structured extract that itself carries identity alone (e.g. an
// extraction that only recovered a name) must not blank the base's existing body either —
// "current" is not the same as "has body content". Mirrors the pending-structure case
// above but for the "current, yet bodyless" seed hasSeedBody exists to catch.
func TestTailorCVBootstrap_IdentityOnlyCurrentStructureDoesNotBlankBase(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "identity-only-current@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Old Base Name"},
		Summary:    "hand-written summary I typed myself",
		Experience: []cv.ExperienceItem{{Company: "Real Job I Had"}},
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	newAt := time.Now().Truncate(time.Microsecond)
	newBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, newAt, newBlob); err != nil {
		t.Fatalf("seed identity-only current résumé: %v", err)
	}

	jobID := seedJobSlug(t, pool, "identity-only-current-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "identity-only-current-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.Document.Summary != "hand-written summary I typed myself" || len(refreshed.Document.Experience) != 1 {
		t.Fatalf("base body blanked by identity-only reseed: summary=%q experience=%+v",
			refreshed.Document.Summary, refreshed.Document.Experience)
	}
	if refreshed.Document.Header.FullName != "Old Base Name" {
		t.Fatalf("header = %+v, want existing name kept (heal is keep-first)", refreshed.Document.Header)
	}
}

func TestTailorCVBootstrap_KeepsBaseEditedAfterUpload(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "edited-base@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	uploadAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "From Seed", Summary: "from seed", Skills: []string{"Go"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, uploadAt, blob); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}

	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Hand Edited"},
		Summary: "hand edited",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	// Ensure updated_at is after the upload stamp.
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`,
		base.ID, uploadAt.Add(time.Minute)); err != nil {
		t.Fatalf("stamp base: %v", err)
	}

	jobID := seedJobSlug(t, pool, "edited-base-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "edited-base-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Summary != "hand edited" {
		t.Fatalf("tailored summary = %q, want hand edited (no forced reseed)", tailored.Document.Summary)
	}
}

// Empty base + stale structure with contacts + bank: tailor heals the header without
// wiping a hand-written summary (provisional path is header-only on reseed).
func TestTailorCVBootstrap_ProvisionalContactsFillEmptyHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "provisional-header@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	oldBlob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+44",
		Summary: "stale summary", Skills: []string{"Go"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test'
		 WHERE id = $1`, user, oldAt, oldBlob); err != nil {
		t.Fatalf("seed old structure: %v", err)
	}
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{},
		Summary: "keep me",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}
	newAt := time.Now().Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`, user, newAt); err != nil {
		t.Fatalf("newer upload stamp: %v", err)
	}
	seedBankedCareer(t, h.queries, user)

	jobID := seedJobSlug(t, pool, "provisional-header-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "provisional-header-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.Document.Header.FullName != "Ada Lovelace" || refreshed.Document.Header.Email != "ada@example.com" {
		t.Fatalf("base header = %+v, want provisional contacts", refreshed.Document.Header)
	}
	if refreshed.Document.Summary != "keep me" {
		t.Fatalf("base summary = %q, want keep me (provisional reseed is header-only)", refreshed.Document.Summary)
	}

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("tailored header = %+v, want healed name", tailored.Document.Header)
	}
}

func TestGetCV_HealsEmptyTailoredHeaderFromProvisional(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "heal-get@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale stamp: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-get-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	resp := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Data.Document.Header.FullName != "Ada Lovelace" || body.Data.Document.Header.Email != "ada@example.com" {
		t.Fatalf("response header = %+v", body.Data.Document.Header)
	}
	if body.Data.Document.Summary != "body" {
		t.Fatalf("summary = %q, want body unchanged", body.Data.Document.Summary)
	}

	stored, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("stored header not persisted: %+v", stored.Document.Header)
	}
}

func TestGetCV_SecondHealDoesNotWriteRevision(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "heal-once@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale stamp: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-once-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	first := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first GET = %d", first.StatusCode)
	}
	first.Body.Close()

	var afterFirst int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cv_revisions WHERE cv_id = $1`, tailored.ID).Scan(&afterFirst); err != nil {
		t.Fatalf("count after first: %v", err)
	}
	if afterFirst < 1 {
		t.Fatalf("revisions after first GET = %d, want at least the heal", afterFirst)
	}

	second := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if second.StatusCode != fiber.StatusOK {
		t.Fatalf("second GET = %d", second.StatusCode)
	}
	second.Body.Close()

	var afterSecond int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cv_revisions WHERE cv_id = $1`, tailored.ID).Scan(&afterSecond); err != nil {
		t.Fatalf("count after second: %v", err)
	}
	if afterSecond != afterFirst {
		t.Fatalf("revisions after second GET = %d, want %d (no new revision)", afterSecond, afterFirst)
	}
}

func TestListCVsAndPDFDoNotHealHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	h.cvRenderer = &fakeCVRenderer{pdf: []byte("%PDF-1.4 fake")}
	app := buildTailorApp(h, iss)
	saved := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/me/cvs", saved, h.ListCVs)
	app.Get("/api/v1/me/cvs/:id/pdf", saved, h.RenderCVPDF)
	ctx := context.Background()

	user := seedAccount(t, pool, "heal-list-pdf@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-list-pdf-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	list := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs", tok, nil)
	if list.StatusCode != fiber.StatusOK {
		t.Fatalf("LIST = %d", list.StatusCode)
	}
	list.Body.Close()

	afterList, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get after list: %v", err)
	}
	if afterList.Document.Header.FullName != "" {
		t.Fatalf("list healed header: %+v", afterList.Document.Header)
	}

	pdf := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String()+"/pdf", tok, nil)
	if pdf.StatusCode != fiber.StatusOK {
		t.Fatalf("PDF = %d", pdf.StatusCode)
	}
	pdf.Body.Close()

	afterPDF, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get after pdf: %v", err)
	}
	if afterPDF.Document.Header.FullName != "" {
		t.Fatalf("pdf healed header: %+v", afterPDF.Document.Header)
	}
}

func TestGetCV_PartialHeaderKeepsNameFillsEmail(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedAccount(t, pool, "heal-partial@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "From Blob", Email: "blob@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(time.Hour)); err != nil {
		t.Fatalf("stale: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-partial-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Keep Me"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resp := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Data.Document.Header.FullName != "Keep Me" {
		t.Fatalf("name = %q, want Keep Me", body.Data.Document.Header.FullName)
	}
	if body.Data.Document.Header.Email != "blob@example.com" {
		t.Fatalf("email = %q, want filled from provisional", body.Data.Document.Header.Email)
	}
}

// TestTailorCVAlignsSkillSurfaceOnMint: a fresh tailored copy stores the vacancy's preferred
// spelling of a skill the seed already carries (IaC → infrastructure as code). A second
// bootstrap must not re-align — a candidate who put the acronym back keeps it.
func TestTailorCVAlignsSkillSurfaceOnMint(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)

	user := seedAccount(t, pool, "align-mint@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	seedJobSlugDesc(t, pool, "align-mint-job",
		"We practice infrastructure as code and Terraform daily.")
	seedFreshResumeSkills(t, pool, user, []string{"IaC", "Terraform"})

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "align-mint-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	read := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, nil)
	var doc struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	json.NewDecoder(read.Body).Decode(&doc)
	read.Body.Close()
	if len(doc.Data.Document.Skills) == 0 || len(doc.Data.Document.Skills[0].Items) == 0 {
		t.Fatalf("skills = %+v, want at least one chip", doc.Data.Document.Skills)
	}
	chip := doc.Data.Document.Skills[0].Items[0]
	if chip != "infrastructure as code" {
		t.Fatalf("skill chip = %q, want vacancy preferred form", chip)
	}

	// Candidate puts the acronym back; second bootstrap must leave it alone.
	patch := doCV(t, app, fiber.MethodPatch, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, map[string]any{
		"ops": []map[string]any{{"kind": "set", "path": "skills[0].items[0]", "value": "IaC"}},
	})
	if patch.StatusCode != fiber.StatusOK {
		t.Fatalf("patch = %d, want 200", patch.StatusCode)
	}
	patch.Body.Close()

	again := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "align-mint-job"})
	if again.StatusCode != fiber.StatusOK && again.StatusCode != fiber.StatusCreated {
		t.Fatalf("second bootstrap = %d", again.StatusCode)
	}
	var againGot struct {
		Data tailorCVResponse `json:"data"`
	}
	json.NewDecoder(again.Body).Decode(&againGot)
	again.Body.Close()
	if againGot.Data.TailorCVID != got.Data.TailorCVID {
		t.Fatalf("second bootstrap minted a new CV %s, want existing %s", againGot.Data.TailorCVID, got.Data.TailorCVID)
	}

	read2 := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+got.Data.TailorCVID, tok, nil)
	json.NewDecoder(read2.Body).Decode(&doc)
	read2.Body.Close()
	if doc.Data.Document.Skills[0].Items[0] != "IaC" {
		t.Fatalf("after second bootstrap chip = %q, want user-edited IaC left in place", doc.Data.Document.Skills[0].Items[0])
	}
}

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	return id
}
