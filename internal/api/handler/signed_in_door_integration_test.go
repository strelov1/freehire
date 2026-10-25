//go:build integration

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// TestSignedInDoor_RealCredentials runs the door behind the real key gate against real
// keys: the exemption Cloudflare grants /api/v1/me/* is only safe while no caller without
// a live, full-scope credential (or a session) gets past it, and while the door never
// lets a credential reach a route its own gate would refuse.
func TestSignedInDoor_RealCredentials(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('door@example.test', true) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	queries := db.New(pool)
	h := &authHandlers{queries: queries}
	never := pgtype.Timestamptz{}
	fullKey, _, err := h.mintAPIKey(ctx, userID, "cli", never, auth.ScopeFull)
	if err != nil {
		t.Fatalf("mint full key: %v", err)
	}
	embedKey, _, err := h.mintAPIKey(ctx, userID, "embed", never, auth.ScopeEmbed)
	if err != nil {
		t.Fatalf("mint embed key: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountSignedInDoor(app, auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}))
	app.Get("/api/v1/jobs/search", func(c *fiber.Ctx) error { return c.SendString("results") })
	app.Get("/api/v1/me/api-keys", auth.RequireAuth(iss, testVersions), h.ListAPIKeys)

	get := func(path string, set func(*http.Request)) (int, string) {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if set != nil {
			set(req)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	bearer := func(token string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
	}
	withCookie := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie}) }

	for _, tc := range []struct {
		name   string
		path   string
		set    func(*http.Request)
		status int
	}{
		{"anonymous", "/api/v1/me/api/jobs/search", nil, 401},
		{"junk bearer", "/api/v1/me/api/jobs/search", bearer("fhk_junk"), 401},
		{"a narrow-scoped key", "/api/v1/me/api/jobs/search", bearer(embedKey), 403},
		{"a full-scope key", "/api/v1/me/api/jobs/search", bearer(fullKey), 200},
		{"a session cookie", "/api/v1/me/api/jobs/search", withCookie, 200},
		// Key management is cookie-only; the door must not turn a key into a way in.
		{"a key on a cookie-only route", "/api/v1/me/api/me/api-keys", bearer(fullKey), 401},
	} {
		if status, body := get(tc.path, tc.set); status != tc.status {
			t.Errorf("%s: GET %s = %d, want %d (body %q)", tc.name, tc.path, status, tc.status, body)
		}
	}
}
