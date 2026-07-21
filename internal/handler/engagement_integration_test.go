//go:build integration

// Integration test for the engagement-stats read endpoint. The counts are pure
// aggregates over user_jobs, users, user_job_analysis and saved_searches, and the
// handler reads through a concrete *db.Queries, so it can only be exercised against
// a real Postgres. It asserts the empty case, then seeds saves/applies/views plus a
// résumé, a fit analysis and a saved search, and checks every count.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func TestEngagementStatsEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/engagement", h.EngagementStats)

	type counts struct {
		Saved         int `json:"saved"`
		Applied       int `json:"applied"`
		Viewed        int `json:"viewed"`
		CvsUploaded   int `json:"cvs_uploaded"`
		FitChecks     int `json:"fit_checks"`
		SavedSearches int `json:"saved_searches"`
	}
	type envelope struct {
		Data counts `json:"data"`
	}
	get := func() counts {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/stats/engagement", nil))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200 (public, unauthenticated read)", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env.Data
	}

	// --- Empty tables: all zeros -----------------------------------------------
	if c := get(); c.Saved != 0 || c.Applied != 0 || c.Viewed != 0 ||
		c.CvsUploaded != 0 || c.FitChecks != 0 || c.SavedSearches != 0 {
		t.Fatalf("empty tables: got %+v, want all zeros", c)
	}

	// --- Seed a user + jobs + interactions -------------------------------------
	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('u@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jobID := func(ext string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test', 'J', $1) RETURNING id`, ext).Scan(&id); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
		return id
	}
	j1, j2, j3 := jobID("j1"), jobID("j2"), jobID("j3")

	// j1: viewed only; j2: viewed + saved; j3: viewed + applied.
	// saved/applied come from user_jobs (→ saved=1, applied=1). "viewed" is now the
	// all-traffic total SUM(jobs.view_count), independent of user_jobs, seeded below.
	seedInteraction := func(jid int64, saved, applied bool) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_jobs (user_id, job_id, viewed_at, saved_at, applied_at)
			 VALUES ($1, $2, now(),
			         CASE WHEN $3 THEN now() END,
			         CASE WHEN $4 THEN now() END)`,
			uid, jid, saved, applied); err != nil {
			t.Fatalf("seed interaction job=%d: %v", jid, err)
		}
	}
	seedInteraction(j1, false, false)
	seedInteraction(j2, true, false)
	seedInteraction(j3, false, true)

	// A stored résumé (→ cvs_uploaded=1), one job-fit analysis (→ fit_checks=1),
	// and one saved search (→ saved_searches=1).
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'cv/u.pdf', resume_uploaded_at = now() WHERE id = $1`,
		uid); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_job_analysis (user_id, job_id, analysis, model)
		 VALUES ($1, $2, '{}'::jsonb, 'test-model')`, uid, j1); err != nil {
		t.Fatalf("seed fit analysis: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'my search', 'go')`,
		uid); err != nil {
		t.Fatalf("seed saved search: %v", err)
	}

	// "viewed" is the all-traffic total: SUM(jobs.view_count), maintained by the
	// nginx-log worker, not derived from user_jobs. Seed counts 5 + 3 + 0 = 8.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET view_count = CASE id WHEN $1 THEN 5 WHEN $2 THEN 3 ELSE 0 END`,
		j1, j2); err != nil {
		t.Fatalf("seed view_count: %v", err)
	}

	if c := get(); c.Saved != 1 || c.Applied != 1 || c.Viewed != 8 ||
		c.CvsUploaded != 1 || c.FitChecks != 1 || c.SavedSearches != 1 {
		t.Errorf("got %+v, want {Saved:1 Applied:1 Viewed:8 CvsUploaded:1 FitChecks:1 SavedSearches:1}", c)
	}
}
