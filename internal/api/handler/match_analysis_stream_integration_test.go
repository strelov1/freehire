//go:build integration

// Integration test for the SSE fit-stream endpoint:
// GET /api/v1/jobs/:slug/fit/stream must emit ordered stage/section events ending in
// `final`, and cache the analysis. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// failingFitModel stands in for an upstream that is down or times out mid-stage — the
// shape of the production failure this test is about.
type failingFitModel struct{}

func (*failingFitModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return nil, errors.New("upstream llm exploded")
}
func (*failingFitModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// sseEvents reads an SSE body and returns the ordered list of event names.
func sseEvents(t *testing.T, body string) []string {
	t.Helper()
	var names []string
	for _, frame := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(frame, "\n") {
			if name, ok := strings.CutPrefix(line, "event: "); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func TestMatchAnalysisStreamEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID, jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('stream@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	makePro(t, pool, userID) // fit analysis is a paid feature
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, description, company_slug, public_slug, skills, content_hash)
		 VALUES ('test','s1','http://e.test','Senior Go Engineer','Build backends.','acme','stream-job', ARRAY['go'], 'h1')
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID, testTokenVersion)

	storeWithCV := func() *resume.Store {
		s := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
		if _, err := s.Put(ctx, userID, "text/plain", []byte("5y Go at Acme.")); err != nil {
			t.Fatalf("seed CV: %v", err)
		}
		if err := s.SetStructured(ctx, userID, resumeextract.Structured{Summary: "5y Go at Acme.", Skills: []string{"Go"}}, "test-model", resumeUploadedAt); err != nil {
			t.Fatalf("seed structured: %v", err)
		}
		seedBankedCareer(t, queries, userID)
		return s
	}

	appFor := func(store *resume.Store, an *matchanalysis.Analyzer) *fiber.App {
		h := &matchHandlers{
			queries:     queries,
			userProfile: userprofile.New(ownedProfile()),
			resume:      store, matchAnalysis: an,
			// Free gets no AI at all, and pro's allowance is unlimited by default; pinned
			// generously since this fixture is shared across several subtests run against
			// the same seeded user, whose daily counter accumulates real consumptions.
			fit: fitanalysis.New(queries,
				plan.NewStore(queries, pool, plan.DefaultConfig().Enforcing().WithProCeiling(plan.FeatureFit, fitProDailyForTests)), an),
		}
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss, testVersions), markPaidAI(plan.FeatureFit), h.StreamMatchAnalysis)
		return app
	}

	get := func(t *testing.T, app *fiber.App, tok string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/stream-job/fit/stream", nil)
		if tok != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
			// The shape the SPA's EventSource produces: a same-origin cookie request stamps
			// Fetch Metadata, and the route refuses a cookie request without it.
			req.Header.Set(headerSecFetchSite, secFetchSiteSameOrigin)
		}
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		defer resp.Body.Close()
		var b strings.Builder
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			b.WriteString(sc.Text())
			b.WriteString("\n")
		}
		return resp.StatusCode, b.String()
	}

	t.Run("unauthenticated is 401", func(t *testing.T) {
		if status, _ := get(t, appFor(storeWithCV(), matchanalysis.NewAnalyzer(nil)), ""); status != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	t.Run("streams ordered events and caches", func(t *testing.T) {
		model := &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}}
		app := appFor(storeWithCV(), matchanalysis.NewAnalyzer(llm.NewWithModel(model)))
		status, body := get(t, app, token)
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		names := sseEvents(t, body)
		if len(names) == 0 || names[0] != "meta" {
			t.Fatalf("first event = %v, want meta; all=%v", names, names)
		}
		if names[len(names)-1] != "final" {
			t.Errorf("last event = %q, want final; all=%v", names[len(names)-1], names)
		}
		joined := strings.Join(names, ",")
		for _, want := range []string{"stage_start", "requirements", "dimensions", "final"} {
			if !strings.Contains(joined, want) {
				t.Errorf("stream missing %q event; got %v", want, names)
			}
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jobID).Scan(&n)
		if n != 1 {
			t.Errorf("cache rows = %d, want 1 after stream", n)
		}
	})

	// The regression this whole change exists for. The handler returns nil long before the
	// chain runs, so a chain failure never reaches RenderError — the only place the API
	// reports a returned error. The reader sees `stream_error` and the access log records the 200
	// the stream opened with, which is why the outage looked like an empty error inbox.
	t.Run("a failed chain is reported", func(t *testing.T) {
		tr := recordExceptions(t)
		app := appFor(
			storeWithCV(),
			matchanalysis.NewAnalyzer(llm.NewWithModel(&failingFitModel{})),
		)
		status, body := get(t, app, token)
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200 (the stream opens before it fails)", status)
		}
		if names := sseEvents(t, body); len(names) == 0 || names[len(names)-1] != "stream_error" {
			t.Fatalf("last event = %v, want stream_error", names)
		}
		if got := tr.count(); got != 1 {
			t.Errorf("reported exceptions = %d, want exactly 1 for a failed chain", got)
		}
	})

	t.Run("no CV closes after meta", func(t *testing.T) {
		app := appFor(resume.New(newFakeResumeBlobs(), &fakeResumeRepo{}), matchanalysis.NewAnalyzer(nil))
		_, body := get(t, app, token)
		names := sseEvents(t, body)
		if len(names) != 1 || names[0] != "meta" {
			t.Errorf("events = %v, want just [meta] when no CV", names)
		}
		if !strings.Contains(body, `"has_cv":false`) {
			t.Errorf("meta should carry has_cv=false; body=%q", body)
		}
	})

	// The security regression this gate exists for, asserted at the side-effect boundary
	// rather than at the status code: a cross-site top-level navigation (browser stamps
	// Sec-Fetch-Site: cross-site; the attacker page cannot choose the stamp) carrying the
	// victim's valid Lax session cookie must not reserve a credit, run the model, or
	// persist an analysis. The same-origin EventSource shape is the control that proves
	// the refusal is the gate's doing and not the route being broken.
	//
	// Self-contained by a FRESH job: the earlier subtests of this suite have already
	// analysed stream-job for this user, so their ledger and cache rows exist and a count
	// against them would assert against somebody else's work.
	t.Run("cross-site navigation spends nothing", func(t *testing.T) {
		var csrfJobID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, description, company_slug, public_slug, skills, content_hash)
			 VALUES ('test','csrf-1','http://e.test','Senior Go Engineer','Build backends.','acme','csrf-job', ARRAY['go'], 'h-csrf')
			 RETURNING id`).Scan(&csrfJobID); err != nil {
			t.Fatalf("seed job: %v", err)
		}
		model := &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}}
		app := appFor(storeWithCV(), matchanalysis.NewAnalyzer(llm.NewWithModel(model)))

		ask := func(slug, secFetchSite string) int {
			t.Helper()
			req := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/jobs/"+slug+"/fit/stream", nil)
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			if secFetchSite != "" {
				req.Header.Set(headerSecFetchSite, secFetchSite)
			}
			resp, err := app.Test(req, 5000)
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			defer resp.Body.Close()
			// Drained here and returned only as a status: the denial is answered before any
			// body exists, while the CONTROL (the same-origin call) must be fully read —
			// the SSE body is produced by a goroutine that runs the chain and writes the
			// cache/ledger rows, so its side effects land only when the body is consumed.
			_, _ = io.Copy(io.Discard, resp.Body)
			return resp.StatusCode
		}
		// ledgerCount counts the user's fit rows in usage_ledger (plan.FeatureFit is the
		// string "match"): the consume a reservation takes and the release a failed run
		// writes both land there, so "the count did not move" is the shape of "nothing
		// was reserved, nothing was refunded either".
		ledgerCount := func() int {
			t.Helper()
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM usage_ledger WHERE user_id=$1 AND feature='match'`, userID).Scan(&n); err != nil {
				t.Fatalf("read usage_ledger: %v", err)
			}
			return n
		}
		analysesFor := func(jid int64) int {
			t.Helper()
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_job_analysis WHERE user_id=$1 AND job_id=$2`, userID, jid).Scan(&n); err != nil {
				t.Fatalf("read user_job_analysis: %v", err)
			}
			return n
		}

		// askBody issues one stream request and drains the SSE body fully, returning it.
		// The body being readable to its end is what guarantees the writer goroutine has
		// finished the chain — and therefore that the cache/ledger assertions below observe
		// settled state rather than a race.
		askBody := func(slug, tok, secFetchSite string) string {
			t.Helper()
			req := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/jobs/"+slug+"/fit/stream", nil)
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
			req.Header.Set(headerSecFetchSite, secFetchSite)
			resp, err := app.Test(req, 10000)
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("stream status = %d, want 200", resp.StatusCode)
			}
			return readSSEBody(t, resp.Body)
		}

		// The hostile navigation: refused before any body exists, so the status is the
		// whole answer and the side-effect reads below can follow immediately. The model
		// counter is the second half of the boundary: zero calls, not merely a refusal
		// status.
		before := ledgerCount()
		if status := ask("csrf-job", secFetchSiteCrossSite); status != fiber.StatusForbidden {
			t.Fatalf("cross-site stream = %d, want 403", status)
		}
		if model.n != 0 {
			t.Errorf("the refused navigation called the LLM %d times, want 0", model.n)
		}
		if n := ledgerCount(); n != before {
			t.Errorf("usage_ledger rows moved %d -> %d on the refused run, want unchanged — no credit may be reserved (or refunded)", before, n)
		}
		if n := analysesFor(csrfJobID); n != 0 {
			t.Errorf("user_job_analysis rows = %d after the refused run, want 0 — no analysis may be cached", n)
		}

		// The control: the same route, same cookie, same paid user — stamped the way the
		// SPA's EventSource is — runs the chain to COMPLETION (its terminal event is the
		// audited `final`, which only a successful cache write follows) and leaves exactly
		// one cached analysis and one new ledger row behind. A bare 200 header would not
		// prove the model finished; the drained body does.
		controlBody := askBody("csrf-job", token, secFetchSiteSameOrigin)
		if names := sseEvents(t, controlBody); len(names) == 0 || names[len(names)-1] != "final" {
			t.Fatalf("same-origin stream last event = %v, want final (the model must complete before the cache is written)", sseEvents(t, controlBody))
		}
		if n := analysesFor(csrfJobID); n != 1 {
			t.Errorf("user_job_analysis rows = %d after the same-origin run, want 1", n)
		}
		if n := ledgerCount(); n != before+1 {
			t.Errorf("usage_ledger rows = %d after the same-origin run, want %d (exactly the control run's own consumption)", n, before+1)
		}
	})
}
