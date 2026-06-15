//go:build integration

// End-to-end HTTP tests for the job-report endpoints against a real Postgres: file
// (201/409/401/400/404), the role-gated review queue (200/403), resolve (optionally
// soft-closing the job, 200/409), and dismiss (records the reason, leaves the job open).
// Run with: go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/report"
)

func TestReportsEndToEnd(t *testing.T) {
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

	const slug1, slug2 = "go-dev-acme-aaaa1111", "fe-dev-beta-bbbb2222"
	var job1ID, job2ID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'report-1', 'http://example.test/1', 'Go Dev', $1) RETURNING id`, slug1).Scan(&job1ID); err != nil {
		t.Fatalf("seed job1: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'report-2', 'http://example.test/2', 'Frontend Dev', $1) RETURNING id`, slug2).Scan(&job2ID); err != nil {
		t.Fatalf("seed job2: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID)
	user1Cookie, _ := iss.Issue(user1ID)
	user2Cookie, _ := iss.Issue(user2ID)
	queries := db.New(pool)
	reportRepo := report.NewQueriesRepository(queries)
	h := &API{
		pool:    pool,
		queries: queries,
		issuer:  iss,
		report:  report.New(reportRepo, reportRepo),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/jobs/:slug/reports", keyAuth, h.CreateReport)
	app.Get("/api/v1/reports", keyAuth, requireMod, h.ListPendingReports)
	app.Post("/api/v1/reports/:id/resolve", keyAuth, requireMod, h.ResolveReport)
	app.Post("/api/v1/reports/:id/dismiss", keyAuth, requireMod, h.DismissReport)

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
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Data.ID
	}

	const goodReport = `{"reason":"fraud","details":"asks for an upfront payment","contact_telegram":"@reporter"}`
	var report1ID int64

	t.Run("user files a report (201, pending, owned)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug1+"/reports", user1Cookie, goodReport))
		if err != nil {
			t.Fatalf("file: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		report1ID = decodeID(t, resp)

		var status string
		var reportedBy, jobID int64
		if err := pool.QueryRow(ctx,
			"SELECT status, reported_by, job_id FROM job_reports WHERE id = $1", report1ID).Scan(&status, &reportedBy, &jobID); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "pending" || reportedBy != user1ID || jobID != job1ID {
			t.Errorf("stored = %q/%d/%d, want pending/%d/%d", status, reportedBy, jobID, user1ID, job1ID)
		}
	})

	t.Run("duplicate open report by the same user is a 409", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug1+"/reports", user1Cookie, goodReport))
		if err != nil {
			t.Fatalf("dup file: %v", err)
		}
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("a different user reporting the same job is allowed (201)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug1+"/reports", user2Cookie, goodReport))
		if err != nil {
			t.Fatalf("u2 file: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			t.Errorf("status = %d, want 201", resp.StatusCode)
		}
	})

	t.Run("unauthenticated report is a 401", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug1+"/reports", "", goodReport))
		if err != nil {
			t.Fatalf("anon file: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("invalid reason and blank details are 400", func(t *testing.T) {
		for _, body := range []string{
			`{"reason":"because","details":"x"}`,
			`{"reason":"spam","details":"   "}`,
		} {
			resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug2+"/reports", user1Cookie, body))
			if err != nil {
				t.Fatalf("bad file: %v", err)
			}
			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("body %s: status = %d, want 400", body, resp.StatusCode)
			}
		}
	})

	t.Run("unknown job slug is a 404", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/does-not-exist/reports", user1Cookie, goodReport))
		if err != nil {
			t.Fatalf("missing slug: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("non-moderator is forbidden from the queue (403)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/reports", user1Cookie, ""))
		if err != nil {
			t.Fatalf("queue as user: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("moderator sees the queue with reporter email and job fields", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodGet, "/api/v1/reports", modCookie, ""))
		if err != nil {
			t.Fatalf("queue as mod: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			Data []struct {
				ID            int64  `json:"id"`
				ReporterEmail string `json:"reporter_email"`
				JobSlug       string `json:"job_slug"`
				JobTitle      string `json:"job_title"`
				ReportedBy    *int64 `json:"reported_by"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var found bool
		for _, r := range out.Data {
			if r.ReportedBy != nil {
				t.Error("reported_by must not be exposed on the wire")
			}
			if r.ID == report1ID {
				found = true
				if r.ReporterEmail != "u1@example.test" || r.JobSlug != slug1 || r.JobTitle != "Go Dev" {
					t.Errorf("queue row = email %q slug %q title %q", r.ReporterEmail, r.JobSlug, r.JobTitle)
				}
			}
		}
		if !found {
			t.Errorf("pending report %d not in the queue", report1ID)
		}
	})

	t.Run("moderator resolves with close: the job is soft-closed (200)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/reports/"+itoa(report1ID)+"/resolve", modCookie, `{"close_job":true}`))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var status string
		var reviewedBy int64
		if err := pool.QueryRow(ctx,
			"SELECT status, reviewed_by FROM job_reports WHERE id = $1", report1ID).Scan(&status, &reviewedBy); err != nil {
			t.Fatalf("read report: %v", err)
		}
		if status != "resolved" || reviewedBy != modID {
			t.Errorf("report status/reviewer = %q/%d, want resolved/%d", status, reviewedBy, modID)
		}
		var closed *time.Time
		if err := pool.QueryRow(ctx, "SELECT closed_at FROM jobs WHERE id = $1", job1ID).Scan(&closed); err != nil {
			t.Fatalf("read job: %v", err)
		}
		if closed == nil {
			t.Error("job1 should be soft-closed after resolve-with-close")
		}
	})

	t.Run("re-resolving a decided report is a 409", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/reports/"+itoa(report1ID)+"/resolve", modCookie, `{"close_job":false}`))
		if err != nil {
			t.Fatalf("re-resolve: %v", err)
		}
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("moderator dismisses a report with a reason (job unchanged)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs/"+slug2+"/reports", user1Cookie, `{"reason":"not_relevant","details":"looks fine to me"}`))
		if err != nil {
			t.Fatalf("file on job2: %v", err)
		}
		r2ID := decodeID(t, resp)

		dresp, err := app.Test(req(fiber.MethodPost, "/api/v1/reports/"+itoa(r2ID)+"/dismiss", modCookie, `{"reason":"not a real issue"}`))
		if err != nil {
			t.Fatalf("dismiss: %v", err)
		}
		if dresp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", dresp.StatusCode)
		}
		var status, reviewReason string
		if err := pool.QueryRow(ctx,
			"SELECT status, review_reason FROM job_reports WHERE id = $1", r2ID).Scan(&status, &reviewReason); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "dismissed" || reviewReason != "not a real issue" {
			t.Errorf("status/reason = %q/%q, want dismissed/'not a real issue'", status, reviewReason)
		}
		var closed *time.Time
		if err := pool.QueryRow(ctx, "SELECT closed_at FROM jobs WHERE id = $1", job2ID).Scan(&closed); err != nil {
			t.Fatalf("read job2: %v", err)
		}
		if closed != nil {
			t.Error("dismiss must not close the job")
		}
	})
}
