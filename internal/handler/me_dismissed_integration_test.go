//go:build integration

// Integration test for the dismissed-slugs wire contract: GET /api/v1/me/tracking/dismissed
// must return exactly the public_slugs the authenticated caller has HIDDEN
// (dismissed) — not merely viewed — scoped to that caller, as a flat
// {"data": [...]} list, and reject an unauthenticated request. The SPA reads this
// to exclude hidden jobs from the browse feed without authenticating the public
// job-read path. It also covers the tracking listing's `dismissed` filter. Run with:
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
	"github.com/strelov1/freehire/internal/jobtracking"
)

func TestListDismissedSlugsEndpoint(t *testing.T) {
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

	hider := seedUser(t, "hider@example.test")
	other := seedUser(t, "other-h@example.test")
	jobA := seedJob(t, "dismissed-a")
	jobB := seedJob(t, "viewed-h-b")
	jobC := seedJob(t, "other-h-c")

	// hider DISMISSED A, but only VIEWED B — the viewed-only job must be excluded
	// from the dismissed set (the dismissed_at IS NOT NULL predicate is the point).
	if _, err := queries.DismissJob(ctx, db.DismissJobParams{UserID: hider, JobID: jobA}); err != nil {
		t.Fatalf("dismiss A: %v", err)
	}
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: hider, JobID: jobB}); err != nil {
		t.Fatalf("view B: %v", err)
	}
	// other user dismissed C — must never leak into hider's set.
	if _, err := queries.DismissJob(ctx, db.DismissJobParams{UserID: other, JobID: jobC}); err != nil {
		t.Fatalf("dismiss C: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss, tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking/dismissed", auth.RequireAuthOrKey(iss, queries), h.ListDismissedSlugs)
	app.Get("/api/v1/me/tracking", auth.RequireAuthOrKey(iss, queries), h.ListTrackedJobs)

	getSlugs := func(t *testing.T, userID int64) []string {
		t.Helper()
		token, err := iss.Issue(userID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/dismissed", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET dismissed: %v", err)
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

	t.Run("returns only the caller's dismissed slugs, scoped to the caller", func(t *testing.T) {
		got := getSlugs(t, hider)
		want := []string{"job-dismissed-a"}
		if !slices.Equal(got, want) {
			t.Fatalf("hider slugs = %v, want %v", got, want)
		}
	})

	t.Run("undismissing drops the slug from the set", func(t *testing.T) {
		if _, err := queries.UndismissJob(ctx, db.UndismissJobParams{UserID: hider, JobID: jobA}); err != nil {
			t.Fatalf("undismiss A: %v", err)
		}
		got := getSlugs(t, hider)
		if len(got) != 0 {
			t.Fatalf("after undismiss slugs = %v, want []", got)
		}
		// Restore for the filter assertion below.
		if _, err := queries.DismissJob(ctx, db.DismissJobParams{UserID: hider, JobID: jobA}); err != nil {
			t.Fatalf("re-dismiss A: %v", err)
		}
	})

	t.Run("dismissed filter lists the hidden job, excludes the viewed-only one", func(t *testing.T) {
		token, err := iss.Issue(hider)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking?filter=dismissed", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET tracking dismissed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Job struct {
					PublicSlug string `json:"public_slug"`
				} `json:"job"`
			} `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Meta.Total != 1 {
			t.Fatalf("meta.total = %d, want 1", body.Meta.Total)
		}
		if len(body.Data) != 1 || body.Data[0].Job.PublicSlug != "job-dismissed-a" {
			t.Fatalf("dismissed listing = %+v, want just job-dismissed-a", body.Data)
		}
	})

	t.Run("user with no dismisses gets an empty list", func(t *testing.T) {
		fresh := seedUser(t, "fresh-h@example.test")
		got := getSlugs(t, fresh)
		if len(got) != 0 {
			t.Fatalf("fresh user slugs = %v, want []", got)
		}
	})

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/dismissed", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET dismissed (no auth): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})
}
