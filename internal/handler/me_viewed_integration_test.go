//go:build integration

// Integration test for the viewed-slugs wire contract: GET /api/v1/me/jobs/viewed
// must return exactly the public_slugs the authenticated caller has interacted
// with, scoped to that caller, as a flat {"data": [...]} list, and reject an
// unauthenticated request. The SPA reads this to dim already-seen cards without
// authenticating the public job-read path. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

func TestListViewedSlugsEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seedUser := func(t *testing.T, email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed user %q: %v", email, err)
		}
		return id
	}
	seedJob := func(t *testing.T, ext string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test', 'Job ' || $1, 'job-' || $1)
			 RETURNING id`, ext).Scan(&id); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
		return id
	}

	viewer := seedUser(t, "viewer@example.test")
	other := seedUser(t, "other@example.test")
	jobA := seedJob(t, "viewed-a")
	jobB := seedJob(t, "viewed-b")
	jobC := seedJob(t, "other-c")

	// viewer interacted with A (view) and B (apply — also counts as viewed).
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: viewer, JobID: jobA}); err != nil {
		t.Fatalf("view A: %v", err)
	}
	if _, err := queries.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: viewer, JobID: jobB}); err != nil {
		t.Fatalf("apply B: %v", err)
	}
	// other user viewed C — must never leak into viewer's set.
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: other, JobID: jobC}); err != nil {
		t.Fatalf("view C: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/jobs/viewed", auth.RequireAuthOrKey(iss, queries), h.ListViewedSlugs)

	getSlugs := func(t *testing.T, userID int64) []string {
		t.Helper()
		token, err := iss.Issue(userID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/jobs/viewed", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET viewed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []string `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		sort.Strings(body.Data)
		return body.Data
	}

	t.Run("returns the caller's viewed slugs, scoped to the caller", func(t *testing.T) {
		got := getSlugs(t, viewer)
		want := []string{"job-viewed-a", "job-viewed-b"}
		if !slices.Equal(got, want) {
			t.Fatalf("viewer slugs = %v, want %v", got, want)
		}
	})

	t.Run("user with no interactions gets an empty list", func(t *testing.T) {
		fresh := seedUser(t, "fresh@example.test")
		got := getSlugs(t, fresh)
		if len(got) != 0 {
			t.Fatalf("fresh user slugs = %v, want []", got)
		}
	})

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/jobs/viewed", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET viewed (no auth): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})
}
