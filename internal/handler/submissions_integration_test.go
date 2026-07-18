//go:build integration

// End-to-end HTTP tests for the public job-submission endpoints against a real Postgres:
// submit (201/409/401/400), the role-gated review queue (200/403), approve (mints a live
// job under the submitter, 200/409), reject (records the reason, no job), "my submissions"
// scoping, and role on /auth/me.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/submission"
)

func TestSubmissionsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var modID, user1ID, user2ID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ('mod@example.test', 'moderator') RETURNING id`).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('u1@example.test') RETURNING id`).Scan(&user1ID); err != nil {
		t.Fatalf("seed user1: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('u2@example.test') RETURNING id`).Scan(&user2ID); err != nil {
		t.Fatalf("seed user2: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID)
	user1Cookie, _ := iss.Issue(user1ID)
	user2Cookie, _ := iss.Issue(user2ID)
	queries := db.New(pool)
	mod := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	h := &API{
		pool:       pool,
		queries:    queries,
		issuer:     iss,
		moderation: mod,
		submission: submission.New(submission.NewQueriesRepository(queries), mod),
		accounts:   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/submissions", keyAuth, h.CreateSubmission)
	app.Get("/api/v1/me/submissions", keyAuth, h.ListMySubmissions)
	app.Get("/api/v1/submissions", keyAuth, requireMod, h.ListPendingSubmissions)
	app.Post("/api/v1/submissions/:id/approve", keyAuth, requireMod, h.ApproveSubmission)
	app.Post("/api/v1/submissions/:id/reject", keyAuth, requireMod, h.RejectSubmission)
	app.Get("/api/v1/auth/me", keyAuth, h.Me)

	req := func(method, path, cookie, body string) *http.Request {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}
	decodeID := func(t *testing.T, resp *http.Response) int64 {
		t.Helper()
		var out struct {
			Data struct {
				ID     int64  `json:"id"`
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Data.ID
	}

	const url1 = "https://acme.example/jobs/1"
	const body1 = `{"url":"` + url1 + `","title":"Senior Go Developer","company":"Acme","location":"Germany","remote":true,"description":"We use Golang."}`
	var sub1ID int64

	t.Run("user submits a vacancy (201, pending, owned)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", user1Cookie, body1))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		sub1ID = decodeID(t, resp)

		var status string
		var submittedBy int64
		if err := pool.QueryRow(ctx,
			"SELECT status, submitted_by FROM job_submissions WHERE id = $1", sub1ID).Scan(&status, &submittedBy); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "pending" || submittedBy != user1ID {
			t.Errorf("stored status/owner = %q/%d, want pending/%d", status, submittedBy, user1ID)
		}
	})

	t.Run("duplicate pending URL is a 409", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", user1Cookie, body1))
		if err != nil {
			t.Fatalf("dup submit: %v", err)
		}
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("unauthenticated submit is a 401", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", "", body1))
		if err != nil {
			t.Fatalf("anon submit: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("missing required field is a 400", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", user1Cookie, `{"title":"X","company":"Y"}`))
		if err != nil {
			t.Fatalf("bad submit: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("non-moderator is forbidden from the queue (403)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/submissions", user1Cookie, ""))
		if err != nil {
			t.Fatalf("queue as user: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("moderator sees the pending queue with submitter email", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/submissions", modCookie, ""))
		if err != nil {
			t.Fatalf("queue as mod: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			Data []struct {
				ID             int64  `json:"id"`
				SubmitterEmail string `json:"submitter_email"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var found bool
		for _, s := range out.Data {
			if s.ID == sub1ID {
				found = true
				if s.SubmitterEmail != "u1@example.test" {
					t.Errorf("submitter_email = %q, want u1@example.test", s.SubmitterEmail)
				}
			}
		}
		if !found {
			t.Errorf("pending submission %d not in the queue", sub1ID)
		}
	})

	t.Run("moderator approves: mints a live job under the submitter (200)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+itoa(sub1ID)+"/approve", modCookie, ""))
		if err != nil {
			t.Fatalf("approve: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}

		var status string
		var reviewedBy int64
		var jobID int64
		if err := pool.QueryRow(ctx,
			"SELECT status, reviewed_by, job_id FROM job_submissions WHERE id = $1", sub1ID).Scan(&status, &reviewedBy, &jobID); err != nil {
			t.Fatalf("read submission: %v", err)
		}
		if status != "approved" || reviewedBy != modID {
			t.Errorf("submission status/reviewer = %q/%d, want approved/%d", status, reviewedBy, modID)
		}

		var source string
		var createdBy int64
		if err := pool.QueryRow(ctx,
			"SELECT source, created_by FROM jobs WHERE id = $1", jobID).Scan(&source, &createdBy); err != nil {
			t.Fatalf("read minted job: %v", err)
		}
		if source != "manual" {
			t.Errorf("minted source = %q, want manual", source)
		}
		if createdBy != user1ID {
			t.Errorf("minted created_by = %d, want the submitter %d", createdBy, user1ID)
		}
	})

	t.Run("re-approving a decided submission is a 409", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+itoa(sub1ID)+"/approve", modCookie, ""))
		if err != nil {
			t.Fatalf("re-approve: %v", err)
		}
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("moderator rejects another submission with a reason (no job)", func(t *testing.T) {
		const url2 = "https://acme.example/jobs/2"
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", user2Cookie,
			`{"url":"`+url2+`","title":"Frontend Dev","company":"Beta"}`))
		if err != nil {
			t.Fatalf("user2 submit: %v", err)
		}
		sub2ID := decodeID(t, resp)

		rresp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+itoa(sub2ID)+"/reject", modCookie, `{"reason":"duplicate"}`))
		if err != nil {
			t.Fatalf("reject: %v", err)
		}
		if rresp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", rresp.StatusCode)
		}
		var status, reason string
		if err := pool.QueryRow(ctx,
			"SELECT status, review_reason FROM job_submissions WHERE id = $1", sub2ID).Scan(&status, &reason); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "rejected" || reason != "duplicate" {
			t.Errorf("status/reason = %q/%q, want rejected/duplicate", status, reason)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE url = $1", url2).Scan(&n); err != nil {
			t.Fatalf("count jobs: %v", err)
		}
		if n != 0 {
			t.Errorf("rejected submission minted %d jobs, want 0", n)
		}
	})

	t.Run("my submissions are scoped to the caller", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/me/submissions", user1Cookie, ""))
		if err != nil {
			t.Fatalf("my submissions: %v", err)
		}
		var out struct {
			Data []struct {
				ID      int64  `json:"id"`
				Status  string `json:"status"`
				JobSlug string `json:"job_slug"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var sawOwn bool
		for _, s := range out.Data {
			if s.ID == sub1ID {
				sawOwn = true
				// sub1 was approved earlier, so it links to the minted live vacancy.
				if s.Status != "approved" || s.JobSlug == "" {
					t.Errorf("approved submission missing job_slug: status=%q job_slug=%q", s.Status, s.JobSlug)
				}
			}
			// user1 must never see user2's submission (url2).
			var owner int64
			if err := pool.QueryRow(ctx, "SELECT submitted_by FROM job_submissions WHERE id = $1", s.ID).Scan(&owner); err == nil && owner != user1ID {
				t.Errorf("my-submissions leaked submission %d owned by %d", s.ID, owner)
			}
		}
		if !sawOwn {
			t.Errorf("own submission %d missing from my-submissions", sub1ID)
		}
	})

	t.Run("auth/me carries the role", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/auth/me", modCookie, ""))
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		var out struct {
			Data struct {
				Role string `json:"role"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Role != "moderator" {
			t.Errorf("role = %q, want moderator", out.Data.Role)
		}
	})
}

// itoa renders an int64 id for a URL path without dragging strconv into the test body.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// TestSubmissionStructuredFacetsEndToEnd covers the enriched submit path: a submission
// carrying explicit structured facets and a salary is stored with them, and on approval
// the minted job carries the facets as overrides (region/city/work-mode/skills) plus an
// authoritative manual salary seeded into the enrichment payload.
func TestSubmissionStructuredFacetsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var modID, userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ('mod2@example.test', 'moderator') RETURNING id`).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('recruiter@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID)
	userCookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	mod := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	h := &API{
		pool:       pool,
		queries:    queries,
		issuer:     iss,
		moderation: mod,
		submission: submission.New(submission.NewQueriesRepository(queries), mod),
		accounts:   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/submissions", keyAuth, h.CreateSubmission)
	app.Post("/api/v1/submissions/:id/approve", keyAuth, requireMod, h.ApproveSubmission)

	req := func(method, path, cookie, body string) *http.Request {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}

	const url = "https://acme.example/jobs/structured"
	// Location "Germany" would derive the eu region; the explicit facets must win.
	body := `{"url":"` + url + `","title":"Senior Go Developer","company":"Acme",` +
		`"location":"Germany","description":"We use Golang.",` +
		`"skills":["kubernetes"],"regions":["north_america"],"cities":["Austin"],"work_mode":"hybrid",` +
		`"salary_min":90000,"salary_max":120000,"salary_currency":"EUR","salary_period":"year"}`

	resp, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions", userCookie, body))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("submit status = %d, want 201 (body %s)", resp.StatusCode, b)
	}
	var subOut struct {
		Data struct {
			ID        int64    `json:"id"`
			Regions   []string `json:"regions"`
			WorkMode  string   `json:"work_mode"`
			SalaryMin *int     `json:"salary_min"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&subOut); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	// The response echoes the structured facets back to the submitter.
	if len(subOut.Data.Regions) != 1 || subOut.Data.Regions[0] != "north_america" || subOut.Data.WorkMode != "hybrid" {
		t.Errorf("echoed facets = %v/%q, want [north_america]/hybrid", subOut.Data.Regions, subOut.Data.WorkMode)
	}
	if subOut.Data.SalaryMin == nil || *subOut.Data.SalaryMin != 90000 {
		t.Errorf("echoed salary_min = %v, want 90000", subOut.Data.SalaryMin)
	}

	// Approve → mint.
	approve, err := app.Test(req(fiber.MethodPost, "/api/v1/submissions/"+strconv.FormatInt(subOut.Data.ID, 10)+"/approve", modCookie, ""))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approve.StatusCode != fiber.StatusOK {
		b, _ := io.ReadAll(approve.Body)
		t.Fatalf("approve status = %d, want 200 (body %s)", approve.StatusCode, b)
	}

	// The minted job carries the explicit facets and the seeded manual salary.
	var regions, cities, skills []string
	var workMode string
	var manualMin int
	var enrichMin int
	if err := pool.QueryRow(ctx,
		`SELECT regions, cities, work_mode, skills, salary_min_manual, (enrichment->>'salary_min')::int
		 FROM jobs WHERE source = 'manual' AND external_id = $1`, url).
		Scan(&regions, &cities, &workMode, &skills, &manualMin, &enrichMin); err != nil {
		t.Fatalf("read minted job: %v", err)
	}
	if len(regions) != 1 || regions[0] != "north_america" {
		t.Errorf("minted regions = %v, want [north_america] (explicit wins over derived eu)", regions)
	}
	if len(cities) != 1 || cities[0] != "Austin" {
		t.Errorf("minted cities = %v, want [Austin]", cities)
	}
	if workMode != "hybrid" {
		t.Errorf("minted work_mode = %q, want hybrid", workMode)
	}
	if !slices.Contains(skills, "kubernetes") || !slices.Contains(skills, "go") {
		t.Errorf("minted skills = %v, want to contain kubernetes and go", skills)
	}
	if manualMin != 90000 {
		t.Errorf("minted salary_min_manual = %d, want 90000", manualMin)
	}
	if enrichMin != 90000 {
		t.Errorf("seeded enrichment salary_min = %d, want 90000", enrichMin)
	}
}
