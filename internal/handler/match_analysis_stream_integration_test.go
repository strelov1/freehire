//go:build integration

// Integration test for the SSE fit-stream endpoint:
// GET /api/v1/jobs/:slug/fit/stream must emit ordered stage/section events ending in
// `final`, and cache the analysis. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/userprofile"
)

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
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, description, company_slug, public_slug, skills, content_hash)
		 VALUES ('test','s1','http://e.test','Senior Go Engineer','Build backends.','acme','stream-job', ARRAY['go'], 'h1')
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, _ := iss.Issue(userID)

	storeWithCV := func() *resume.Store {
		s := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
		if _, err := s.Put(ctx, userID, "text/plain", []byte("5y Go at Acme.")); err != nil {
			t.Fatalf("seed CV: %v", err)
		}
		return s
	}

	appFor := func(store *resume.Store, an *matchanalysis.Analyzer) *fiber.App {
		h := &API{
			pool: pool, queries: queries, issuer: iss,
			userProfile: userprofile.New(ownedProfile()),
			resume:      store, matchAnalysis: an, matchAnalysisCache: queries,
			credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3}),
		}
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		app.Get("/api/v1/jobs/:slug/fit/stream", auth.RequireAuth(iss), h.StreamMatchAnalysis)
		return app
	}

	get := func(t *testing.T, app *fiber.App, tok string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/stream-job/fit/stream", nil)
		if tok != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
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
}
