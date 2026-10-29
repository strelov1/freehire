//go:build integration

// Integration tests for the signed-in MCP account server's HTTP wiring against
// a real Postgres: an unknown bearer token is rejected, and a live grant's
// token reaches the MCP server and runs a tool as the right user. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

type bearerRoundTripper struct{ token string }

func (rt bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return http.DefaultTransport.RoundTrip(req)
}

func TestMCPAccountEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "mcp-account-1@example.test")
	seedOAuthTestClient(t, queries, "client-mcp-1", []string{"http://127.0.0.1:9999/callback"})

	version, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	const liveToken = "fhm_test-live-token"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if _, err := queries.CreateOAuthGrant(context.Background(), db.CreateOAuthGrantParams{
		UserID: userID, ClientID: "client-mcp-1", AccessTokenHash: oauth2server.HashToken(liveToken),
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatalf("CreateOAuthGrant: %v", err)
	}

	assistants := &assistantHandlers{
		queries:  queries,
		tracking: &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mountMCPAccount(api, middleware{}, assistants, testMCPOrigin)

	server := httptest.NewServer(adaptor.FiberApp(app))
	defer server.Close()

	t.Run("an unknown token is rejected before any MCP framing", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/mcp/account", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Authorization", "Bearer fhm_no-such-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		// RFC 6750: a token that was presented and refused is invalid_token, which is
		// what tells a client to re-run the sign-in rather than retry the same token.
		if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) ||
			!strings.Contains(got, "resource_metadata=") {
			t.Fatalf("WWW-Authenticate = %q, want an invalid_token challenge naming resource_metadata", got)
		}
	})

	t.Run("a live token runs a tool as the right user", func(t *testing.T) {
		transport := &mcp.StreamableClientTransport{
			Endpoint:   server.URL + "/api/v1/mcp/account",
			HTTPClient: &http.Client{Transport: bearerRoundTripper{token: liveToken}},
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		session, err := client.Connect(context.Background(), transport, nil)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = session.Close() }()

		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "my_jobs", Arguments: map[string]any{},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("my_jobs failed: %+v", res.Content)
		}
	})
}

// TestMCPAccountAuthenticate_InfraFailureIsA500NotA401 is a regression test
// for a code-review finding: an unexpected failure resolving the bearer token
// (here, a closed connection pool — standing in for any DB/infra fault) must
// not be folded into the same "invalid token" answer a real unknown token
// gets. Those are different failures with different owners: one is the
// caller's mistake, the other is ours and belongs in the error inbox. Only a
// real Fiber handler returning a Go error gets reported through classify/
// Sentry, which is why authentication now runs as middleware ahead of the
// adapted raw http.Handler rather than inside it.
func TestMCPAccountAuthenticate_InfraFailureIsA500NotA401(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	assistants := &assistantHandlers{
		queries:  queries,
		tracking: &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mountMCPAccount(api, middleware{}, assistants, testMCPOrigin)

	pool.Close() // simulate the database going away, not an unknown token

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp/account", nil)
	req.Header.Set("Authorization", "Bearer fhm_whatever")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (an infra failure must not read as an invalid token)", resp.StatusCode)
	}
}

// A full-scope API key is let in — the local MCP package holds a key, not a grant — and is
// served the alert tools but not auto-apply; a narrow key is refused as insufficient scope.
func TestMCPAccountByAPIKey(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "mcp-account-key@example.test")
	if err := queries.SetUserEmailVerified(ctx, userID); err != nil {
		t.Fatalf("SetUserEmailVerified: %v", err)
	}
	const fullKey, cvKey = "fhk_test-full-key", "fhk_test-cv-key"
	for token, scope := range map[string]string{fullKey: auth.ScopeFull, cvKey: auth.ScopeCV} {
		if _, err := queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
			UserID: userID, Name: scope, TokenHash: auth.HashAPIKey(token), TokenPrefix: "fhk_test", Scope: scope,
		}); err != nil {
			t.Fatalf("CreateAPIKey(%s): %v", scope, err)
		}
	}

	assistants := &assistantHandlers{
		queries:  queries,
		tracking: &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCPAccount(app.Group("/api/v1"), middleware{}, assistants, testMCPOrigin)
	server := httptest.NewServer(adaptor.FiberApp(app))
	defer server.Close()

	status := func(token string) int {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/mcp/account", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := status(cvKey); got != http.StatusForbidden {
		t.Errorf("cv key: status = %d, want 403", got)
	}
	if got := status("fhk_no-such-key"); got != http.StatusUnauthorized {
		t.Errorf("unknown key: status = %d, want 401", got)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/api/v1/mcp/account",
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: fullKey}},
	}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	served := map[string]bool{}
	for _, tool := range listed.Tools {
		served[tool.Name] = true
	}
	if !served["subscribe"] || !served["list_subscriptions"] {
		t.Errorf("a full key is not served the alert tools: %v", served)
	}
	if served["auto_apply_start"] {
		t.Error("a full key is served auto_apply_start")
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_subscriptions", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_subscriptions failed: %+v", res.Content)
	}
}
