//go:build integration

// Integration tests for the OAuth authorize HTTP flow against a real Postgres:
// the consent screen, the sessionless-visitor redirect, redirect_uri validation
// against a registered client, and the approve/deny decision. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// oauthAuthorizeApp wires the two authorize routes behind OptionalCookieAuth,
// the same gate they run behind in production (auth.go's mw.optionalCookie) —
// a signed-out GET must redirect rather than 401.
func oauthAuthorizeApp(h *authHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	optional := auth.OptionalCookieAuth(iss, testVersions)
	app.Get("/api/v1/oauth/authorize", optional, h.OAuthAuthorize)
	app.Post("/api/v1/oauth/authorize", optional, h.OAuthAuthorizeSubmit)
	return app
}

func seedOAuthUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func seedOAuthTestClient(t *testing.T, queries *db.Queries, clientID string, redirectURIs []string) {
	t.Helper()
	if _, err := queries.RegisterOAuthClient(context.Background(), db.RegisterOAuthClientParams{
		ClientID: clientID, ClientName: "Test MCP Client", RedirectUris: redirectURIs,
	}); err != nil {
		t.Fatalf("RegisterOAuthClient: %v", err)
	}
}

func validAuthorizeQuery(clientID, redirectURI string) string {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {"xyz"},
		"code_challenge":        {"abc"},
		"code_challenge_method": {"S256"},
	}.Encode()
}

func TestOAuthAuthorize_ShowsConsentWhenSignedIn(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "authorize-1@example.test")
	seedOAuthTestClient(t, queries, "client-1", []string{"http://127.0.0.1:9999/callback"})

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	app := oauthAuthorizeApp(&authHandlers{queries: queries, issuer: iss}, iss)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet,
		"/api/v1/oauth/authorize?"+validAuthorizeQuery("client-1", "http://127.0.0.1:9999/callback"), nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestOAuthAuthorize_RejectsUnregisteredRedirect(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "authorize-2@example.test")
	seedOAuthTestClient(t, queries, "client-2", []string{"http://127.0.0.1:9999/callback"})

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	app := oauthAuthorizeApp(&authHandlers{queries: queries, issuer: iss}, iss)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet,
		"/api/v1/oauth/authorize?"+validAuthorizeQuery("client-2", "https://evil.example.com/callback"), nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestOAuthAuthorize_SignedOutVisitorIsRedirectedToSignIn(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	seedOAuthTestClient(t, queries, "client-3", []string{"http://127.0.0.1:9999/callback"})

	iss := auth.NewIssuer("test-secret", time.Hour)
	app := oauthAuthorizeApp(&authHandlers{queries: queries, issuer: iss, frontendOrigin: "https://freehire.me"}, iss)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet,
		"/api/v1/oauth/authorize?"+validAuthorizeQuery("client-3", "http://127.0.0.1:9999/callback"), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("no Location header")
	}
}

func TestOAuthAuthorizeSubmit_IssuesCodeOnApproval(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "authorize-4@example.test")
	seedOAuthTestClient(t, queries, "client-4", []string{"http://127.0.0.1:9999/callback"})

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	app := oauthAuthorizeApp(&authHandlers{queries: queries, issuer: iss}, iss)

	form := url.Values{
		"decision":              {"allow"},
		"client_id":             {"client-4"},
		"redirect_uri":          {"http://127.0.0.1:9999/callback"},
		"state":                 {"xyz"},
		"code_challenge":        {"abc"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.Query().Get("code") == "" {
		t.Error("redirect carries no code")
	}
	if loc.Query().Get("state") != "xyz" {
		t.Errorf("state = %q, want xyz", loc.Query().Get("state"))
	}
}

func TestOAuthAuthorizeSubmit_DenialRedirectsAccessDenied(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "authorize-5@example.test")
	seedOAuthTestClient(t, queries, "client-5", []string{"http://127.0.0.1:9999/callback"})

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	app := oauthAuthorizeApp(&authHandlers{queries: queries, issuer: iss}, iss)

	form := url.Values{
		"decision": {"deny"}, "client_id": {"client-5"},
		"redirect_uri": {"http://127.0.0.1:9999/callback"}, "state": {"xyz"},
		"code_challenge": {"abc"}, "code_challenge_method": {"S256"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.Query().Get("error") != "access_denied" {
		t.Errorf("error = %q, want access_denied", loc.Query().Get("error"))
	}

	rows, err := queries.ListOAuthGrantsByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListOAuthGrantsByUser: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("denial must not create a grant, got %d", len(rows))
	}
}
