package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	called   bool
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) AuthCodeURL(state string) string {
	return "https://provider.example/consent?state=" + state
}
func (f *fakeProvider) FetchIdentity(ctx context.Context, code string) (oauth.Identity, error) {
	f.called = true
	return f.identity, f.err
}

// fakeRegistry adapts a name->Provider map to the handler's oauthRegistry seam;
// origin is ignored (the fake providers carry a fixed AuthCodeURL).
type fakeRegistry map[string]oauth.Provider

func (r fakeRegistry) Names() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	return names
}
func (r fakeRegistry) Provider(name, _ string) (oauth.Provider, bool) {
	p, ok := r[name]
	return p, ok
}
func (r fakeRegistry) ProviderV2(name, _ string) (oauth.Provider, bool) {
	p, ok := r[name]
	return p, ok
}

func oauthApp(providers map[string]oauth.Provider) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &authHandlers{
		issuer:         auth.NewIssuer("test-secret", time.Hour),
		oauth:          fakeRegistry(providers),
		oauthCodes:     oauth.NewCodeStore(time.Minute),
		frontendOrigin: "http://app.example",
	}
	app.Get("/api/v1/auth/oauth/providers", h.ListOAuthProviders)
	app.Get("/api/v1/auth/oauth/:provider/start", h.OAuthStart)
	app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	app.Post("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	app.Post("/api/v1/auth/oauth/exchange", h.OAuthExchange)
	return app
}

func postOAuthJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func get(t *testing.T, app *fiber.App, path string, cookies ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
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
	defer resp.Body.Close()
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
	resp := get(t, app, "/api/v1/auth/oauth/myspace/start")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOAuthStart_RedirectsWithStateCookie(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/start")
	defer resp.Body.Close()

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
	resp := get(t, app, "/api/v1/auth/oauth/myspace/callback?code=x&state=s")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOAuthCallback_StateMismatchRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?code=x&state=evil",
		oauth.StateCookieName+"=good")
	defer resp.Body.Close()

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
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "http://app.example/?auth_error=oauth" {
		t.Errorf("status/Location = %d %q, want error redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// postFormOAuth issues a POST with a form-encoded body, as Apple's callback
// arrives (response_mode=form_post is mandatory once the email scope is
// requested), unlike every other provider's GET query-string callback.
func postFormOAuth(t *testing.T, app *fiber.App, path, body string, cookies ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", fiber.MIMEApplicationForm)
	for _, c := range cookies {
		req.Header.Add("Cookie", c)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

// The handler has no seam to fake accounts.Service (it's a concrete struct
// backed by a real DB — see oauth_integration_test.go for the full round
// trip), so this proves the POST form body's state/code were correctly
// parsed by asserting the flow reaches FetchIdentity at all: a fake provider
// error short-circuits before the handler ever touches h.accounts.
func TestOAuthCallback_POSTFormBodyReachesProvider(t *testing.T) {
	provider := &fakeProvider{name: "apple", err: errors.New("boom")}
	app := oauthApp(map[string]oauth.Provider{"apple": provider})
	resp := postFormOAuth(t, app, "/api/v1/auth/oauth/apple/callback", "code=x&state=s",
		oauth.StateCookieName+"=s")
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if !provider.called {
		t.Error("FetchIdentity not called — state/code were not read from the POST form body")
	}
}

func TestOAuthCallback_POSTStateMismatchRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"apple": &fakeProvider{name: "apple"}})
	resp := postFormOAuth(t, app, "/api/v1/auth/oauth/apple/callback", "code=x&state=evil",
		oauth.StateCookieName+"=good")
	defer resp.Body.Close()

	if loc := resp.Header.Get("Location"); loc != "http://app.example/?auth_error=oauth" {
		t.Errorf("Location = %q, want auth_error redirect", loc)
	}
}

func TestOAuthCallback_MissingCodeRedirectsWithError(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})
	resp := get(t, app, "/api/v1/auth/oauth/google/callback?state=s", oauth.StateCookieName+"=s")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "http://app.example/?auth_error=oauth" {
		t.Errorf("status/Location = %d %q, want error redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// A callback arriving on a served domain must redirect the browser back to THAT
// domain (host-derived origin), so sign-in stays on .me during the migration; an
// unknown Host falls back to the canonical frontendOrigin (no open redirect).
func TestOAuthCallback_ErrorRedirectUsesRequestOrigin(t *testing.T) {
	newApp := func() *fiber.App {
		app := fiber.New(fiber.Config{ErrorHandler: RenderError})
		h := &authHandlers{
			issuer:         auth.NewIssuer("test-secret", time.Hour),
			oauth:          fakeRegistry{"google": &fakeProvider{name: "google"}},
			oauthCodes:     oauth.NewCodeStore(time.Minute),
			frontendOrigin: "https://freehire.dev",
			cookieDomains:  []string{"freehire.dev", "freehire.me"},
			servedHosts:    []string{"freehire.dev", "freehire.me"},
		}
		app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
		return app
	}

	// Force the state-mismatch error path; the redirect origin is what we assert.
	// host drives requestOrigin, so it's set explicitly (app.Test needs Host set).
	callback := func(host string) string {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/auth/oauth/google/callback?code=x&state=evil", nil)
		req.Host = host
		req.Header.Add("Cookie", oauth.StateCookieName+"=good")
		resp, err := newApp().Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.Header.Get("Location")
	}

	if loc := callback("freehire.me"); loc != "https://freehire.me/?auth_error=oauth" {
		t.Errorf("served-domain callback Location = %q, want host-derived https://freehire.me", loc)
	}
	if loc := callback("evil.example"); loc != "https://freehire.dev/?auth_error=oauth" {
		t.Errorf("unknown-host callback Location = %q, want canonical frontendOrigin fallback", loc)
	}
}

// --- Mobile flow ------------------------------------------------------------

func TestOAuthStart_MobileSetsPlatformCookie(t *testing.T) {
	app := oauthApp(map[string]oauth.Provider{"google": &fakeProvider{name: "google"}})

	resp := get(t, app, "/api/v1/auth/oauth/google/start?platform=mobile")
	defer resp.Body.Close()
	setCookie := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(setCookie, oauth.PlatformCookieName+"=mobile") {
		t.Errorf("Set-Cookie %q missing platform=mobile", setCookie)
	}

	// A plain (web) start must not set the platform cookie.
	web := get(t, app, "/api/v1/auth/oauth/google/start")
	defer web.Body.Close()
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// requestOrigin must trust the request Host only when it names a host this deployment
// actually serves. Suffix-matching a cookie domain lets any subdomain — including a
// hijacked or third-party-hosted one — steer the OAuth redirect.
func TestRequestOrigin_OnlyTrustsAnExactServedHost(t *testing.T) {
	h := &authHandlers{
		frontendOrigin: "https://freehire.me",
		cookieDomains:  []string{"freehire.me"},
		servedHosts:    []string{"freehire.me", "apply.freehire.me"},
	}

	cases := []struct {
		host, want string
	}{
		{"apply.freehire.me", "https://apply.freehire.me"},
		{"freehire.me", "https://freehire.me"},
		{"takeover.freehire.me", "https://freehire.me"},
		{"evil.example", "https://freehire.me"},
	}
	for _, tc := range cases {
		if got := originForHost(t, h, tc.host); got != tc.want {
			t.Errorf("Host %q → origin %q, want %q", tc.host, got, tc.want)
		}
	}
}

// With no SERVED_HOSTS configured the canonical frontend origin's own host is still
// honoured, so a deployment that never sets the variable keeps working.
func TestRequestOrigin_DefaultsToTheFrontendHost(t *testing.T) {
	h := &authHandlers{
		frontendOrigin: "https://freehire.me",
		servedHosts:    servedHostsOrDefault(nil, "https://freehire.me"),
	}
	if got := originForHost(t, h, "freehire.me"); got != "https://freehire.me" {
		t.Errorf("origin = %q, want the frontend host to be served by default", got)
	}
}

// originForHost runs requestOrigin for one request Host.
func originForHost(t *testing.T, h *authHandlers, host string) string {
	t.Helper()
	app := fiber.New()
	app.Get("/origin", func(c *fiber.Ctx) error { return c.SendString(h.requestOrigin(c)) })

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/origin", nil)
	req.Host = host
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}
