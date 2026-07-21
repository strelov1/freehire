package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
)

// fakeProvider is a stub oauth.Provider for handler-level tests.
type fakeProvider struct {
	name     string
	identity oauth.Identity
	err      error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) AuthCodeURL(state string) string {
	return "https://provider.example/consent?state=" + state
}
func (f *fakeProvider) FetchIdentity(ctx context.Context, code string) (oauth.Identity, error) {
	return f.identity, f.err
}

func oauthApp(providers map[string]oauth.Provider) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &API{
		issuer:         auth.NewIssuer("test-secret", time.Hour),
		oauth:          providers,
		oauthCodes:     oauth.NewCodeStore(time.Minute),
		frontendOrigin: "http://app.example",
	}
	app.Get("/api/v1/auth/oauth/providers", h.ListOAuthProviders)
	app.Get("/api/v1/auth/oauth/:provider/start", h.OAuthStart)
	app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	app.Post("/api/v1/auth/oauth/exchange", h.OAuthExchange)
	return app
}

func postOAuthJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func get(t *testing.T, app *fiber.App, path string, cookies ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, path, nil)
	for _, c := range cookies {
		req.Header.Add("Cookie", c)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestListOAuthProviders(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{
		"google": &fakeProvider{name: "google"},
		"github": &fakeProvider{name: "github"},
	})
	resp := get(t, app, "/api/v1/auth/oauth/providers")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Data) != 2 {
		t.Errorf("data = %v, want 2 providers", out.Data)
	}
}

func TestOAuthStart_UnknownProviderIs404(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{})
	if resp := get(t, app, "/api/v1/auth/oauth/myspace/start"); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOAuthStart_RedirectsWithStateCookie(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/start")

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://provider.example/consent?state=") {
		t.Errorf("Location = %q, want provider consent URL", loc)
	}
	setCookie := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(setCookie, oauth.StateCookieName+"=") {
		t.Errorf("Set-Cookie %q missing state cookie", setCookie)
	}
	// The state in the URL must match the cookie value.
	state := strings.TrimPrefix(loc, "https://provider.example/consent?state=")
	if !strings.Contains(setCookie, oauth.StateCookieName+"="+state) {
		t.Errorf("cookie does not carry the redirect state %q", state)
	}
}

func TestOAuthCallback_UnknownProviderIs404(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{})
	if resp := get(t, app, "/api/v1/auth/oauth/myspace/callback?code=x&state=s"); resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOAuthCallback_StateMismatchRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?code=x&state=evil",
		oauth.StateCookieName+"=good")

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "http://app.example/?auth_error=oauth" {
		t.Errorf("Location = %q, want auth_error redirect", loc)
	}
	if sc := strings.Join(resp.Header.Values("Set-Cookie"), "\n"); strings.Contains(sc, auth.CookieName+"=") {
		t.Errorf("session cookie set on failed callback: %q", sc)
	}
}

func TestOAuthCallback_MissingStateCookieRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?code=x&state=s")
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "http://app.example/?auth_error=oauth" {
		t.Errorf("status/Location = %d %q, want error redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestOAuthCallback_MissingCodeRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?state=s", oauth.StateCookieName+"=s")
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "http://app.example/?auth_error=oauth" {
		t.Errorf("status/Location = %d %q, want error redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// --- Mobile flow ------------------------------------------------------------

func TestOAuthStart_MobileSetsPlatformCookie(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})

	resp := get(t, app, "/api/v1/auth/oauth/google/start?platform=mobile")
	setCookie := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(setCookie, oauth.PlatformCookieName+"=mobile") {
		t.Errorf("Set-Cookie %q missing platform=mobile", setCookie)
	}

	// A plain (web) start must not set the platform cookie.
	web := get(t, app, "/api/v1/auth/oauth/google/start")
	if sc := strings.Join(web.Header.Values("Set-Cookie"), "\n"); strings.Contains(sc, oauth.PlatformCookieName+"=mobile") {
		t.Errorf("web start unexpectedly set platform cookie: %q", sc)
	}
}

func TestOAuthCallback_MobileErrorRedirectsToScheme(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	// State mismatch, but the platform cookie marks this a mobile flow → the
	// error must bounce to the app's custom scheme, not the web frontend.
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?code=x&state=evil",
		oauth.StateCookieName+"=good", oauth.PlatformCookieName+"=mobile")

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != oauth.MobileCallbackURL+"?auth_error=oauth" {
		t.Errorf("Location = %q, want mobile auth_error deep link", loc)
	}
}

func TestOAuthExchange_InvalidCodeIs401(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{})
	resp := postOAuthJSON(t, app, "/api/v1/auth/oauth/exchange", `{"code":"nope"}`)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if sc := strings.Join(resp.Header.Values("Set-Cookie"), "\n"); strings.Contains(sc, auth.CookieName+"=") {
		t.Errorf("session cookie set for an invalid code: %q", sc)
	}
}

func TestOAuthExchange_BadBodyIs400(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{})
	resp := postOAuthJSON(t, app, "/api/v1/auth/oauth/exchange", `not json`)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
