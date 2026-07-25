//go:build integration

// End-to-end HTTP tests for the moderator-authored job endpoints against a real
// Postgres: the full chain (route → cookie/key auth → role gate → handler → service →
// DB). Create is moderator-only (201 / 403 / 401 / 400), the row lands under the manual
// source with authorship recorded, and edit is partial + manual-scoped (200 / 404).
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
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/moderation"
)

func TestModeratorJobsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var modID, userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ('mod@example.test', 'moderator') RETURNING id`).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('user@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// A non-manual job, to prove the edit path cannot touch it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'gh:1', 'http://ats.test/1', 'ATS Job', 'ats-job-acme-aaaa1111')`); err != nil {
		t.Fatalf("seed ats job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID)
	userCookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	h := &jobsHandlers{
		queries:    queries,
		moderation: moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version)),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	requireMod := auth.RequireRole(queries, "moderator")
	app.Post("/api/v1/jobs", keyAuth, requireMod, h.CreateJob)
	app.Patch("/api/v1/jobs/:slug", keyAuth, requireMod, h.UpdateJob)

	req := func(method, path, cookie string, body string) *http.Request {
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

	const validBody = `{"url":"https://acme.example/jobs/1","title":"Senior Go Developer","company":"Acme","location":"Germany","remote":true,"description":"We use Golang."}`

	var createdSlug string

	t.Run("moderator creates a manual job (201)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs", modCookie, validBody))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		var out struct {
			Data struct {
				PublicSlug string   `json:"public_slug"`
				Source     string   `json:"source"`
				WorkMode   string   `json:"work_mode"`
				Countries  []string `json:"countries"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		createdSlug = out.Data.PublicSlug
		if out.Data.Source != "manual" {
			t.Errorf("source = %q, want manual", out.Data.Source)
		}
		if out.Data.WorkMode != "remote" {
			t.Errorf("work_mode = %q, want remote (from the remote flag)", out.Data.WorkMode)
		}

		// Authorship is recorded and not on the wire.
		var createdBy int64
		var source string
		if err := pool.QueryRow(ctx,
			"SELECT created_by, source FROM jobs WHERE public_slug = $1", createdSlug).Scan(&createdBy, &source); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if createdBy != modID {
			t.Errorf("created_by = %d, want %d", createdBy, modID)
		}
		body, _ := json.Marshal(out)
		if bytes.Contains(body, []byte("created_by")) {
			t.Error("wire response leaked created_by")
		}
	})

	t.Run("re-POST same URL is idempotent (no duplicate)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs", modCookie, validBody))
		if err != nil {
			t.Fatalf("re-create: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM jobs WHERE source = 'manual' AND url = 'https://acme.example/jobs/1'").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("manual jobs for the URL = %d, want 1 (idempotent)", n)
		}
	})

	t.Run("non-moderator is forbidden (403)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs", userCookie, validBody))
		if err != nil {
			t.Fatalf("create as user: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("unauthenticated is rejected (401)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs", "", validBody))
		if err != nil {
			t.Fatalf("create anon: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("missing required field is a 400", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPost, "/api/v1/jobs", modCookie, `{"title":"X","company":"Y"}`))
		if err != nil {
			t.Fatalf("create bad: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("moderator edits the manual job (200)", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPatch, "/api/v1/jobs/"+createdSlug, modCookie, `{"title":"Staff Go Developer"}`))
		if err != nil {
			t.Fatalf("edit: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var title string
		var updatedBy int64
		if err := pool.QueryRow(ctx,
			"SELECT title, updated_by FROM jobs WHERE public_slug = $1", createdSlug).Scan(&title, &updatedBy); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if title != "Staff Go Developer" {
			t.Errorf("title = %q, want the edited value", title)
		}
		if updatedBy != modID {
			t.Errorf("updated_by = %d, want %d", updatedBy, modID)
		}
	})

	t.Run("editing a non-manual job is a 404", func(t *testing.T) {
		resp, err := app.Test(req(fiber.MethodPatch, "/api/v1/jobs/ats-job-acme-aaaa1111", modCookie, `{"title":"Hijacked"}`))
		if err != nil {
			t.Fatalf("edit ats: %v", err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}
