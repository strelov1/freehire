//go:build integration

// End-to-end HTTP tests for the ghost-report evidence channel against a real Postgres:
// file (201), the three refusals that are enforced in SQL rather than by a check
// (403 unverified, 409 closed, 409 duplicate), the daily cap (429), and retraction
// (204, then 404). Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/ghostreport"
)

func TestGhostReportsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var verifiedID, unverifiedID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('gv@example.test', true) RETURNING id`).Scan(&verifiedID); err != nil {
		t.Fatalf("seed verified user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, email_verified) VALUES ('gu@example.test', false) RETURNING id`).Scan(&unverifiedID); err != nil {
		t.Fatalf("seed unverified user: %v", err)
	}

	const openSlug, closedSlug = "ghost-open-acme-cccc3333", "ghost-closed-beta-dddd4444"
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'ghost-open', 'http://example.test/o', 'Go Dev', $1)`, openSlug); err != nil {
		t.Fatalf("seed open job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, closed_at)
		 VALUES ('test', 'ghost-closed', 'http://example.test/c', 'FE Dev', $1, now())`, closedSlug); err != nil {
		t.Fatalf("seed closed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	verifiedCookie, _ := iss.Issue(verifiedID, testTokenVersion)
	unverifiedCookie, _ := iss.Issue(unverifiedID, testTokenVersion)
	queries := db.New(pool)
	h := &ghostReportHandlers{
		reports: ghostreport.New(ghostreport.NewQueriesRepository(queries)),
		queries: queries,
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/:slug/ghost-report", keyAuth, h.CreateGhostReport)
	app.Delete("/api/v1/jobs/:slug/ghost-report", keyAuth, h.RetractGhostReport)

	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	body := fmt.Sprintf(`{"applied_on":%q}`, thirtyDaysAgo)

	req := func(method, path, cookie, b string) *http.Request {
		var r *http.Request
		if b != "" {
			r = httptest.NewRequest(method, path, bytes.NewReader([]byte(b)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}

	post := func(slug, cookie, b string) *http.Response {
		t.Helper()
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug+"/ghost-report", cookie, b))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		return resp
	}
	del := func(slug, cookie string) *http.Response {
		t.Helper()
		resp, err := app.Test(req(fiber.MethodDelete, "/api/v1/jobs/"+slug+"/ghost-report", cookie, ""))
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		return resp
	}

	t.Run("a verified user files a claim", func(t *testing.T) {
		if got := post(openSlug, verifiedCookie, body).StatusCode; got != fiber.StatusCreated {
			t.Fatalf("status = %d, want 201", got)
		}
		var appliedOn time.Time
		if err := pool.QueryRow(ctx,
			`SELECT applied_on FROM ghost_reports WHERE user_id = $1`, verifiedID).Scan(&appliedOn); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if appliedOn.Format("2006-01-02") != thirtyDaysAgo {
			t.Errorf("applied_on = %s, want %s", appliedOn.Format("2006-01-02"), thirtyDaysAgo)
		}
	})

	t.Run("a second claim on the same job is a 409", func(t *testing.T) {
		if got := post(openSlug, verifiedCookie, body).StatusCode; got != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", got)
		}
	})

	// The gate is the INSERT's own SELECT: an unproven address writes no row, so there
	// is no service check that a later caller could route around.
	t.Run("an unverified account is a 403", func(t *testing.T) {
		if got := post(openSlug, unverifiedCookie, body).StatusCode; got != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", got)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM ghost_reports WHERE user_id = $1`, unverifiedID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("rows for the unverified account = %d, want 0", n)
		}
	})

	t.Run("a closed job is a 409", func(t *testing.T) {
		if got := post(closedSlug, verifiedCookie, body).StatusCode; got != fiber.StatusConflict {
			t.Errorf("status = %d, want 409 — nothing left to warn anyone about", got)
		}
	})

	t.Run("a future apply date is a 400", func(t *testing.T) {
		future := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
		if got := post(openSlug, verifiedCookie, fmt.Sprintf(`{"applied_on":%q}`, future)).StatusCode; got != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("an unparseable date is a 400", func(t *testing.T) {
		if got := post(openSlug, verifiedCookie, `{"applied_on":"last tuesday"}`).StatusCode; got != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("an unknown slug is a 404", func(t *testing.T) {
		if got := post("does-not-exist", verifiedCookie, body).StatusCode; got != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", got)
		}
	})

	t.Run("an unauthenticated request is a 401", func(t *testing.T) {
		if got := post(openSlug, "", body).StatusCode; got != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	// 204, not 200: SendStatus(200) writes the body "OK" and breaks a JSON client on a
	// call that succeeded.
	t.Run("retraction answers 204 with no body", func(t *testing.T) {
		resp := del(openSlug, verifiedCookie)
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}
		if resp.ContentLength > 0 {
			t.Errorf("content length = %d, want no body", resp.ContentLength)
		}
		var retracted *time.Time
		if err := pool.QueryRow(ctx,
			`SELECT retracted_at FROM ghost_reports WHERE user_id = $1`, verifiedID).Scan(&retracted); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if retracted == nil {
			t.Error("the claim was not marked retracted")
		}
	})

	t.Run("retracting twice is a 404", func(t *testing.T) {
		if got := del(openSlug, verifiedCookie).StatusCode; got != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", got)
		}
	})

	// Retracting by mistake must not lock somebody out of their own claim, and reviving
	// cannot inflate anything: the row, and so the person, is still counted once.
	t.Run("filing again after a retraction revives the claim", func(t *testing.T) {
		if got := post(openSlug, verifiedCookie, body).StatusCode; got != fiber.StatusCreated {
			t.Fatalf("status = %d, want 201", got)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM ghost_reports WHERE user_id = $1`, verifiedID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("rows = %d, want 1 — reviving must not add a second claim", n)
		}
	})

	// The cap counts retracted rows too: filing and withdrawing in a loop is exactly the
	// pattern it exists to bound, so forgiving that would leave it trivially bypassable.
	t.Run("past the daily cap is a 429", func(t *testing.T) {
		var already int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM ghost_reports WHERE user_id = $1 AND created_at >= now() - interval '1 day'`,
			verifiedID).Scan(&already); err != nil {
			t.Fatalf("count filed today: %v", err)
		}
		remaining := ghostreport.DailyCap - already
		if remaining < 1 {
			t.Fatalf("the cap is already full before this subtest (filed %d)", already)
		}

		slugs := seedGhostJobs(t, pool, remaining+1)
		for i, slug := range slugs[:remaining] {
			if got := post(slug, verifiedCookie, body).StatusCode; got != fiber.StatusCreated {
				t.Fatalf("filling the cap at %d of %d: status = %d, want 201", i+1, remaining, got)
			}
		}
		if got := post(slugs[remaining], verifiedCookie, body).StatusCode; got != fiber.StatusTooManyRequests {
			t.Errorf("status = %d, want 429 at exactly the cap", got)
		}
	})
}

// seedGhostJobs inserts n open jobs and returns their slugs.
func seedGhostJobs(t *testing.T, pool *pgxpool.Pool, n int) []string {
	t.Helper()
	slugs := make([]string, n)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("ghost-cap-job-%04d", i)
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test/cap', 'Cap Dev', $2)`,
			"ghost-cap-"+slugs[i], slugs[i]); err != nil {
			t.Fatalf("seed cap job %d: %v", i, err)
		}
	}
	return slugs
}
