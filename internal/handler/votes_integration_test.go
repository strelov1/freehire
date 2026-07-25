//go:build integration

// Integration tests for the thumbs-vote wire contract against a real Postgres:
// the POST toggle/flip response shape, the DELETE clear, the 400 (bad direction)
// and 404 (unknown slug) gates, and the my_vote overlay on the auth-aware job
// detail read (present when signed in via OptionalAuth, 0 when anonymous). Run
// with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/vote"
)

type voteResult struct {
	UpvoteCount   int32 `json:"upvote_count"`
	DownvoteCount int32 `json:"downvote_count"`
	MyVote        int32 `json:"my_vote"`
}

func TestVoteEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('voter@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		 VALUES ('test', 'vote-1', 'http://example.test', 'Go Dev', 'go-dev-acme-t35nijto', 'acme')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	h := &voteHandlers{votes: vote.New(queries, pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	optionalAuth := auth.OptionalAuth(iss, queries)
	app.Post("/api/v1/jobs/:slug/vote", keyAuth, h.VoteJob)
	app.Delete("/api/v1/jobs/:slug/vote", keyAuth, h.ClearJobVote)
	app.Post("/api/v1/companies/:slug/vote", keyAuth, h.VoteCompany)
	app.Get("/api/v1/jobs/:slug", optionalAuth, (&API{pool: pool, queries: queries}).GetJob)

	post := func(t *testing.T, path, body string, authed bool) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authed {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		return resp
	}
	decode := func(t *testing.T, resp *http.Response) voteResult {
		t.Helper()
		var env struct {
			Data voteResult `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env.Data
	}

	const jobPath = "/api/v1/jobs/go-dev-acme-t35nijto/vote"

	// Anonymous cast is rejected.
	if r := post(t, jobPath, `{"vote":"up"}`, false); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon vote: want 401, got %d", r.StatusCode)
	}

	// Up-vote returns counters and my_vote.
	r := decode(t, post(t, jobPath, `{"vote":"up"}`, true))
	if r.UpvoteCount != 1 || r.DownvoteCount != 0 || r.MyVote != 1 {
		t.Fatalf("up-vote result: %+v", r)
	}

	// Re-cast up toggles to cleared.
	r = decode(t, post(t, jobPath, `{"vote":"up"}`, true))
	if r.UpvoteCount != 0 || r.MyVote != 0 {
		t.Fatalf("toggle-clear result: %+v", r)
	}

	// Down-vote, then the detail read reflects my_vote for the signed-in caller.
	r = decode(t, post(t, jobPath, `{"vote":"down"}`, true))
	if r.DownvoteCount != 1 || r.MyVote != -1 {
		t.Fatalf("down-vote result: %+v", r)
	}

	// Invalid direction -> 400 before any write.
	if resp := post(t, jobPath, `{"vote":"sideways"}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid direction: want 400, got %d", resp.StatusCode)
	}
	// Unknown slug -> 404.
	if resp := post(t, "/api/v1/jobs/ghost/vote", `{"vote":"up"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown slug: want 404, got %d", resp.StatusCode)
	}

	// Detail read: signed-in caller sees my_vote = -1; anonymous sees 0 with the
	// public counter intact.
	detail := func(t *testing.T, authed bool) voteResult {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/go-dev-acme-t35nijto", nil)
		if authed {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("detail request: %v", err)
		}
		var env struct {
			Data voteResult `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("detail decode: %v", err)
		}
		return env.Data
	}
	if d := detail(t, true); d.MyVote != -1 || d.DownvoteCount != 1 {
		t.Fatalf("signed-in detail: %+v", d)
	}
	if d := detail(t, false); d.MyVote != 0 || d.DownvoteCount != 1 {
		t.Fatalf("anon detail: %+v", d)
	}

	// Company vote path (basic).
	cr := decode(t, post(t, "/api/v1/companies/acme/vote", `{"vote":"up"}`, true))
	if cr.UpvoteCount != 1 || cr.MyVote != 1 {
		t.Fatalf("company up-vote: %+v", cr)
	}
	if resp := post(t, "/api/v1/companies/ghost/vote", `{"vote":"up"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown company: want 404, got %d", resp.StatusCode)
	}
}
