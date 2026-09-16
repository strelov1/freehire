package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
)

// Sign-in and re-authentication share ONE registered callback URL, because a provider
// console has room for one. They are told apart here, at the callback, by which state
// cookie the browser brings back: each flow sets its own, under its own name, and each
// is single-use.
//
// Before this, re-authentication asked the provider to return to `/api/v2/...` — a URL
// only Google had ever been told about. GitHub answered "The redirect_uri is not
// associated with this application" and the member could not confirm at all.

func dispatchApp(v2Enabled bool) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &authHandlers{
		issuer:         auth.NewIssuer("test-secret", time.Hour),
		oauth:          fakeRegistry(map[string]oauth.Provider{"github": &fakeProvider{name: "github"}}),
		oauthCodes:     oauth.NewCodeStore(time.Minute),
		frontendOrigin: "http://app.example",
		authV2Enabled:  v2Enabled,
	}
	app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	return app
}

// Returns where the callback sent the browser — the one thing that tells the two flows
// apart from outside.
func callbackLocation(t *testing.T, app *fiber.App, cookies map[string]string) string {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/auth/oauth/github/callback?state=S&code=C", nil)
	for name, value := range cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.Header.Get("Location")
}

// The sign-in path answers every failure with a redirect carrying auth_error; seeing it
// means the request was treated as a sign-in.
const signInFailureRedirect = "http://app.example/?auth_error=oauth"

func TestOAuthCallback_HandsAReauthenticationReturnToTheV2Flow(t *testing.T) {
	loc := callbackLocation(t, dispatchApp(true), map[string]string{oauthV2StateCookieName: "S"})

	if loc == signInFailureRedirect {
		t.Fatalf("re-authentication return was handled as a sign-in: %s", loc)
	}
}

func TestOAuthCallback_StillSignsInWhenOnlyTheSignInStateIsPresent(t *testing.T) {
	// The ordinary flow must not change: its own cookie still selects it, even with the
	// other flow enabled. A deliberately mismatched value keeps the case at the state
	// check, which is the branch under test — going further would need the account
	// resolver this stub has no business carrying.
	loc := callbackLocation(t, dispatchApp(true), map[string]string{oauth.StateCookieName: "OTHER"})

	if loc != signInFailureRedirect {
		t.Fatalf("got %q; want the sign-in path to have handled it", loc)
	}
}

func TestOAuthCallback_DoesNotDispatchWhileTheSecondFlowIsOff(t *testing.T) {
	// With AUTH_V2 disabled no re-authentication attempt can exist, so a cookie of that
	// name proves nothing and must not divert an ordinary sign-in.
	loc := callbackLocation(t, dispatchApp(false), map[string]string{oauthV2StateCookieName: "S"})

	if loc != signInFailureRedirect {
		t.Fatalf("got %q; want the sign-in state-mismatch redirect", loc)
	}
}
