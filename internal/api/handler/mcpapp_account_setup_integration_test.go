//go:build integration

// The guided setup over a real grant and a real Postgres: a CV-less account is sent to add a
// CV, adds one as text through the website's upload use case, and cannot replace it from the
// chat. Run with: go test -tags=integration ./internal/api/handler/ -run TestMCPAccountSetup
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func TestMCPAccountSetup_AddsAFirstCVAndRefusesAReplacement(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "mcp-setup@example.test")
	seedOAuthTestClient(t, queries, "client-mcp-setup", []string{"http://127.0.0.1:9999/callback"})
	version, err := queries.GetUserTokenVersion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	const token = "fhm_test-setup-token"
	expiresAt := time.Now().Add(time.Hour)
	if _, err := queries.CreateOAuthGrant(ctx, db.CreateOAuthGrantParams{
		UserID: userID, ClientID: "client-mcp-setup", AccessTokenHash: oauth2server.HashToken(token),
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatal(err)
	}

	blobs := newFakeResumeBlobs()
	store := resume.New(blobs, resume.NewQueriesRepository(queries))
	profiles := userprofile.New(userprofile.NewQueriesRepository(queries))
	assistants := &assistantHandlers{
		queries: queries,
		resume:  &resumeHandlers{resume: store},
		profile: &profileHandlers{userProfile: profiles, resume: store},
		plans:   plan.NewStore(queries, pool, plan.DefaultConfig()),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCPAccount(app.Group("/api/v1"), middleware{}, assistants, newMCPAppHandlers(nil, nil, queries, testMCPOrigin))
	server := httptest.NewServer(adaptor.FiberApp(app))
	defer server.Close()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx,
		&mcp.StreamableClientTransport{
			Endpoint:   server.URL + "/api/v1/mcp/account",
			HTTPClient: &http.Client{Transport: bearerRoundTripper{token: token}},
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res
	}
	status := func() setupStatus {
		t.Helper()
		res := call("get_setup_status", map[string]any{})
		if res.IsError {
			t.Fatalf("get_setup_status: %s", res.Content[0].(*mcp.TextContent).Text)
		}
		var out setupStatus
		if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	before := status()
	if before.NextStep.Tool != "add_cv" || before.Complete {
		t.Fatalf("a new account's setup = %+v, want next_step add_cv", before)
	}
	for _, name := range []string{"cv", "preferences", "alerts", "paid_plan", "agents_may_apply"} {
		stepNamed(t, before, name)
	}

	res := call("add_cv", map[string]any{"text": addCVText})
	if res.IsError {
		t.Fatalf("add_cv: %s", res.Content[0].(*mcp.TextContent).Text)
	}
	var added addCVResult
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &added); err != nil {
		t.Fatal(err)
	}
	if !added.Stored || len(added.Suggested.Skills) == 0 {
		t.Errorf("add_cv result = %+v, want stored with suggested skills", added)
	}
	if meta, err := store.Status(ctx, userID); err != nil || !meta.Present {
		t.Fatalf("stored CV status = %+v, %v; want present", meta, err)
	}
	// The background derivation has no extractor here and records a failure; wait for it so
	// the goroutine is done with the database before the test tears it down.
	deadline := time.Now().Add(5 * time.Second)
	for {
		row, err := store.StructuredRow(ctx, userID)
		if err == nil && resume.ResolveExtractStatus(row).Status == resume.ExtractStatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background CV derivation never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if after := status(); after.NextStep.Tool == "add_cv" {
		t.Errorf("after add_cv, next_step still asks for a CV: %+v", after.NextStep)
	}

	res = call("add_cv", map[string]any{"text": addCVText + " Replaced."})
	if !res.IsError {
		t.Fatal("a second add_cv replaced the stored CV")
	}
	if got := string(blobs.objs[blobstore.ResumeKey(userID)]); got != addCVText {
		t.Errorf("stored CV = %q, want the first text", got)
	}
}
