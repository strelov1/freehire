//go:build integration

// Integration test for POST /api/v1/auth/logout-all: signing out everywhere bumps the
// account's session generation, so every token minted before it — including ones held by
// other devices — stops authenticating. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// logoutAllApp mounts the revocation endpoint plus one protected probe, both against the
// real token-version column, so the test observes revocation the way a client would.
func logoutAllApp(h *API, iss *auth.Issuer, queries *db.Queries) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/auth/logout-all", auth.RequireAuth(iss, queries), h.LogoutAll)
	app.Get("/api/v1/probe", auth.RequireAuth(iss, queries), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func probeWithCookie(t *testing.T, app *fiber.App, token string) int {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/probe", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestLogoutAllRevokesEverySession(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('logoutall@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss}
	app := logoutAllApp(h, iss, queries)

	// Two devices, both signed in under the current generation.
	laptop, _ := iss.Issue(userID, 1)
	phone, _ := iss.Issue(userID, 1)

	if got := probeWithCookie(t, app, phone); got != fiber.StatusOK {
		t.Fatalf("phone probe before revocation = %d, want 200", got)
	}

	req := httptest.NewRequest(fiber.MethodPost, "/api/v1/auth/logout-all", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: laptop})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("logout-all: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("logout-all status = %d, want 204", resp.StatusCode)
	}

	if got := probeWithCookie(t, app, phone); got != fiber.StatusUnauthorized {
		t.Errorf("phone probe after revocation = %d, want 401 — the other device must be signed out", got)
	}
	if got := probeWithCookie(t, app, laptop); got != fiber.StatusUnauthorized {
		t.Errorf("caller's own token after revocation = %d, want 401 — everywhere means everywhere", got)
	}

	var version int32
	if err := pool.QueryRow(ctx, `SELECT token_version FROM users WHERE id = $1`, userID).Scan(&version); err != nil {
		t.Fatalf("read token_version: %v", err)
	}
	if version != 2 {
		t.Errorf("token_version = %d, want 2", version)
	}
}

func TestLogoutAllRequiresASession(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss}
	app := logoutAllApp(h, iss, queries)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/api/v1/auth/logout-all", nil))
	if err != nil {
		t.Fatalf("logout-all: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without a session cookie", resp.StatusCode)
	}
}
