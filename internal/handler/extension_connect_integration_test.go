//go:build integration

// Integration tests for the browser-extension connect flow against a real
// Postgres: the endpoint is cookie-only, it rejects unlisted redirects, approval
// mints a named key and returns the token in the redirect fragment, decline mints
// nothing, and the minted token behaves as an ordinary revocable API key. Run with:
// go test -tags=integration ./internal/handler/
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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobtracking"
)

func TestExtensionConnectEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('ext@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'ext-1', 'http://example.test', 'Go Dev', 'go-dev-acme-t35nijto')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	const applyPath = "/api/v1/jobs/go-dev-acme-t35nijto/apply"

	const extID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	redirectURI := "https://" + extID + ".chromiumapp.org/"

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	queries := db.New(pool)
	h := &authHandlers{
		queries:                    queries,
		issuer:                     iss,
		extensionRedirectAllowlist: []string{extID},
	}
	th := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss, testVersions)
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Get("/api/v1/auth/extension/connect", cookieAuth, h.ExtensionConnect)
	app.Post("/api/v1/auth/extension/connect", cookieAuth, h.ExtensionConnectSubmit)
	app.Get("/api/v1/me/api-keys", cookieAuth, h.ListAPIKeys)
	app.Delete("/api/v1/me/api-keys/:id", cookieAuth, h.RevokeAPIKey)
	app.Post("/api/v1/jobs/:slug/apply", keyAuth, th.MarkApplied)

	const connectPath = "/api/v1/auth/extension/connect"

	withCookie := func(r *http.Request) *http.Request {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}
	form := func(vals url.Values) *http.Request {
		r := httptest.NewRequest(fiber.MethodPost, connectPath, strings.NewReader(vals.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	keyCount := func() int {
		rows, err := queries.ListAPIKeysByUser(ctx, userID)
		if err != nil {
			t.Fatalf("count keys: %v", err)
		}
		return len(rows)
	}

	// 4.1 Session-only: no cookie and Bearer-only are both rejected; nothing minted.
	t.Run("anonymous GET is rejected", func(t *testing.T) {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, connectPath+"?redirect_uri="+url.QueryEscape(redirectURI), nil))
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})
	t.Run("bearer-only POST is rejected and mints nothing", func(t *testing.T) {
		before := keyCount()
		r := form(url.Values{"redirect_uri": {redirectURI}, "decision": {"allow"}})
		r.Header.Set("Authorization", "Bearer some-key-value")
		resp, _ := app.Test(r)
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if after := keyCount(); after != before {
			t.Fatalf("keys changed %d -> %d, want no mint", before, after)
		}
	})

	// 4.2 Redirect validation: an unlisted redirect is refused with 400, no mint.
	t.Run("unlisted redirect is rejected and mints nothing", func(t *testing.T) {
		before := keyCount()
		resp, _ := app.Test(withCookie(form(url.Values{
			"redirect_uri": {"https://evil.example.com/"},
			"decision":     {"allow"},
		})))
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if after := keyCount(); after != before {
			t.Fatalf("keys changed %d -> %d, want no mint", before, after)
		}
	})

	// 4.4 Decline: mints nothing, redirects an error (not a token).
	t.Run("decline redirects an error and mints nothing", func(t *testing.T) {
		before := keyCount()
		resp, _ := app.Test(withCookie(form(url.Values{
			"redirect_uri": {redirectURI},
			"state":        {"s1"},
			"decision":     {"cancel"},
		})))
		if resp.StatusCode != fiber.StatusFound {
			t.Fatalf("status = %d, want 302", resp.StatusCode)
		}
		frag := fragmentValues(t, resp.Header.Get("Location"))
		if frag.Get("token") != "" {
			t.Fatalf("decline returned a token: %q", frag.Get("token"))
		}
		if frag.Get("error") == "" {
			t.Fatalf("decline missing error indication, fragment = %q", resp.Header.Get("Location"))
		}
		if after := keyCount(); after != before {
			t.Fatalf("keys changed %d -> %d, want no mint", before, after)
		}
	})

	// 4.3 Approve issues a session JWT (not an API key) in the fragment, with state.
	var mintedToken string
	t.Run("approve returns a session JWT in the fragment", func(t *testing.T) {
		before := keyCount()
		resp, _ := app.Test(withCookie(form(url.Values{
			"redirect_uri": {redirectURI},
			"state":        {"abc"},
			"decision":     {"allow"},
		})))
		if resp.StatusCode != fiber.StatusFound {
			t.Fatalf("status = %d, want 302", resp.StatusCode)
		}
		frag := fragmentValues(t, resp.Header.Get("Location"))
		if frag.Get("state") != "abc" {
			t.Fatalf("state = %q, want abc", frag.Get("state"))
		}
		mintedToken = frag.Get("token")
		if mintedToken == "" {
			t.Fatalf("no token in fragment: %q", resp.Header.Get("Location"))
		}
		// The token rides the fragment, never the query.
		loc, _ := url.Parse(resp.Header.Get("Location"))
		if loc.Query().Get("token") != "" {
			t.Fatalf("token leaked into the query string")
		}
		// It is a session JWT for the user — not an API key (no api_keys row written) —
		// and it carries the account's session generation, so logout-all evicts it.
		id, version, err := iss.Parse(mintedToken)
		if err != nil || id != userID {
			t.Fatalf("token is not a valid JWT for the user: id=%d err=%v", id, err)
		}
		if version != testTokenVersion {
			t.Fatalf("token version = %d, want the account's current generation %d", version, testTokenVersion)
		}
		if after := keyCount(); after != before {
			t.Fatalf("connect wrote an api_keys row (%d -> %d); it should issue a JWT only", before, after)
		}
	})

	t.Run("the JWT authenticates a per-user endpoint via Bearer", func(t *testing.T) {
		r := httptest.NewRequest(fiber.MethodPost, applyPath, nil)
		r.Header.Set("Authorization", "Bearer "+mintedToken)
		resp, _ := app.Test(r)
		if resp.StatusCode != fiber.StatusOK && resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("apply with minted JWT = %d, want 2xx", resp.StatusCode)
		}
	})
}

func fragmentValues(t *testing.T, location string) url.Values {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location %q: %v", location, err)
	}
	vals, err := url.ParseQuery(u.Fragment)
	if err != nil {
		t.Fatalf("parse fragment %q: %v", u.Fragment, err)
	}
	return vals
}
