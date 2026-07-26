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
	cookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	h := &API{
		pool:                       pool,
		queries:                    queries,
		issuer:                     iss,
		tracking:                   jobtracking.New(jobtracking.NewQueriesRepository(queries)),
		extensionRedirectAllowlist: []string{extID},
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss)
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	app.Get("/api/v1/auth/extension/connect", cookieAuth, h.ExtensionConnect)
	app.Post("/api/v1/auth/extension/connect", cookieAuth, h.ExtensionConnectSubmit)
	app.Get("/api/v1/me/api-keys", cookieAuth, h.ListAPIKeys)
	app.Delete("/api/v1/me/api-keys/:id", cookieAuth, h.RevokeAPIKey)
	app.Post("/api/v1/jobs/:slug/apply", keyAuth, h.MarkApplied)

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

	// 4.3 + 4.5 Approve mints a named key and returns the token in the fragment;
	// the token then behaves as an ordinary revocable API key.
	var mintedToken string
	var mintedKeyID int64
	t.Run("approve mints a key and returns the token in the fragment", func(t *testing.T) {
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
		// The token must be in the fragment, never the query.
		loc, _ := url.Parse(resp.Header.Get("Location"))
		if loc.Query().Get("token") != "" {
			t.Fatalf("token leaked into the query string")
		}

		rows, err := queries.ListAPIKeysByUser(ctx, userID)
		if err != nil {
			t.Fatalf("list keys: %v", err)
		}
		var found bool
		for _, r := range rows {
			if r.Name == extensionKeyName {
				found, mintedKeyID = true, r.ID
			}
		}
		if !found {
			t.Fatalf("no key named %q after approve", extensionKeyName)
		}
	})

	t.Run("minted token authenticates as the user", func(t *testing.T) {
		r := httptest.NewRequest(fiber.MethodPost, applyPath, nil)
		r.Header.Set("Authorization", "Bearer "+mintedToken)
		resp, _ := app.Test(r)
		if resp.StatusCode != fiber.StatusOK && resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("apply with minted token = %d, want 2xx", resp.StatusCode)
		}
	})

	t.Run("revoking the key stops the token", func(t *testing.T) {
		resp, _ := app.Test(withCookie(httptest.NewRequest(fiber.MethodDelete,
			"/api/v1/me/api-keys/"+itoa(mintedKeyID), nil)))
		if resp.StatusCode != fiber.StatusNoContent && resp.StatusCode != fiber.StatusOK {
			t.Fatalf("revoke = %d, want 2xx", resp.StatusCode)
		}
		r := httptest.NewRequest(fiber.MethodPost, applyPath, nil)
		r.Header.Set("Authorization", "Bearer "+mintedToken)
		resp2, _ := app.Test(r)
		if resp2.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("revoked token = %d, want 401", resp2.StatusCode)
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
