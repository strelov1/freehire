//go:build integration

// Integration tests for RFC 8707 resource indicators on the OAuth server
// against a real Postgres: the resource is validated at /authorize and /token,
// bound to the grant, and enforced by the two MCP endpoints. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	resourceCB      = "http://127.0.0.1:9999/callback"
	resourceMixed   = testMCPOrigin + "/api/v1/mcp"
	resourceAccount = testMCPOrigin + "/api/v1/mcp/account"
	resourceVerif   = "a-high-entropy-verifier-the-client-generated-1234567890"
)

type resourceFixture struct {
	h      *authHandlers
	iss    *auth.Issuer
	q      *db.Queries
	userID int64
	client string
	tokens *fiber.App
	probe  *fiber.App
}

func newResourceFixture(t *testing.T, name string) *resourceFixture {
	t.Helper()
	pool := startPostgres(t)
	q := db.New(pool)
	f := &resourceFixture{q: q, client: "client-" + name, iss: auth.NewIssuer("test-secret", time.Hour)}
	f.userID = seedOAuthUser(t, pool, name+"@example.test")
	seedOAuthTestClient(t, q, f.client, []string{resourceCB})
	f.h = &authHandlers{queries: q, issuer: f.iss, frontendOrigin: testMCPOrigin}
	f.tokens = fiber.New(fiber.Config{ErrorHandler: RenderError})
	f.tokens.Post("/api/v1/oauth/token", f.h.OAuthToken)

	assistants := &assistantHandlers{queries: q}
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	f.probe = fiber.New(fiber.Config{ErrorHandler: RenderError})
	f.probe.Post("/api/v1/mcp", mcpAuthenticate(assistants, testMCPOrigin, mcpapp.MixedResourceMetadataPath, oauth2server.MCPResourcePath, true), ok)
	f.probe.Post("/api/v1/mcp/account", mcpAccountAuthenticate(assistants, testMCPOrigin), ok)
	return f
}

// authorize posts the consent decision with the given resource values (none when
// resources is empty) and returns the redirect the client would follow.
func (f *resourceFixture) authorize(t *testing.T, resources ...string) *url.URL {
	t.Helper()
	cookie, err := f.iss.Issue(f.userID, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	form := url.Values{
		"decision": {"allow"}, "client_id": {f.client}, "redirect_uri": {resourceCB},
		"state": {"xyz"}, "code_challenge": {challengeForVerifier(resourceVerif)}, "code_challenge_method": {"S256"},
		"resource": resources,
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := oauthAuthorizeApp(f.h, f.iss).Test(req)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("authorize status = %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	return loc
}

// exchange redeems code at /token, optionally sending a resource, and returns
// the status and decoded JSON body.
func (f *resourceFixture) exchange(t *testing.T, code string, resources ...string) (int, map[string]any) {
	t.Helper()
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {f.client},
		"redirect_uri": {resourceCB}, "code_verifier": {resourceVerif}, "resource": resources,
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.tokens.Test(req)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (f *resourceFixture) callMCP(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := f.probe.Test(req)
	if err != nil {
		t.Fatalf("mcp call: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// grantToken runs authorize + token and returns the access token.
func (f *resourceFixture) grantToken(t *testing.T, authorizeResource string, tokenResource string) string {
	t.Helper()
	var ar, tr []string
	if authorizeResource != "" {
		ar = []string{authorizeResource}
	}
	if tokenResource != "" {
		tr = []string{tokenResource}
	}
	code := f.authorize(t, ar...).Query().Get("code")
	if code == "" {
		t.Fatal("no code issued")
	}
	status, body := f.exchange(t, code, tr...)
	if status != fiber.StatusOK {
		t.Fatalf("token status = %d, body %v", status, body)
	}
	return body["access_token"].(string)
}

func TestOAuthResource_UnknownResourceAtAuthorizeRedirectsInvalidTarget(t *testing.T) {
	f := newResourceFixture(t, "res-auth-1")

	loc := f.authorize(t, "https://evil.example/api/v1/mcp")
	if got := loc.Query().Get("error"); got != "invalid_target" {
		t.Fatalf("error = %q, want invalid_target (Location %s)", got, loc)
	}
	if loc.Query().Get("state") != "xyz" || loc.Query().Get("code") != "" {
		t.Errorf("want state echoed and no code, got %s", loc)
	}

	// GET too: the screen is never shown for a resource we would refuse.
	q := validAuthorizeQuery(f.client, resourceCB) + "&resource=" + url.QueryEscape("https://evil.example/x")
	cookie, _ := f.iss.Issue(f.userID, testTokenVersion)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/oauth/authorize?"+q, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := oauthAuthorizeApp(f.h, f.iss).Test(req)
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound || !strings.Contains(resp.Header.Get("Location"), "error=invalid_target") {
		t.Fatalf("GET: status %d Location %q, want 302 invalid_target redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestOAuthResource_BindingRoundTripsAndIsEnforcedPerEndpoint(t *testing.T) {
	f := newResourceFixture(t, "res-bind-1")

	account := f.grantToken(t, resourceAccount+"/", "") // trailing slash tolerated
	mixed := f.grantToken(t, resourceMixed, "")
	legacy := f.grantToken(t, "", "")

	cases := []struct {
		name, token, path string
		want              int
	}{
		{"account grant at account", account, "/api/v1/mcp/account", 200},
		{"account grant at mixed", account, "/api/v1/mcp", 401},
		{"mixed grant at mixed", mixed, "/api/v1/mcp", 200},
		{"mixed grant at account", mixed, "/api/v1/mcp/account", 401},
		{"unbound grant at account", legacy, "/api/v1/mcp/account", 200},
		{"unbound grant at mixed", legacy, "/api/v1/mcp", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := f.callMCP(t, tc.path, tc.token)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == 401 {
				if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) || !strings.Contains(got, "resource_metadata=") {
					t.Errorf("WWW-Authenticate = %q, want invalid_token + resource_metadata", got)
				}
			}
		})
	}

	t.Run("REST still admits a bound grant", func(t *testing.T) {
		identity, err := apiKeys{f.q}.AuthenticateAPIKey(context.Background(), oauth2server.HashToken(account))
		if err != nil || identity.UserID != f.userID || !identity.OAuthGrant {
			t.Fatalf("identity = %+v, err %v; want the grant's user", identity, err)
		}
	})
}

func TestOAuthResource_TokenEndpoint(t *testing.T) {
	f := newResourceFixture(t, "res-token-1")

	t.Run("an unknown resource is invalid_target and does not burn the code", func(t *testing.T) {
		code := f.authorize(t).Query().Get("code")
		status, body := f.exchange(t, code, "https://evil.example/x")
		if status != fiber.StatusBadRequest || body["error"] != "invalid_target" {
			t.Fatalf("status %d body %v, want 400 invalid_target", status, body)
		}
		if status, _ := f.exchange(t, code); status != fiber.StatusOK {
			t.Fatalf("retry without the bad resource: status %d, want 200", status)
		}
	})

	t.Run("a resource contradicting the bound one is invalid_target", func(t *testing.T) {
		code := f.authorize(t, resourceMixed).Query().Get("code")
		status, body := f.exchange(t, code, resourceAccount)
		if status != fiber.StatusBadRequest || body["error"] != "invalid_target" {
			t.Fatalf("status %d body %v, want 400 invalid_target", status, body)
		}
	})

	t.Run("a matching resource is accepted", func(t *testing.T) {
		code := f.authorize(t, resourceMixed).Query().Get("code")
		if status, body := f.exchange(t, code, resourceMixed); status != fiber.StatusOK {
			t.Fatalf("status %d body %v, want 200", status, body)
		}
	})

	t.Run("a resource sent only at /token binds the grant", func(t *testing.T) {
		token := f.grantToken(t, "", resourceAccount)
		if got := f.callMCP(t, "/api/v1/mcp", token).StatusCode; got != 401 {
			t.Errorf("at /mcp: status %d, want 401", got)
		}
		if got := f.callMCP(t, "/api/v1/mcp/account", token).StatusCode; got != 200 {
			t.Errorf("at /mcp/account: status %d, want 200", got)
		}
	})
}
