//go:build integration

// Integration test for the saved-slugs wire contract: GET /api/v1/me/tracking/saved
// must return exactly the public_slugs the authenticated caller has SAVED
// (bookmarked) — not merely viewed or applied — scoped to that caller, as a flat
// {"data": [...]} list, and reject an unauthenticated request. The SPA reads this
// to render the save toggle as filled on already-saved cards without authenticating
// the public job-read path. Run with:
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

func TestListSavedSlugsEndpoint(t *testing.T) {
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

	saver := seedUser(t, "saver@example.test")
	other := seedUser(t, "other@example.test")
	jobA := seedJob(t, "saved-a")
	jobB := seedJob(t, "viewed-b")
	jobC := seedJob(t, "other-c")

	// saver SAVED A, but only VIEWED B — the viewed-only job must be excluded from
	// the saved set (the saved_at IS NOT NULL predicate is the whole point).
	if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: saver, JobID: jobA}); err != nil {
		t.Fatalf("save A: %v", err)
	}
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: saver, JobID: jobB}); err != nil {
		t.Fatalf("view B: %v", err)
	}
	// other user saved C — must never leak into saver's set.
	if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: other, JobID: jobC}); err != nil {
		t.Fatalf("save C: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss, tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking/saved", auth.RequireAuthOrKey(iss, queries), h.ListSavedSlugs)

	getSlugs := func(t *testing.T, userID int64) []string {
		t.Helper()
		token, err := iss.Issue(userID)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/saved", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET saved: %v", err)
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

	t.Run("returns only the caller's saved slugs, scoped to the caller", func(t *testing.T) {
		got := getSlugs(t, saver)
		want := []string{"job-saved-a"}
		if !slices.Equal(got, want) {
			t.Fatalf("saver slugs = %v, want %v", got, want)
		}
	})

	t.Run("unsaving drops the slug from the set", func(t *testing.T) {
		if _, err := queries.UnsaveJob(ctx, db.UnsaveJobParams{UserID: saver, JobID: jobA}); err != nil {
			t.Fatalf("unsave A: %v", err)
		}
		got := getSlugs(t, saver)
		if len(got) != 0 {
			t.Fatalf("after unsave slugs = %v, want []", got)
		}
		// Restore for isolation from any later assertions.
		if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: saver, JobID: jobA}); err != nil {
			t.Fatalf("re-save A: %v", err)
		}
	})

	t.Run("user with no saves gets an empty list", func(t *testing.T) {
		fresh := seedUser(t, "fresh@example.test")
		got := getSlugs(t, fresh)
		if len(got) != 0 {
			t.Fatalf("fresh user slugs = %v, want []", got)
		}
	})

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/tracking/saved", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET saved (no auth): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})
}
