//go:build integration

package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The account routes that moved into the contract keep the middleware chain `register`
// mounts. This builds the real chain (recent-auth gate on) and checks what it answers.
func TestAccountContractRoutes_KeepTheirGates(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{
		queries:       queries,
		issuer:        iss,
		accounts:      accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
		authV2Enabled: true,
		recentAuth:    recentauth.NewStore(pool, time.Minute),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookie := auth.RequireAuth(iss, queries)
	h.register(app.Group("/api/v1"), middleware{cookie: cookie, cvKey: cookie, optionalCookie: cookie},
		newContract(contractHandlers{authHandlers: h}))

	var userID int64
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO users (email, password_hash, email_verified) VALUES ('gates@example.test', 'x', true) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	session, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	do := func(method, path, body string) *http.Response {
		req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if session != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		}
		resp, err := app.Test(req, 10_000)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}

	// The recent-auth gate answers before the contract does, for every gated route.
	for _, route := range []struct{ method, path, body string }{
		{fiber.MethodPost, "/api/v1/me/api-keys", `{"name":"ci"}`},
		{fiber.MethodDelete, "/api/v1/me/api-keys/1", ``},
		{fiber.MethodPost, "/api/v1/me/email", `{"email":"new@example.test"}`},
		{fiber.MethodDelete, "/api/v1/me/oauth-grants/1", ``},
	} {
		resp := do(route.method, route.path, route.body)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 428 || !strings.Contains(string(raw), "recent_auth_required") {
			t.Errorf("%s %s = %d %s, want 428 recent_auth_required", route.method, route.path, resp.StatusCode, raw)
		}
	}

	// The contract still answers no-store on its own routes.
	resp := do(fiber.MethodGet, "/api/v1/me/api-keys", ``)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("list keys = %d, Cache-Control %q, want 200 no-store", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}

	// Signed out, the identity read is a 401 from the gate.
	session = ""
	resp = do(fiber.MethodGet, "/api/v1/auth/me", ``)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("GET /auth/me signed out = %d %s, want 401", resp.StatusCode, raw)
	}
}
