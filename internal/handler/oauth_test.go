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
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	h := &Handler{
		issuer:         auth.NewIssuer("test-secret", time.Hour),
		oauth:          providers,
		frontendOrigin: "http://app.example",
	}
	app.Get("/api/v1/auth/oauth/providers", h.ListOAuthProviders)
	app.Get("/api/v1/auth/oauth/:provider/start", h.OAuthStart)
	app.Get("/api/v1/auth/oauth/:provider/callback", h.OAuthCallback)
	return app
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
