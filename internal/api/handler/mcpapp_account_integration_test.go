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
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/application/jobtracking"
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
	mountMCPAccount(api, middleware{}, assistants)

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
	mountMCPAccount(api, middleware{}, assistants)

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
