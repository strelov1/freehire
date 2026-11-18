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
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Prefill parses a recognized job URL into draft field values without persisting
// anything — no job, no submission, no credit reward.
func TestSubmissionsPrefillEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('prefill-user@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	userCookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)

	importer := linkimport.New(pool, queries, nil, prefillPageClient{}, nil, nil)
	mod := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	sh := newSubmissionHandlers(queries, pool, mod, importer, sources.PostingURLResolver{})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/submissions/prefill", keyAuth, submissionsContract(sh))

	req := func(cookie, body string) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/submissions/prefill", bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}

	t.Run("recognized URL resolves without persisting", func(t *testing.T) {
		resp, err := app.Test(req(userCookie, `{"url":"`+prefillPageURL+`"}`))
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("prefill status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var out struct {
			Data struct {
				Title   string `json:"title"`
				Company string `json:"company"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Title != "Staff Java Backend Developer" || out.Data.Company != "Mindera" {
			t.Errorf("parsed = %q/%q, want the page's title/company", out.Data.Title, out.Data.Company)
		}
		var rows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&rows); err != nil {
			t.Fatalf("count jobs: %v", err)
		}
		if rows != 0 {
			t.Errorf("catalog holds %d postings after prefill, want none", rows)
		}
		var subRows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_submissions`).Scan(&subRows); err != nil {
			t.Fatalf("count submissions: %v", err)
		}
		if subRows != 0 {
			t.Errorf("queue holds %d submissions after prefill, want none", subRows)
		}
	})

	t.Run("unrecognized URL degrades to empty fields, not an error", func(t *testing.T) {
		resp, err := app.Test(req(userCookie, `{"url":"https://example.test/not-a-job"}`))
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("prefill status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var out struct {
			Data struct {
				Title string `json:"title"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Title != "" {
			t.Errorf("title = %q, want empty for an unrecognized page", out.Data.Title)
		}
	})

	t.Run("a URL the catalogue already carries answers with its slug and nothing else", func(t *testing.T) {
		const knownURL = "https://acme.example/jobs/known"
		job, _, err := mod.Create(ctx, userID, moderation.CreateInput{
			URL: knownURL, Title: "Senior Go Developer", Company: "Acme", Description: "We use Golang.",
		})
		if err != nil {
			t.Fatalf("seed job: %v", err)
		}
		resp, err := app.Test(req(userCookie, `{"url":"`+knownURL+`/"}`))
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("prefill status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var out struct {
			Data struct {
				Title           string `json:"title"`
				ExistingJobSlug string `json:"existing_job_slug"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.ExistingJobSlug != job.Fields().PublicSlug || out.Data.Title != "" {
			t.Errorf("prefill = slug %q title %q, want slug %q and no fields", out.Data.ExistingJobSlug, out.Data.Title, job.Fields().PublicSlug)
		}
	})

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		resp, err := app.Test(req("", `{"url":"`+prefillPageURL+`"}`))
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})
}
