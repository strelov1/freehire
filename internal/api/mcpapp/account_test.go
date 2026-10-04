package mcpapp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/mcpapp"
)

func TestNewAccountServer_RunsToolAsTheResolvedUser(t *testing.T) {
	var gotUserID int64
	tool := assistant.Tool{
		Name:        "whoami",
		Description: "test tool",
		Schema:      map[string]any{"type": "object"},
		Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
			gotUserID = userID
			return map[string]any{"user_id": userID}, nil
		},
	}
	server := mcpapp.NewAccountServer([]assistant.Tool{tool}, 42)
	if server == nil {
		t.Fatal("NewAccountServer returned nil")
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(context.Background(), serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "whoami", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool call failed: %+v", res.Content)
	}
	if gotUserID != 42 {
		t.Errorf("Run saw userID = %d, want 42", gotUserID)
	}
}

func TestNewAccountServer_ToolErrorBecomesAnErrorResult(t *testing.T) {
	tool := assistant.Tool{
		Name:        "failing",
		Description: "always fails",
		Schema:      map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("boom")
		},
	}
	server := mcpapp.NewAccountServer([]assistant.Tool{tool}, 1)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "failing", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error result, got a success")
	}
}

// AccountHandler no longer authenticates — that moved to a Fiber middleware in
// mountMCPAccount (internal/api/handler/mcpapp_account.go), which can report an
// infra failure through the normal Sentry/classify pipeline; a bare
// net/http.Handler has no such pipeline to report through. These two tests
// cover AccountHandler's own remaining contract: trust the resolved user id the
// middleware already validated and stamped onto the request.
func TestAccountHandler_ReadsUserIDFromTrustedHeader(t *testing.T) {
	var gotUserID int64
	tool := assistant.Tool{
		Name:   "whoami",
		Schema: map[string]any{"type": "object"},
		Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
			gotUserID = userID
			return map[string]any{"user_id": userID}, nil
		},
	}
	handler := mcpapp.AccountHandler([]assistant.Tool{tool})

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(mcpapp.AccountUserIDHeader, "42")

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: headerRoundTripper{req.Header}}}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "whoami", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool call failed: %+v", res.Content)
	}
	if gotUserID != 42 {
		t.Errorf("Run saw userID = %d, want 42", gotUserID)
	}
}

func TestAccountHandler_MissingTrustedHeaderIsAnInternalError(t *testing.T) {
	// Reaching AccountHandler with no resolved user id means the auth
	// middleware was skipped or misconfigured — a wiring bug, not a caller
	// mistake, so it must read as 500, not as the caller's fault.
	handler := mcpapp.AccountHandler(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

type headerRoundTripper struct{ header http.Header }

func (rt headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range rt.header {
		req.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(req)
}
