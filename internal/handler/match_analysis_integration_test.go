//go:build integration

// Integration test for the on-demand LLM fit endpoints:
// GET /api/v1/jobs/:slug/fit serves the cached analysis (or a null one), and
// POST computes the three-stage chain, caches it per (user, job), and returns it
// fresh. The job/company lookup and the cache hit a real Postgres; the LLM is a
// canned model and the CV a fake blob store. Run with:
//
//	go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/userprofile"
)

// fit stage responses for the canned three-stage model.
const (
	fitStage1 = `{"requirements":[{"text":"Go","priority":"required","status":"covered","evidence":"5y"},{"text":"Kafka","priority":"preferred","status":"missing-gap"}]}`
	fitStage2 = `{"title_alignment":{"score":80},"experience_relevance":{"score":70},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"strengths":["Strong Go"],"gaps":["No Kafka"],"recommendation":"Apply."}`
	fitStage3 = `{"title_alignment":{"score":80},"experience_relevance":{"score":60},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"strengths":["Strong Go"],"gaps":["No Kafka"],"recommendation":"Apply, address Kafka."}`
)

// fitModel returns canned responses in order — one per stage of the chain.
type fitModel struct {
	resp []string
	n    int
}

func (m *fitModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	// Cycle the canned stage responses so a test can drive more than one full analysis
	// through the same model; m.n stays a total-calls counter for "LLM not called" checks.
	r := m.resp[m.n%len(m.resp)]
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}
func (*fitModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

type fitBody struct {
	Data struct {
		HasCV    bool             `json:"has_cv"`
		Stale    bool             `json:"stale"`
		Analysis *matchanalysis.Analysis `json:"analysis"`
	} `json:"data"`
}

func fitAPI(pool *pgxpool.Pool, queries *db.Queries, iss *auth.Issuer, store *resume.Store, an *matchanalysis.Analyzer) *API {
	return &API{
		pool: pool, queries: queries, issuer: iss,
		userProfile: userprofile.New(ownedProfile()),
		resume:      store, matchAnalysis: an, matchAnalysisCache: queries,
		credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}),
	}
}

func TestMatchAnalysisEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID, jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('fit@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, description, company_slug, public_slug, skills, content_hash)
		 VALUES ('test','f1','http://e.test','Senior Go Engineer','Build backends in Go.',
		         'acme','fit-job', ARRAY['go'], 'hash-1') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme','Acme', '{"tagline":"We ship"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	storeWithCVFor := func(t *testing.T) *resume.Store {
		t.Helper()
		s := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
		if _, err := s.Put(ctx, userID, "text/plain", []byte("Backend engineer, 5y Go at Acme.")); err != nil {
			t.Fatalf("seed CV: %v", err)
		}
		return s
	}

	do := func(t *testing.T, app *fiber.App, method, slug, tok string) (int, fitBody) {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/jobs/"+slug+"/fit", nil)
		if tok != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s fit: %v", method, err)
		}
		defer resp.Body.Close()
		var body fitBody
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	appFor := func(store *resume.Store, an *matchanalysis.Analyzer) *fiber.App {
		h := fitAPI(pool, queries, iss, store, an)
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		g := auth.RequireAuth(iss)
		app.Get("/api/v1/jobs/:slug/fit", g, h.GetMatchAnalysis)
		app.Post("/api/v1/jobs/:slug/fit", g, h.PostMatchAnalysis)
		return app
	}

	t.Run("unauthenticated is 401", func(t *testing.T) {
		app := appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(nil))
		if status, _ := do(t, app, fiber.MethodGet, "fit-job", ""); status != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	t.Run("unknown slug is 404", func(t *testing.T) {
		app := appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(nil))
		if status, _ := do(t, app, fiber.MethodGet, "no-such", token); status != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", status)
		}
	})

	t.Run("GET without a stored CV → has_cv false, no LLM", func(t *testing.T) {
		app := appFor(resume.New(newFakeResumeBlobs(), &fakeResumeRepo{}), matchanalysis.NewAnalyzer(nil))
		status, body := do(t, app, fiber.MethodGet, "fit-job", token)
		if status != fiber.StatusOK || body.Data.HasCV {
			t.Errorf("got status=%d has_cv=%v, want 200/false", status, body.Data.HasCV)
		}
	})

	t.Run("GET never-analyzed → has_cv true, null analysis", func(t *testing.T) {
		app := appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(nil))
		status, body := do(t, app, fiber.MethodGet, "fit-job", token)
		if status != fiber.StatusOK || !body.Data.HasCV || body.Data.Analysis != nil {
			t.Errorf("got status=%d has_cv=%v analysis=%v, want 200/true/nil", status, body.Data.HasCV, body.Data.Analysis)
		}
	})

	t.Run("POST LLM off → has_cv true, null analysis, nothing cached", func(t *testing.T) {
		app := appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(nil))
		status, body := do(t, app, fiber.MethodPost, "fit-job", token)
		if status != fiber.StatusOK || !body.Data.HasCV || body.Data.Analysis != nil {
			t.Errorf("got status=%d has_cv=%v analysis=%v, want 200/true/nil", status, body.Data.HasCV, body.Data.Analysis)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1`, userID).Scan(&n)
		if n != 0 {
			t.Errorf("cache rows = %d, want 0 with LLM off", n)
		}
	})

	t.Run("POST computes, caches, GET returns fresh", func(t *testing.T) {
		model := &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}}
		app := appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(llm.NewWithModel(model)))

		status, body := do(t, app, fiber.MethodPost, "fit-job", token)
		if status != fiber.StatusOK || body.Data.Analysis == nil {
			t.Fatalf("POST got status=%d analysis=%v, want 200 + analysis", status, body.Data.Analysis)
		}
		if body.Data.Analysis.Verdict == "" || len(body.Data.Analysis.Dimensions) != 6 {
			t.Errorf("analysis = %+v, want verdict + 6 dimensions", body.Data.Analysis)
		}
		if len(body.Data.Analysis.RequirementMatch) != 2 {
			t.Errorf("requirement_match = %d, want 2", len(body.Data.Analysis.RequirementMatch))
		}

		// The row was cached and a fresh GET (no CV/job change) serves it non-stale, no LLM.
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jobID).Scan(&n)
		if n != 1 {
			t.Fatalf("cache rows = %d, want 1", n)
		}
		gstatus, gbody := do(t, appFor(storeWithCVFor(t), matchanalysis.NewAnalyzer(nil)), fiber.MethodGet, "fit-job", token)
		if gstatus != fiber.StatusOK || gbody.Data.Analysis == nil || gbody.Data.Stale {
			t.Errorf("GET after compute = status %d stale %v analysis %v, want 200/false/present",
				gstatus, gbody.Data.Stale, gbody.Data.Analysis)
		}
	})
}

// TestMatchAnalysisCredits covers the points gate on the match feature: a new job with no points
// is a 402 (no LLM call, nothing persisted), a recompute of an already-analyzed job is
// always free, a fresh analysis debits one point, and GET reports the balance.
func TestMatchAnalysisCredits(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)

	seedUser := func(t *testing.T, email string) (int64, string) {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		tok, err := iss.Issue(id)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		return id, tok
	}
	seedJob := func(t *testing.T, ext, slug string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, description, public_slug, skills, content_hash)
			 VALUES ('test',$1,'http://e.test','Go Engineer','Build backends in Go.',$2, ARRAY['go'], 'h')
			 RETURNING id`, ext, slug).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", slug, err)
		}
		return id
	}
	// seedAnalysis places a prior analysis row for (user, job) at the given age, so it
	// counts toward (recent) or outside (old) the rolling window.
	seedAnalysis := func(t *testing.T, userID, jobID int64, age time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_job_analysis (user_id, job_id, analysis, model, created_at)
			 VALUES ($1,$2,'{}','seed-model', now() - $3::interval)`,
			userID, jobID, age.String()); err != nil {
			t.Fatalf("seed analysis: %v", err)
		}
	}
	storeWithCVFor := func(t *testing.T, userID int64) *resume.Store {
		t.Helper()
		s := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
		if _, err := s.Put(ctx, userID, "text/plain", []byte("5y Go.")); err != nil {
			t.Fatalf("seed CV: %v", err)
		}
		return s
	}
	appFor := func(store *resume.Store, an *matchanalysis.Analyzer, grant int) *fiber.App {
		h := &API{
			pool: pool, queries: queries, issuer: iss,
			userProfile: userprofile.New(ownedProfile()),
			resume:      store, matchAnalysis: an, matchAnalysisCache: queries,
			credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: grant, CostMatch: 1, CostTailor: 3}),
		}
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		g := auth.RequireAuth(iss)
		app.Get("/api/v1/jobs/:slug/fit", g, h.GetMatchAnalysis)
		app.Post("/api/v1/jobs/:slug/fit", g, h.PostMatchAnalysis)
		app.Get("/api/v1/jobs/:slug/fit/stream", g, h.StreamMatchAnalysis)
		return app
	}
	postFit := func(t *testing.T, app *fiber.App, slug, tok string) (int, fitBody) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodPost, "/api/v1/jobs/"+slug+"/fit", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("POST fit: %v", err)
		}
		defer resp.Body.Close()
		var body fitBody
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	newModel := func() *fitModel { return &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}} }

	t.Run("new job with no points is 402 and never calls the LLM", func(t *testing.T) {
		userID, token := seedUser(t, "broke@example.test")
		seedJob(t, "broke-new", "broke-new")
		model := newModel()
		app := appFor(storeWithCVFor(t, userID), matchanalysis.NewAnalyzer(llm.NewWithModel(model)), 0)

		status, _ := postFit(t, app, "broke-new", token)
		if status != fiber.StatusPaymentRequired {
			t.Errorf("status = %d, want 402", status)
		}
		if model.n != 0 {
			t.Errorf("LLM was called %d times, want 0 when out of credits", model.n)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1`, userID).Scan(&n)
		if n != 0 {
			t.Errorf("cache rows = %d, want 0 (nothing persisted on 402)", n)
		}
	})

	t.Run("recompute of an analyzed job is free even with no points", func(t *testing.T) {
		userID, token := seedUser(t, "recompute@example.test")
		jid := seedJob(t, "rc", "rc-job")
		seedAnalysis(t, userID, jid, time.Hour) // prior cache row → recompute, not a new job
		model := newModel()
		app := appFor(storeWithCVFor(t, userID), matchanalysis.NewAnalyzer(llm.NewWithModel(model)), 0)

		status, body := postFit(t, app, "rc-job", token)
		if status != fiber.StatusOK || body.Data.Analysis == nil {
			t.Fatalf("recompute got status=%d analysis=%v, want 200 + analysis", status, body.Data.Analysis)
		}
		if model.n == 0 {
			t.Error("recompute must run the LLM")
		}
		var debits int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND kind='debit'`, userID).Scan(&debits)
		if debits != 0 {
			t.Errorf("recompute debited %d points, want 0", debits)
		}
	})

	t.Run("a fresh analysis debits one point and GET reports the balance", func(t *testing.T) {
		userID, token := seedUser(t, "spend@example.test")
		seedJob(t, "spend-1", "spend-1")
		app := appFor(storeWithCVFor(t, userID), matchanalysis.NewAnalyzer(llm.NewWithModel(newModel())), 2)

		// GET before compute: full grant remaining, nothing consumed.
		greq := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/spend-1/fit", nil)
		greq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		gresp, _ := app.Test(greq)
		var gbody struct {
			Data struct {
				Credits *credits.Balance `json:"credits"`
			} `json:"data"`
		}
		_ = json.NewDecoder(gresp.Body).Decode(&gbody)
		gresp.Body.Close()
		if gbody.Data.Credits == nil || gbody.Data.Credits.Remaining != 2 {
			t.Fatalf("GET credits = %+v, want remaining=2", gbody.Data.Credits)
		}

		// First new job debits to 1.
		if status, body := postFit(t, app, "spend-1", token); status != fiber.StatusOK || body.Data.Analysis == nil {
			t.Fatalf("first analysis status=%d analysis=%v, want 200 + analysis", status, body.Data.Analysis)
		}
		var remaining int
		_ = pool.QueryRow(ctx, `SELECT remaining FROM credit_balances WHERE user_id=$1`, userID).Scan(&remaining)
		if remaining != 1 {
			t.Errorf("remaining after one match = %d, want 1", remaining)
		}
		// Second new job debits to 0; a third is then out of credits (402).
		seedJob(t, "spend-2", "spend-2")
		if status, _ := postFit(t, app, "spend-2", token); status != fiber.StatusOK {
			t.Fatalf("second analysis status=%d, want 200", status)
		}
		seedJob(t, "spend-3", "spend-3")
		if status, _ := postFit(t, app, "spend-3", token); status != fiber.StatusPaymentRequired {
			t.Errorf("third new job status = %d, want 402", status)
		}
	})

	t.Run("stream out of credits is 402 before opening the stream", func(t *testing.T) {
		userID, token := seedUser(t, "stream-broke@example.test")
		seedJob(t, "sb-new", "sb-new")
		model := newModel()
		app := appFor(storeWithCVFor(t, userID), matchanalysis.NewAnalyzer(llm.NewWithModel(model)), 0)

		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/sb-new/fit/stream", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusPaymentRequired {
			t.Errorf("stream status = %d, want 402", resp.StatusCode)
		}
		if model.n != 0 {
			t.Errorf("LLM called %d times, want 0 for an out-of-credits stream", model.n)
		}
	})
}
