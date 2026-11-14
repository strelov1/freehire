//go:build integration

// Integration tests for RFC 8707 resource indicators on the OAuth server
// against a real Postgres: the resource is validated at /authorize and /token,
// bound to the grant, and enforced by the MCP endpoint. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

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
	pool   *pgxpool.Pool
	userID int64
	client string
	tokens *fiber.App
	probe  *fiber.App
}

func newResourceFixture(t *testing.T, name string) *resourceFixture {
	t.Helper()
	pool := startPostgres(t)
	q := db.New(pool)
	f := &resourceFixture{q: q, pool: pool, client: "client-" + name, iss: auth.NewIssuer("test-secret", time.Hour)}
	f.userID = seedOAuthUser(t, pool, name+"@example.test")
	seedOAuthTestClient(t, q, f.client, []string{resourceCB})
	f.h = &authHandlers{queries: q, issuer: f.iss, frontendOrigin: testMCPOrigin}
	f.tokens = fiber.New(fiber.Config{ErrorHandler: RenderError})
	f.tokens.Post("/api/v1/oauth/token", f.h.OAuthToken)

	assistants := &assistantHandlers{queries: q}
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	f.probe = fiber.New(fiber.Config{ErrorHandler: RenderError})
	f.probe.Post("/api/v1/mcp", mcpAuthenticate(assistants, testMCPOrigin), ok)
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

func (f *resourceFixture) callMCP(t *testing.T, path, token string) (int, http.Header) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := f.probe.Test(req)
	if err != nil {
		t.Fatalf("mcp call: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header
}

// grantRow reads the stored resource binding and last_used_at of token's grant.
func (f *resourceFixture) grantRow(t *testing.T, token string) (resource *string, lastUsed *time.Time) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT resource, last_used_at FROM oauth_grants WHERE access_token_hash = $1`,
		oauth2server.HashToken(token)).Scan(&resource, &lastUsed); err != nil {
		t.Fatalf("read grant: %v", err)
	}
	return resource, lastUsed
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

// /api/v1/mcp/account is an alias of /api/v1/mcp (connectors saved before #4539 still
// use it): asking for it issues a code bound to the one resource.
func TestOAuthResource_TheAccountAliasBindsTheOneResource(t *testing.T) {
	f := newResourceFixture(t, "res-account-1")
	token := f.grantToken(t, resourceAccount, "")
	if stored, _ := f.grantRow(t, token); stored == nil || *stored != oauth2server.MCPResourcePath {
		t.Fatalf("stored resource = %v, want %q", stored, oauth2server.MCPResourcePath)
	}
	if status, _ := f.callMCP(t, "/api/v1/mcp", token); status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestOAuthResource_BindingRoundTripsAndIsEnforced(t *testing.T) {
	f := newResourceFixture(t, "res-bind-1")

	mixed := f.grantToken(t, "HTTPS://"+strings.ToUpper(strings.TrimPrefix(testMCPOrigin, "https://"))+"/api/v1/mcp/", "") // case and trailing slash tolerated
	legacy := f.grantToken(t, "", "")
	for name, token := range map[string]string{"bound grant": mixed, "unbound grant": legacy} {
		t.Run(name, func(t *testing.T) {
			if status, _ := f.callMCP(t, "/api/v1/mcp", token); status != 200 {
				t.Fatalf("status = %d, want 200", status)
			}
		})
	}

	t.Run("the binding is stored as the path", func(t *testing.T) {
		if stored, _ := f.grantRow(t, mixed); stored == nil || *stored != oauth2server.MCPResourcePath {
			t.Errorf("stored resource = %v, want the path %q", stored, oauth2server.MCPResourcePath)
		}
	})

	t.Run("a grant bound to the removed account resource is refused and leaves no trace", func(t *testing.T) {
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE oauth_grants SET resource = '/api/v1/mcp/account' WHERE access_token_hash = $1`, oauth2server.HashToken(mixed)); err != nil {
			t.Fatalf("rebind grant: %v", err)
		}
		_, before := f.grantRow(t, mixed)
		status, header := f.callMCP(t, "/api/v1/mcp", mixed)
		if status != 401 {
			t.Fatalf("status = %d, want 401", status)
		}
		if got := header.Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) || !strings.Contains(got, "resource_metadata=") {
			t.Errorf("WWW-Authenticate = %q, want invalid_token + resource_metadata", got)
		}
		if _, after := f.grantRow(t, mixed); (before == nil) != (after == nil) || (before != nil && !before.Equal(*after)) {
			t.Errorf("last_used_at moved on a refused call: %v -> %v", before, after)
		}
	})

	t.Run("REST still admits a bound grant", func(t *testing.T) {
		bound := f.grantToken(t, resourceMixed, "")
		identity, err := apiKeys{f.q}.AuthenticateAPIKey(context.Background(), oauth2server.HashToken(bound))
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

	t.Run("the account alias matches the one resource at /token too", func(t *testing.T) {
		code := f.authorize(t, resourceMixed).Query().Get("code")
		if status, body := f.exchange(t, code, resourceAccount); status != fiber.StatusOK {
			t.Fatalf("status %d body %v, want 200", status, body)
		}
	})

	t.Run("a matching resource is accepted", func(t *testing.T) {
		code := f.authorize(t, resourceMixed).Query().Get("code")
		if status, body := f.exchange(t, code, resourceMixed); status != fiber.StatusOK {
			t.Fatalf("status %d body %v, want 200", status, body)
		}
	})

	t.Run("a resource sent only at /token binds the grant", func(t *testing.T) {
		token := f.grantToken(t, "", resourceMixed)
		if stored, _ := f.grantRow(t, token); stored == nil || *stored != oauth2server.MCPResourcePath {
			t.Errorf("stored resource = %v, want %q", stored, oauth2server.MCPResourcePath)
		}
		if got, _ := f.callMCP(t, "/api/v1/mcp", token); got != 200 {
			t.Errorf("at /mcp: status %d, want 200", got)
		}
	})
}

func multipartBody(t *testing.T, fields url.Values) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, vs := range fields {
		for _, v := range vs {
			if err := w.WriteField(k, v); err != nil {
				t.Fatalf("WriteField: %v", err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return w.FormDataContentType(), &buf
}

// `resource` is read from the urlencoded args only, so a multipart body would
// bind nothing and still succeed. Both endpoints refuse it instead.
func TestOAuthResource_MultipartBodiesAreRefused(t *testing.T) {
	f := newResourceFixture(t, "res-multipart-1")
	count := func(table string) int {
		var n int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}

	t.Run("consent POST", func(t *testing.T) {
		cookie, _ := f.iss.Issue(f.userID, testTokenVersion)
		ct, body := multipartBody(t, url.Values{
			"decision": {"allow"}, "client_id": {f.client}, "redirect_uri": {resourceCB}, "state": {"xyz"},
			"code_challenge": {challengeForVerifier(resourceVerif)}, "code_challenge_method": {"S256"}, "resource": {resourceMixed},
		})
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize", body)
		req.Header.Set("Content-Type", ct)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := oauthAuthorizeApp(f.h, f.iss).Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if n := count("oauth_authorization_codes"); n != 0 {
			t.Errorf("%d authorization code(s) issued, want none", n)
		}
	})

	t.Run("token request", func(t *testing.T) {
		code := f.authorize(t, resourceMixed).Query().Get("code")
		ct, body := multipartBody(t, url.Values{
			"grant_type": {"authorization_code"}, "code": {code}, "client_id": {f.client},
			"redirect_uri": {resourceCB}, "code_verifier": {resourceVerif},
		})
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", body)
		req.Header.Set("Content-Type", ct)
		resp, err := f.tokens.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != fiber.StatusBadRequest || out["error"] != "invalid_request" {
			t.Fatalf("status %d body %v, want 400 invalid_request", resp.StatusCode, out)
		}
		if n := count("oauth_grants"); n != 0 {
			t.Errorf("%d grant(s) created, want none", n)
		}
	})
}

// With a trailing-slash FRONTEND_ORIGIN the advertised resource must still be one
// a client can bind and use.
func TestOAuthResource_AdvertisedResourceBindsUnderTrailingSlashOrigin(t *testing.T) {
	f := newResourceFixture(t, "res-slash-1")
	f.h.frontendOrigin = testMCPOrigin + "/"
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/resource", f.h.OAuthProtectedResourceMetadata)
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/resource", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var doc protectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	token := f.grantToken(t, doc.Resource, "")
	if got, _ := f.callMCP(t, "/api/v1/mcp", token); got != 200 {
		t.Errorf("at /mcp: status %d, want 200", got)
	}
}
