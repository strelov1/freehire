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
	// optionalCookie, exactly as authHandlers.register mounts it: the flow runs in a
	// browser window, so a sessionless visitor is sent to sign in rather than refused.
	// Mounting RequireAuth here would test a route that does not exist.
	optionalCookie := auth.OptionalCookieAuth(iss, testVersions)
	app.Get("/api/v1/auth/extension/connect", optionalCookie, h.ExtensionConnect)
	app.Post("/api/v1/auth/extension/connect", optionalCookie, h.ExtensionConnectSubmit)
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

	// 4.1 Session-only, but a browser navigation: a sessionless caller is sent to sign in
	// (Chrome's auth window has its own cookie jar, so this is the normal first run) and
	// a Bearer is still no session — neither issues anything.
	t.Run("anonymous GET is sent to sign in", func(t *testing.T) {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, connectPath+"?redirect_uri="+url.QueryEscape(redirectURI)+"&state=s1", nil))
		if resp.StatusCode != fiber.StatusFound {
			t.Fatalf("status = %d, want 302", resp.StatusCode)
		}
		loc, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("parse Location %q: %v", resp.Header.Get("Location"), err)
		}
		if loc.Path != "/extension/connect" {
			t.Fatalf("sign-in path = %q, want /extension/connect", loc.Path)
		}
		// The extension's parameters survive the round trip, or the visitor comes back
		// signed in to a flow that no longer knows where to send the token.
		if got := loc.Query().Get("redirect_uri"); got != redirectURI {
			t.Fatalf("redirect_uri = %q, want %q", got, redirectURI)
		}
		if got := loc.Query().Get("state"); got != "s1" {
			t.Fatalf("state = %q, want s1", got)
		}
	})

	// The loop stop: back from the web app and still no session. Bouncing again would
	// spin forever, so this says so instead.
	t.Run("anonymous GET back from the web app is refused, not bounced", func(t *testing.T) {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, connectPath+"?redirect_uri="+url.QueryEscape(redirectURI)+"&via=web", nil))
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	// An unlisted redirect is refused before the sign-in bounce: a crafted link must not
	// ride a sign-in round trip, and a 302 would tell the caller it is a valid target.
	t.Run("anonymous GET with an unlisted redirect is rejected", func(t *testing.T) {
		resp, _ := app.Test(httptest.NewRequest(fiber.MethodGet, connectPath+"?redirect_uri="+url.QueryEscape("https://evil.example.com/"), nil))
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("bearer-only POST issues nothing", func(t *testing.T) {
		before := keyCount()
		r := form(url.Values{"redirect_uri": {redirectURI}, "decision": {"allow"}})
		r.Header.Set("Authorization", "Bearer some-key-value")
		resp, _ := app.Test(r)
		// Sent to sign in, like any other sessionless navigation — and to the web app,
		// never to the extension's own redirect, so no token can ride along.
		if resp.StatusCode != fiber.StatusFound {
			t.Fatalf("status = %d, want 302", resp.StatusCode)
		}
		loc, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("parse Location %q: %v", resp.Header.Get("Location"), err)
		}
		// The extension's own redirect only ever appears as a parameter to carry through
		// the sign-in, never as the destination — and nothing rides the fragment.
		if loc.Host != "" || loc.Path != "/extension/connect" || loc.Fragment != "" {
			t.Fatalf("sessionless POST redirected to %q, want the sign-in page", resp.Header.Get("Location"))
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
