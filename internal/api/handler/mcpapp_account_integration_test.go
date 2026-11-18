//go:build integration

// Integration tests for the signed-in MCP account server's HTTP wiring against
// a real Postgres: an unknown bearer token is rejected, and a live grant's
// token reaches the MCP server and runs a tool as the right user. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/engage/problemreport"
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

func TestMCPAuthenticatedEndpoint(t *testing.T) {
	const endpoint = "/api/v1/mcp"
	t.Run("oauth", func(t *testing.T) { testMCPEndToEnd(t, endpoint) })
	t.Run("database failure", func(t *testing.T) { testMCPInfraFailure(t, endpoint) })
	t.Run("api keys", func(t *testing.T) { testMCPByAPIKey(t, endpoint) })
}

func testMCPEndToEnd(t *testing.T, endpoint string) {
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
		queries:    queries,
		dictionary: &dictionaryHandlers{queries: queries},
		problems:   problemreport.New(queries),
		tracking:   &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))},
	}
	sink := &recordingSink{}
	catalogue := newMCPAppHandlers(nil, nil, queries, testMCPOrigin)
	catalogue.agentViews = sink
	seedApplyFormJob(t, queries, "mcp-viewer", "mcp-viewer")
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mountMCP(api, assistants, catalogue)

	server := httptest.NewServer(adaptor.FiberApp(app))
	defer server.Close()

	t.Run("an unknown token is rejected before any MCP framing", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+endpoint, nil)
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
			Endpoint:   server.URL + endpoint,
			HTTPClient: &http.Client{Transport: bearerRoundTripper{token: liveToken}},
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		session, err := client.Connect(context.Background(), transport, nil)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = session.Close() }()
		assertAccountCatalogue(t, session)
		assertMCPViewer(t, session, sink, userID)
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      suggestDictionaryTermToolName,
			Arguments: map[string]any{"kind": "specialization", "term": "Unlisted Quasar Specialist"},
		})
		if err != nil || res.IsError {
			t.Fatalf("dictionary call: %+v %v", res, err)
		}
		var submittedBy int64
		if err := pool.QueryRow(t.Context(), `SELECT submitted_by FROM dictionary_gaps WHERE kind='specialization'`).Scan(&submittedBy); err != nil {
			t.Fatal(err)
		}
		if submittedBy != userID {
			t.Fatalf("dictionary suggestion owner=%d want%d", submittedBy, userID)
		}
		res, err = session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "report_problem",
			Arguments: map[string]any{
				"category": "missing_capability", "goal": "Compare a requested unsupported field",
				"expected": "A matching filter", "actual": "No such filter is available",
			},
		})
		if err != nil || res.IsError {
			t.Fatalf("problem report call: %+v %v", res, err)
		}
		if err := pool.QueryRow(t.Context(), `SELECT user_id FROM problem_reports`).Scan(&submittedBy); err != nil {
			t.Fatal(err)
		}
		if submittedBy != userID {
			t.Fatalf("problem report owner=%d want%d", submittedBy, userID)
		}

		res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "my_jobs", Arguments: map[string]any{},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("my_jobs failed: %+v", res.Content)
		}
	})
	t.Run("a revoked generation cannot fall back to anonymous", func(t *testing.T) {
		if _, err := queries.BumpUserTokenVersion(t.Context(), userID); err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+liveToken)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "invalid_token") {
			t.Fatalf("revoked grant status=%d challenge=%s", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
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
// the exception log, which is why authentication now runs as middleware ahead of the
// adapted raw http.Handler rather than inside it.
func testMCPInfraFailure(t *testing.T, endpoint string) {
	pool := startPostgres(t)
	queries := db.New(pool)
	assistants := &assistantHandlers{
		queries:  queries,
		tracking: &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mountMCP(api, assistants, newMCPAppHandlers(nil, nil, queries, testMCPOrigin))

	pool.Close() // simulate the database going away, not an unknown token

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, endpoint, nil)
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
func testMCPByAPIKey(t *testing.T, endpoint string) {
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
	sink := &recordingSink{}
	catalogue := newMCPAppHandlers(nil, nil, queries, testMCPOrigin)
	catalogue.agentViews = sink
	seedApplyFormJob(t, queries, "mcp-viewer", "mcp-viewer")
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCP(app.Group("/api/v1"), assistants, catalogue)
	server := httptest.NewServer(adaptor.FiberApp(app))
	defer server.Close()

	status := func(token string) int {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+endpoint, nil)
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
		Endpoint:   server.URL + endpoint,
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: fullKey}},
	}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()
	assertAccountCatalogue(t, session)
	assertMCPViewer(t, session, sink, userID)

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
	for _, name := range []string{"auto_apply_start", "suggest_dictionary_term", "report_problem"} {
		if served[name] {
			t.Errorf("a full key is served OAuth-only tool %s", name)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_subscriptions", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_subscriptions failed: %+v", res.Content)
	}
}

func assertAccountCatalogue(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	served := map[string]bool{}
	for _, tool := range listed.Tools {
		served[tool.Name] = true
	}
	// /mcp serves the catalogue's own search_jobs, not the assistant's `filters` object.
	for _, tool := range listed.Tools {
		if schema, _ := json.Marshal(tool.InputSchema); tool.Name == "search_jobs" && !strings.Contains(string(schema), `"work_mode"`) {
			t.Errorf("search_jobs is not the catalogue's schema: %s", schema)
		}
	}
	for _, name := range []string{
		"search_jobs", "get_job", "get_company", "search_companies", "check_jobs_open",
		"similar_jobs", "salary_stats", "job_market_stats", "list_filter_values", "search", "fetch",
	} {
		if !served[name] {
			t.Errorf("authenticated endpoint is missing public tool %s", name)
		}
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_jobs_open", Arguments: map[string]any{"slugs": []string{"account-catalogue-missing-posting"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("catalogue call through authenticated mount failed: %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("catalogue content is %T, want JSON text", result.Content[0])
	}
	var states struct {
		Jobs []struct{ Status string }
	}
	if err := json.Unmarshal([]byte(text.Text), &states); err != nil {
		t.Fatal(err)
	}
	if len(states.Jobs) != 1 || states.Jobs[0].Status != "not_found" {
		t.Fatalf("catalogue state = %+v, want missing posting", states)
	}
}

func assertMCPViewer(t *testing.T, session *mcp.ClientSession, sink *recordingSink, userID int64) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_job", Arguments: map[string]any{"slug": "mcp-viewer"}})
	if err != nil || result.IsError {
		t.Fatalf("get_job for viewer recording: %+v %v", result, err)
	}
	if len(sink.got) != 1 || sink.got[0].Viewer != "user:"+strconv.FormatInt(userID, 10) || sink.got[0].Kind != agentview.KindDetail {
		t.Fatalf("MCP events=%+v, want one detail recorded as verified account %d", sink.got, userID)
	}
}
