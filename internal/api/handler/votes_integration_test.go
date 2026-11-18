//go:build integration

// Integration tests for the thumbs-vote wire contract against a real Postgres:
// the POST toggle/flip response shape, the DELETE clear, the 400 (bad direction)
// and 404 (unknown slug) gates, and the my_vote overlay on the auth-aware job
// detail read (present when signed in via OptionalAuth, 0 when anonymous). Run
// with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/vote"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
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
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	h := &voteHandlers{votes: vote.New(queries, pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	optionalAuth := auth.OptionalAuth(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/:slug/vote", keyAuth, newContract(contractHandlers{voteHandlers: h}))
	app.Delete("/api/v1/jobs/:slug/vote", keyAuth, newContract(contractHandlers{voteHandlers: h}))
	app.Post("/api/v1/companies/:slug/vote", keyAuth, newContract(contractHandlers{voteHandlers: h}))
	mountJobsContract(app, &jobsHandlers{queries: queries}, middleware{optional: optionalAuth})

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

// TestVotePrivateJobNonOwner reproduces #4763: another user's private posting must not
// resolve through /jobs/find, must 404 on vote without writing a user_jobs row, and must
// not list on the tracking board even when such a row already exists.
func TestVotePrivateJobNonOwner(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var owner, other int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('owner@example.test') RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&other); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	const slug = "secret-role-secret-co-y6n25ddy"
	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug, is_private, created_by)
		 VALUES ('weblink', 'private-1', 'https://example.com/jobs/123', 'Secret Role', 'Secret Co', $1, true, $2)
		 RETURNING id`, slug, owner).Scan(&jobID); err != nil {
		t.Fatalf("seed private job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &voteHandlers{votes: vote.New(queries, pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/:slug/vote", keyAuth, newContract(contractHandlers{voteHandlers: h}))
	app.Delete("/api/v1/jobs/:slug/vote", keyAuth, newContract(contractHandlers{voteHandlers: h}))
	mountJobsContract(app, &jobsHandlers{queries: queries}, middleware{})

	// do returns the status and body, closing the response.
	do := func(t *testing.T, method, path, body string, userID int64) (int, []byte) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if userID != 0 {
			cookie, err := iss.Issue(userID, testTokenVersion)
			if err != nil {
				t.Fatalf("issue token: %v", err)
			}
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return resp.StatusCode, out
	}

	_, body := do(t, http.MethodGet, "/api/v1/jobs/find?url=https%3A%2F%2Fexample.com%2Fjobs%2F123", "", 0)
	var found struct {
		Data *struct {
			PublicSlug string `json:"public_slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &found); err != nil {
		t.Fatalf("decode find: %v", err)
	}
	if found.Data != nil {
		t.Fatalf("/jobs/find answered with private slug %q", found.Data.PublicSlug)
	}

	const path = "/api/v1/jobs/" + slug + "/vote"
	if code, _ := do(t, http.MethodPost, path, `{"vote":"up"}`, other); code != http.StatusNotFound {
		t.Fatalf("non-owner vote: want 404, got %d", code)
	}
	if code, _ := do(t, http.MethodDelete, path, "", other); code != http.StatusNotFound {
		t.Fatalf("non-owner clear: want 404, got %d", code)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_jobs WHERE job_id = $1`, jobID).Scan(&rows); err != nil {
		t.Fatalf("count user_jobs: %v", err)
	}
	if rows != 0 {
		t.Fatalf("non-owner vote left %d user_jobs rows", rows)
	}

	// The owner still votes on their own posting.
	if code, _ := do(t, http.MethodPost, path, `{"vote":"up"}`, owner); code != http.StatusOK {
		t.Fatalf("owner vote: want 200, got %d", code)
	}

	// A row written before the fix must not surface the card or count on a tab.
	if _, err := pool.Exec(ctx, `INSERT INTO user_jobs (user_id, job_id, vote) VALUES ($1, $2, 1)`, other, jobID); err != nil {
		t.Fatalf("seed leaked row: %v", err)
	}
	list, err := queries.ListUserJobs(ctx, db.ListUserJobsParams{UserID: other, Filter: "all", Limit: 50})
	if err != nil {
		t.Fatalf("ListUserJobs: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListUserJobs listed another user's private job: %+v", list)
	}
	counts, err := queries.CountUserJobs(ctx, other)
	if err != nil {
		t.Fatalf("CountUserJobs: %v", err)
	}
	if counts.All != 0 {
		t.Fatalf("CountUserJobs all = %d, want 0", counts.All)
	}
	ownList, err := queries.ListUserJobs(ctx, db.ListUserJobsParams{UserID: owner, Filter: "all", Limit: 50})
	if err != nil {
		t.Fatalf("ListUserJobs owner: %v", err)
	}
	if len(ownList) != 1 {
		t.Fatalf("owner's own private job: want 1 row, got %d", len(ownList))
	}
}
