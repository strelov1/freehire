//go:build integration

// Integration test for the engagement-stats read endpoint. The counts are pure
// aggregates over user_jobs, users, cvs, user_job_analysis, gmail_connections,
// mailboxes and saved_searches, and the handler reads through a concrete
// *db.Queries, so it can only be exercised against a real Postgres. It asserts the
// empty case, then seeds saves/applies/views plus a résumé, a tailored CV, a match
// analysis, both kinds of connected inbox and a saved search, and checks every
// count.
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

	h := &statsHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/engagement", h.EngagementStats)

	type counts struct {
		Saved            int `json:"saved"`
		Applied          int `json:"applied"`
		Viewed           int `json:"viewed"`
		CvsUploaded      int `json:"cvs_uploaded"`
		CvsTailored      int `json:"cvs_tailored"`
		MatchAnalyses    int `json:"match_analyses"`
		InboxesConnected int `json:"inboxes_connected"`
		SavedSearches    int `json:"saved_searches"`
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
		c.CvsUploaded != 0 || c.CvsTailored != 0 || c.MatchAnalyses != 0 ||
		c.InboxesConnected != 0 || c.SavedSearches != 0 {
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
			`WITH mark AS (
			     INSERT INTO user_jobs (user_id, job_id, viewed_at, saved_at)
			     VALUES ($1, $2, now(), CASE WHEN $3 THEN now() END)
			 )
			 INSERT INTO applications (user_id, job_id, company_slug, role_title, applied_at)
			 SELECT $1, $2, j.company_slug, j.title, now() FROM jobs j WHERE j.id = $2 AND $4`,
			uid, jid, saved, applied); err != nil {
			t.Fatalf("seed interaction job=%d: %v", jid, err)
		}
	}
	seedInteraction(j1, false, false)
	seedInteraction(j2, true, false)
	seedInteraction(j3, false, true)

	// A second user, holding no résumé and no CV, so the per-user counts below stay
	// pinned to the first one. It exists only to own the revoked Gmail grant —
	// gmail_connections is keyed by user_id, so both statuses need two users.
	var uid2 int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('v@example.test') RETURNING id`).Scan(&uid2); err != nil {
		t.Fatalf("seed second user: %v", err)
	}

	// A stored résumé (→ cvs_uploaded=1), one Analyze-match run (→ match_analyses=1),
	// and one saved search (→ saved_searches=1).
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'cv/u.pdf', resume_uploaded_at = now() WHERE id = $1`,
		uid); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_job_analysis (user_id, job_id, analysis, model)
		 VALUES ($1, $2, '{}'::jsonb, 'test-model')`, uid, j1); err != nil {
		t.Fatalf("seed match analysis: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'my search', 'go')`,
		uid); err != nil {
		t.Fatalf("seed saved search: %v", err)
	}

	// Two CVs, one tailored to j2 and one plain (→ cvs_tailored=1). The plain row is
	// what makes this an assertion about is_tailored rather than about count(cvs).
	if _, err := pool.Exec(ctx,
		`INSERT INTO cvs (user_id, title, data, job_id, is_tailored) VALUES
		   ($1, 'Tailored', '{}'::jsonb, $2, true),
		   ($1, 'Base',     '{}'::jsonb, NULL, false)`, uid, j2); err != nil {
		t.Fatalf("seed cvs: %v", err)
	}

	// One live Gmail grant + one claimed hosted mailbox = inboxes_connected 2. The
	// revoked grant on the second user must NOT count — that is the status filter.
	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status) VALUES
		   ($1, 'u@gmail.test', 'enc', 'connected'),
		   ($2, 'v@gmail.test', 'enc', 'needs_reconsent')`, uid, uid2); err != nil {
		t.Fatalf("seed gmail connections: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO mailboxes (user_id, address) VALUES ($1, 'u@mail.test')`, uid); err != nil {
		t.Fatalf("seed mailbox: %v", err)
	}

	// "viewed" is the all-traffic total: SUM(job_daily_views.uniques), the nginx-log
	// worker's rollup, not derived from user_jobs. Summed from the small rollup (not
	// SUM over the 6M-row jobs table, which seqscans for ~90s). Seed 5 + 2 + 1 = 8
	// across days/jobs.
	if _, err := pool.Exec(ctx,
		`INSERT INTO job_daily_views (day, job_id, uniques) VALUES
		   (DATE '2026-07-20', $1, 5),
		   (DATE '2026-07-20', $2, 2),
		   (DATE '2026-07-19', $1, 1)`,
		j1, j2); err != nil {
		t.Fatalf("seed job_daily_views: %v", err)
	}

	if c := get(); c.Saved != 1 || c.Applied != 1 || c.Viewed != 8 ||
		c.CvsUploaded != 1 || c.CvsTailored != 1 || c.MatchAnalyses != 1 ||
		c.InboxesConnected != 2 || c.SavedSearches != 1 {
		t.Errorf("got %+v, want {Saved:1 Applied:1 Viewed:8 CvsUploaded:1 CvsTailored:1 "+
			"MatchAnalyses:1 InboxesConnected:2 SavedSearches:1}", c)
	}
}
