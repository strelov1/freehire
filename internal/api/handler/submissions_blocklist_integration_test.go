//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/submission"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestSubmissionDomainBlocklistEndToEnd covers the submission-domain blocklist: a
// moderator rejecting one submission with block_domain=true blocks the host, bulk-rejects
// every other pending submission on that host, and a later submission of the same host is
// then refused with no row written.
func TestSubmissionDomainBlocklistEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var modID, userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ('mod3@example.test', 'moderator') RETURNING id`).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('u3@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID, testTokenVersion)
	userCookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)
	mod := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	sh := &submissionHandlers{submission: submission.New(submission.NewQueriesRepository(queries, pool), mod), queries: queries}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/submissions", keyAuth, submissionsContract(sh))
	app.Post("/api/v1/submissions/:id/reject", keyAuth, requireMod, submissionsContract(sh))

	req := func(method, path, cookie, body string) *http.Request {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequestWithContext(ctx, method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie}) //nolint:gosec // a request cookie: the Set-Cookie attributes do not apply
		}
		return r
	}
	decodeID := func(t *testing.T, resp *http.Response) int64 {
		t.Helper()
		var out struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Data.ID
	}
	submit := func(t *testing.T, url string) *http.Response {
		t.Helper()
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", userCookie,
			`{"url":"`+url+`","title":"Some Role","company":"Gridnaut Recruiting"}`))
		if err != nil {
			t.Fatalf("submit %s: %v", url, err)
		}
		return resp
	}

	spam1 := submit(t, "https://gridnaut.site/jobs/role-one/")
	defer spam1.Body.Close()
	if spam1.StatusCode != fiber.StatusCreated {
		b, _ := io.ReadAll(spam1.Body)
		t.Fatalf("seed spam1 status = %d, want 201 (body %s)", spam1.StatusCode, b)
	}
	spam1ID := decodeID(t, spam1)

	spam2 := submit(t, "https://gridnaut.site/jobs/role-two/")
	defer spam2.Body.Close()
	if spam2.StatusCode != fiber.StatusCreated {
		b, _ := io.ReadAll(spam2.Body)
		t.Fatalf("seed spam2 status = %d, want 201 (body %s)", spam2.StatusCode, b)
	}
	spam2ID := decodeID(t, spam2)

	t.Run("rejecting with block_domain blocks the host and bulk-rejects siblings", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+itoa(spam1ID)+"/reject", modCookie,
			`{"reason":"referral spam","block_domain":true}`))
		if err != nil {
			t.Fatalf("reject with block_domain: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}

		var status1, reason1 string
		if err := pool.QueryRow(ctx,
			"SELECT status, review_reason FROM job_submissions WHERE id = $1", spam1ID).Scan(&status1, &reason1); err != nil {
			t.Fatalf("read spam1: %v", err)
		}
		if status1 != "rejected" || reason1 != "referral spam" {
			t.Errorf("spam1 status/reason = %q/%q, want rejected/referral spam", status1, reason1)
		}

		var status2 string
		if err := pool.QueryRow(ctx,
			"SELECT status FROM job_submissions WHERE id = $1", spam2ID).Scan(&status2); err != nil {
			t.Fatalf("read spam2: %v", err)
		}
		if status2 != "rejected" {
			t.Errorf("sibling spam2 status = %q, want rejected (bulk-rejected by block_domain)", status2)
		}

		var blockedCount int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM submission_domain_blocklist WHERE host = 'gridnaut.site'").Scan(&blockedCount); err != nil {
			t.Fatalf("count blocklist: %v", err)
		}
		if blockedCount != 1 {
			t.Errorf("gridnaut.site blocklist rows = %d, want 1", blockedCount)
		}
	})

	t.Run("a later submission of the blocked host is refused with no row written", func(t *testing.T) {
		resp := submit(t, "https://gridnaut.site/jobs/role-three/")
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 403 (body %s)", resp.StatusCode, b)
		}
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM job_submissions WHERE url = $1", "https://gridnaut.site/jobs/role-three/").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("blocked submission wrote %d rows, want 0", n)
		}
	})

	t.Run("a www.-prefixed variant of the blocked host is also refused", func(t *testing.T) {
		resp := submit(t, "https://www.gridnaut.site/jobs/role-four/")
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 403 (body %s)", resp.StatusCode, b)
		}
	})

	t.Run("blocking an already-blocked host while rejecting is a no-op for the blocklist", func(t *testing.T) {
		// A row can be pending on an already-blocked host if it existed before the block
		// (Submit refuses new ones, so this simulates that ordering directly).
		var preExistingID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO job_submissions (submitted_by, url, title, company, status)
			 VALUES ($1, 'https://gridnaut.site/jobs/pre-existing/', 'Pre-existing Role', 'Gridnaut Recruiting', 'pending')
			 RETURNING id`, userID).Scan(&preExistingID); err != nil {
			t.Fatalf("seed pre-existing pending row on the blocked host: %v", err)
		}

		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+itoa(preExistingID)+"/reject", modCookie,
			`{"reason":"referral spam","block_domain":true}`))
		if err != nil {
			t.Fatalf("reject pre-existing: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}

		var status string
		if err := pool.QueryRow(ctx,
			"SELECT status FROM job_submissions WHERE id = $1", preExistingID).Scan(&status); err != nil {
			t.Fatalf("read pre-existing: %v", err)
		}
		if status != "rejected" {
			t.Errorf("pre-existing status = %q, want rejected", status)
		}

		var blockedCount int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM submission_domain_blocklist WHERE host = 'gridnaut.site'").Scan(&blockedCount); err != nil {
			t.Fatalf("count blocklist: %v", err)
		}
		if blockedCount != 1 {
			t.Errorf("gridnaut.site blocklist rows = %d, want 1 (still no duplicate)", blockedCount)
		}
	})
}
