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
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The employer prefill differs from the public one in a single way: a vacancy the catalogue
// already carries is not refused but read back from its stored row.
func TestEmployerPrefillEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seed := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		return id
	}
	employerID, plainID := seed("hr@acme.test"), seed("plain@example.test")
	if _, err := pool.Exec(ctx, `
		INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status, verified_at, email_confirmed_at)
		VALUES ($1, 'acme', 'Acme', 'hr@acme.test', 'active', now(), now())`, employerID); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	accountsSvc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	employerRepo := employer.NewQueriesRepository(queries, pool)
	mod := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	employerSvc := employer.New(employerRepo, accountsSvc, nil, employerRepo, mod)
	importer := linkimport.New(pool, queries, nil, prefillPageClient{}, nil, nil)
	h := newEmployerHandlers(queries, employerSvc)
	h.prefiller = &jobPrefiller{queries: queries, importer: importer, postings: sources.PostingURLResolver{}}

	iss := auth.NewIssuer("test-secret", time.Hour)
	employerCookie, _ := iss.Issue(employerID, testTokenVersion)
	plainCookie, _ := iss.Issue(plainID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)}, newContract(contractHandlers{employerHandlers: h}))

	prefill := func(t *testing.T, cookie, url string) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/employer/jobs/prefill", bytes.NewReader([]byte(`{"url":"`+url+`"}`)))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie}) //nolint:gosec // test request
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var out struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(b, &out)
		return resp.StatusCode, out.Data
	}

	t.Run("a vacancy already in the catalogue comes back from its stored row", func(t *testing.T) {
		const knownURL = "https://acme.example/jobs/known"
		job, _, err := mod.Create(ctx, employerID, moderation.CreateInput{
			URL: knownURL, Title: "Senior Go Developer", Company: "Acme", Location: "Berlin, Germany",
			Description: "We use Golang.", WorkMode: "hybrid", Seniority: "senior", EmploymentType: "full_time",
		})
		if err != nil {
			t.Fatalf("seed job: %v", err)
		}
		status, data := prefill(t, employerCookie, knownURL+"/")
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		for key, want := range map[string]string{
			"title": "Senior Go Developer", "location": "Berlin, Germany", "work_mode": "hybrid",
			"seniority": "senior", "employment_type": "full_time", "existing_job_slug": job.Fields().PublicSlug,
		} {
			if data[key] != want {
				t.Errorf("%s = %v, want %q", key, data[key], want)
			}
		}
	})

	t.Run("an unknown URL is parsed from the page, nothing is persisted", func(t *testing.T) {
		countJobs := func() (n int) {
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&n); err != nil {
				t.Fatalf("count jobs: %v", err)
			}
			return n
		}
		before := countJobs()
		status, data := prefill(t, employerCookie, prefillPageURL)
		if status != fiber.StatusOK || data["title"] != "Staff Java Backend Developer" || data["existing_job_slug"] != nil {
			t.Fatalf("status %d data %v, want the page's title and no slug", status, data)
		}
		if after := countJobs(); after != before {
			t.Errorf("jobs %d -> %d, prefill must not write", before, after)
		}
	})

	t.Run("an account without an active employer claim is refused", func(t *testing.T) {
		if status, _ := prefill(t, plainCookie, prefillPageURL); status != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404 (no employer account)", status)
		}
	})
}
