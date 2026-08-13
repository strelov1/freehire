package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/tokencrypt"
)

// The Gmail callback is a top-level browser navigation returning from Google, not
// an XHR: a caller whose session cookie did not survive the round-trip must land
// back on the inbox with an error marker, never on a JSON 401 with no way forward.
// This is the shape the handler was written for — RequireAuth on the route made its
// own unauthenticated branch unreachable and rendered `{"error":"not authenticated"}`
// into the browser instead.
func TestGmailCallbackWithoutSessionRedirectsInsteadOfJSON(t *testing.T) {
	app, frontend := newGmailCallbackApp(t)

	// No auth cookie: the state the browser carried back is irrelevant, the session
	// is what is missing.
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/me/gmail/callback?state=abc&code=xyz", nil), -1)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want %d (a redirect, not a JSON error)", resp.StatusCode, fiber.StatusFound)
	}
	if got, want := resp.Header.Get("Location"), frontend+"/my/inbox?gmail_error=auth"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// A signed-in caller whose state cookie is missing or mismatched is the CSRF case:
// it must be refused, and refused the same way — a redirect carrying the marker, so
// the SPA can explain what happened.
func TestGmailCallbackStateMismatchRedirects(t *testing.T) {
	app, frontend := newGmailCallbackApp(t)

	iss := auth.NewIssuer(testGmailSecret, time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/me/gmail/callback?state=abc&code=xyz", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+token)
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusFound)
	}
	if got, want := resp.Header.Get("Location"), frontend+"/my/inbox?gmail_error=state"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

const testGmailSecret = "test-secret-that-is-long-enough-0001"

// newGmailCallbackApp mounts the real route table (register), so the middleware the
// callback is actually served behind is what these tests exercise.
func newGmailCallbackApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	cipher, err := tokencrypt.New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	const frontend = "https://freehire.me"
	h := newInboxHandlers(nil, nil, gmailsync.NewConnector("id", "secret", frontend), cipher, frontend, true, "")

	iss := auth.NewIssuer(testGmailSecret, time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie:         auth.RequireAuth(iss, testVersions),
		optionalCookie: auth.OptionalCookieAuth(iss, testVersions),
		key:            auth.RequireAuthOrKey(iss, testVersions, nil),
	})
	return app, frontend
}
