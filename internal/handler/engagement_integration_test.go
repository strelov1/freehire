//go:build integration

// Integration test for the engagement-stats read endpoint. The counts are a pure
// aggregate over user_jobs and the handler reads through a concrete *db.Queries,
// so it can only be exercised against a real Postgres. It asserts the empty case,
// then seeds saves/applies/views and checks the counts.
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
		Saved   int `json:"saved"`
		Applied int `json:"applied"`
		Viewed  int `json:"viewed"`
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

	// --- Empty table: all zeros ------------------------------------------------
	if c := get(); c.Saved != 0 || c.Applied != 0 || c.Viewed != 0 {
		t.Fatalf("empty table: got %+v, want all zeros", c)
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
	// Expected: saved=1, applied=1, viewed=3 (viewed_at is set on every row).
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

	if c := get(); c.Saved != 1 || c.Applied != 1 || c.Viewed != 3 {
		t.Errorf("got %+v, want {Saved:1 Applied:1 Viewed:3}", c)
	}
}
