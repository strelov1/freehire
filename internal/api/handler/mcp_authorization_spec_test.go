package handler

// Black-box checks of the MCP authorization specification (2025-06-18 and
// 2025-11-25) against both MCP endpoints, over a real listener: the test follows
// the challenge a client would follow, nothing more. The official conformance
// suite only exercises MCP CLIENTS, so this is where the server side is pinned.
//
// What is deliberately not here, because it is already pinned elsewhere:
// invalid_token on a refused bearer (mcpapp_account_integration_test.go), the
// ChatGPT in-band challenge (mcpapp_mixed_test.go), and the per-host status table
// at the handler level (mcpapp.TestMixedAnonymousPersonalCallStatus).

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// specServer serves the real route tree (auth metadata routes and both MCP
// mounts) on a loopback port, with that port as the deployment origin. The
// SvelteKit app proxies /.well-known/* to the API in production; the shim below
// does the same (it fetches the internal path and relays the answer, status and
// content type included), with the mapping read from the SvelteKit route files.
func specServer(t *testing.T) (origin string, get func(url string) specResponse, post func(endpoint, userAgent, body string) specResponse) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin = "http://" + ln.Addr().String()

	app := fiber.New(fiber.Config{ErrorHandler: RenderError, DisableStartupMessage: true})
	passthrough := func(c *fiber.Ctx) error { return c.Next() }
	api := app.Group("/api/v1")
	auth := &authHandlers{frontendOrigin: origin}
	auth.register(api, middleware{cookie: passthrough, optionalCookie: passthrough}, newContract(contractHandlers{authHandlers: auth}))
	assistants := &assistantHandlers{}
	catalogue := &mcpappHandlers{origin: origin}
	mountMCP(api, middleware{}, assistants, catalogue)
	mountMCPAccount(api, middleware{}, assistants, catalogue)
	client := &http.Client{Timeout: 10 * time.Second}
	for wellKnown, internal := range wellKnownProxies(t) {
		app.Get(wellKnown, func(c *fiber.Ctx) error {
			req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, origin+internal, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			c.Status(resp.StatusCode).Set(fiber.HeaderContentType, resp.Header.Get(fiber.HeaderContentType))
			return c.Send(body)
		})
	}

	served := make(chan error, 1)
	go func() { served <- app.Listener(ln) }()
	t.Cleanup(func() {
		_ = app.Shutdown()
		if err := <-served; err != nil {
			t.Errorf("app.Listener: %v", err)
		}
	})

	get = func(url string) specResponse {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		return drain(t, client, req)
	}
	post = func(endpoint, userAgent, body string) specResponse {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, origin+endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		req.Header.Set("User-Agent", userAgent)
		return drain(t, client, req)
	}
	return origin, get, post
}

// specResponse is a response read to the end, so no test holds a body open.
type specResponse struct {
	URL        string
	StatusCode int
	Header     http.Header
	Body       []byte
}

func drain(t *testing.T, client *http.Client, req *http.Request) specResponse {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	return specResponse{URL: resp.Request.URL.String(), StatusCode: resp.StatusCode, Header: resp.Header, Body: body}
}

var proxyCall = regexp.MustCompile(`proxyWellKnownJSON\(\s*fetch,\s*'([^']+)'`)

// wellKnownProxies maps each /.well-known/... URL the SvelteKit app serves to the
// API path it proxies, by reading web/src/routes/.well-known.
func wellKnownProxies(t *testing.T) map[string]string {
	t.Helper()
	routes := filepath.Join("..", "..", "..", "web", "src", "routes")
	out := map[string]string{}
	err := filepath.Walk(filepath.Join(routes, ".well-known"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "+server.ts" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := proxyCall.FindSubmatch(src); m != nil {
			dir, err := filepath.Rel(routes, filepath.Dir(path))
			if err != nil {
				return err
			}
			out["/"+filepath.ToSlash(dir)] = string(m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/api/v1/mcp",
		"/.well-known/oauth-authorization-server",
	} {
		if out[want] == "" {
			t.Fatalf("no proxyWellKnownJSON route found for %s under web/src/routes (found %v); "+
				"the route is missing or its +server.ts no longer matches %s", want, out, proxyCall)
		}
	}
	return out
}

var resourceMetadataParam = regexp.MustCompile(`resource_metadata="([^"]+)"`)

const personalCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_profile","arguments":{}}}`

type resourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type serverMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

func decodeSpecJSON(t *testing.T, resp specResponse, into any) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: status %d, want 200", resp.URL, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s: Content-Type %q, want application/json", resp.URL, ct)
	}
	if err := json.Unmarshal(resp.Body, into); err != nil {
		t.Fatalf("%s: %v", resp.URL, err)
	}
}

// Authorization spec, "Authorization Server Location" (RFC 9728 §5.1 and
// RFC 6750 §3): a request without credentials is a 401 whose WWW-Authenticate
// carries resource_metadata; that document names the resource the client
// connected to and an authorization server; and that server's RFC 8414 metadata
// has what the "Authorization Flow Steps" need (PKCE S256, dynamic registration).
func TestMCPAuthorizationSpec_ChallengeLeadsToWorkingDiscovery(t *testing.T) {
	origin, get, post := specServer(t)
	cases := []struct {
		endpoint, wantMetadata string
		insufficientScope      bool
	}{
		// RFC 9728 §3.1: the canonical endpoint has a path-specific document.
		// A no-credential 401 carries error="insufficient_scope" here. RFC 6750 §3.1 says
		// a server SHOULD NOT include an error code when the request lacks authentication;
		// mixedChallenge (mcpapp/mixed.go) does so deliberately, for host compatibility
		// (ChatGPT's in-band challenge shares it). Asserted so a change is a conscious one.
		{"/api/v1/mcp", origin + "/.well-known/oauth-protected-resource/api/v1/mcp", true},
		// LEGACY DEVIATION from RFC 9728 §3.1/§3.3: the account alias is served from the
		// ROOT document, whose resource is .../api/v1/mcp/account rather than the root URL.
		// That works only because the client follows resource_metadata from the challenge
		// (RFC 9728 §5.1) instead of deriving the URL from the resource.
		{"/api/v1/mcp/account", origin + "/.well-known/oauth-protected-resource", false},
	}
	for _, tc := range cases {
		t.Run(tc.endpoint, func(t *testing.T) {
			resp := post(tc.endpoint, "codex-mcp-client/0.167.0", personalCall)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("anonymous personal call: status %d, want 401", resp.StatusCode)
			}
			challenge := resp.Header.Get("WWW-Authenticate")
			if !strings.HasPrefix(challenge, "Bearer ") {
				t.Fatalf("WWW-Authenticate = %q, want the Bearer scheme", challenge)
			}
			// RFC 6750 §3.1: no credentials, so no invalid_token; see the case comments
			// for the one endpoint that adds insufficient_scope on purpose.
			if strings.Contains(challenge, "invalid_token") {
				t.Fatalf("WWW-Authenticate = %q: no token was presented", challenge)
			}
			if tc.insufficientScope {
				if !strings.Contains(challenge, `error="insufficient_scope"`) {
					t.Fatalf("WWW-Authenticate = %q, want the documented insufficient_scope", challenge)
				}
			} else if strings.Contains(challenge, "error=") {
				t.Fatalf("WWW-Authenticate = %q, want no error= on a request without credentials", challenge)
			}
			m := resourceMetadataParam.FindStringSubmatch(challenge)
			if m == nil || m[1] != tc.wantMetadata {
				t.Fatalf("WWW-Authenticate = %q, want resource_metadata=%q", challenge, tc.wantMetadata)
			}

			var resource resourceMetadata
			decodeSpecJSON(t, get(m[1]), &resource)
			// RFC 9728 §3.3 (the resource must match the URL the metadata was derived
			// from) is not what holds for the account alias, see above. What the MCP
			// spec asks of a client is that the resource identify the server it is
			// connecting to, so that is what is asserted.
			if resource.Resource != origin+tc.endpoint {
				t.Errorf("resource = %q, want %q", resource.Resource, origin+tc.endpoint)
			}
			if len(resource.AuthorizationServers) == 0 {
				t.Fatal("authorization_servers is empty; a client could not pick a server")
			}

			// RFC 8414 §3: the metadata lives at the well-known path under the issuer.
			issuer := resource.AuthorizationServers[0]
			var as serverMetadata
			decodeSpecJSON(t, get(issuer+"/.well-known/oauth-authorization-server"), &as)
			if as.Issuer != issuer { // RFC 8414 §3.3
				t.Errorf("issuer = %q, want %q (it must equal the URL discovery started from)", as.Issuer, issuer)
			}
			for name, endpoint := range map[string]string{
				"authorization_endpoint": as.AuthorizationEndpoint,
				"token_endpoint":         as.TokenEndpoint,
				"registration_endpoint":  as.RegistrationEndpoint, // RFC 7591, offered by this server
			} {
				if !strings.HasPrefix(endpoint, issuer+"/") {
					t.Errorf("%s = %q, want a URL under the issuer", name, endpoint)
				}
			}
			// Authorization spec, "Authorization Code Protection": clients MUST use
			// PKCE S256 and MUST refuse a server that does not advertise it. The rest
			// of the assertions below are this server's POLICY, not RFC 8414 requirements.
			if !slices.Contains(as.CodeChallengeMethodsSupported, "S256") {
				t.Errorf("code_challenge_methods_supported = %v, want S256", as.CodeChallengeMethodsSupported)
			}
			if slices.Contains(as.CodeChallengeMethodsSupported, "plain") {
				t.Errorf("code_challenge_methods_supported = %v: policy is S256 only, plain gives PKCE no protection", as.CodeChallengeMethodsSupported)
			}
			if slices.Contains(as.GrantTypesSupported, "implicit") || slices.Contains(as.ResponseTypesSupported, "token") {
				t.Errorf("response_types %v / grant_types %v: policy forbids the implicit flow", as.ResponseTypesSupported, as.GrantTypesSupported)
			}
			if !slices.Contains(as.ResponseTypesSupported, "code") || !slices.Contains(as.GrantTypesSupported, "authorization_code") {
				t.Errorf("response_types %v / grant_types %v, want code / authorization_code", as.ResponseTypesSupported, as.GrantTypesSupported)
			}
			// Policy: public clients (no secret) are the only kind an MCP host is.
			if !slices.Contains(as.TokenEndpointAuthMethodsSupported, "none") {
				t.Errorf("token_endpoint_auth_methods_supported = %v, want none", as.TokenEndpointAuthMethodsSupported)
			}
		})
	}
}

// RFC 9728 §3.1 derives the metadata URL from the resource URL; the two documents
// must also stay distinguishable, since the root one identifies the account alias.
func TestMCPAuthorizationSpec_RootDocumentStillNamesTheAccountAlias(t *testing.T) {
	origin, get, _ := specServer(t)
	var root, canonical resourceMetadata
	decodeSpecJSON(t, get(origin+"/.well-known/oauth-protected-resource"), &root)
	decodeSpecJSON(t, get(origin+"/.well-known/oauth-protected-resource/api/v1/mcp"), &canonical)
	if root.Resource != origin+"/api/v1/mcp/account" {
		t.Errorf("root resource = %q, want the account alias", root.Resource)
	}
	if canonical.Resource != origin+"/api/v1/mcp" {
		t.Errorf("canonical resource = %q, want the canonical endpoint", canonical.Resource)
	}
}

// Authorization spec, "Error Handling" and the host notes in mcpapp/AGENTS.md:
// the same anonymous request is answered by host, and every 401 still carries the
// path-specific metadata pointer, through the real Fiber wiring.
func TestMCPAuthorizationSpec_AnonymousAnswerByHost(t *testing.T) {
	origin, _, post := specServer(t)
	const searchJobs = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_jobs","arguments":{"query":"Go"}}}`
	wantPointer := `resource_metadata="` + origin + `/.well-known/oauth-protected-resource/api/v1/mcp"`
	cases := []struct {
		name, agent, body string
		want              int
	}{
		{"other host, personal tool", "codex-mcp-client/0.167.0", personalCall, http.StatusUnauthorized},
		// Claude.ai and Grok open sign-in only at connect time, so even a public call is refused.
		{"claude.ai, public tool", "Claude-User", searchJobs, http.StatusUnauthorized},
		{"grok, public tool", "grok-connectors-manager/0.1.0", searchJobs, http.StatusUnauthorized},
		// ChatGPT's in-band challenge travels in a 200 result; it must not be a 401.
		{"chatgpt, personal tool", "openai-mcp/1.0.0", personalCall, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := post("/api/v1/mcp", tc.agent, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(resp.Header.Get("WWW-Authenticate"), wantPointer) {
				t.Errorf("WWW-Authenticate = %q, want %s", resp.Header.Get("WWW-Authenticate"), wantPointer)
			}
		})
	}
}

// The advertisement above is only worth anything if /authorize refuses what it
// does not advertise: PKCE downgrade protection (authorization spec, "Authorization
// Code Protection"). Refused before any client lookup, so no database is involved.
func TestMCPAuthorizationSpec_AuthorizeRefusesPlainPKCE(t *testing.T) {
	origin, get, _ := specServer(t)
	query := url.Values{
		"response_type": {"code"}, "client_id": {"c"}, "redirect_uri": {"http://127.0.0.1:9999/callback"},
		"state": {"s"}, "code_challenge": {"abc"}, "code_challenge_method": {"plain"},
	}
	resp := get(origin + "/api/v1/oauth/authorize?" + query.Encode())
	// 400 naming the method — not any 4xx: a missing or misrouted /authorize answers 404, and an
	// unknown client 400 for another reason; neither proves PKCE refused plain.
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(resp.Body), "code_challenge_method") {
		t.Fatalf("code_challenge_method=plain: status %d %s, want 400 refusing the method", resp.StatusCode, resp.Body)
	}
}
