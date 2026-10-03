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

func TestAccountHandler_RejectsMissingBearer(t *testing.T) {
	authenticate := func(context.Context, string) (int64, error) {
		t.Fatal("authenticate must not be called with no bearer token")
		return 0, nil
	}
	handler := mcpapp.AccountHandler(nil, authenticate)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAccountHandler_RejectsAnUnauthenticatedToken(t *testing.T) {
	authenticate := func(context.Context, string) (int64, error) {
		return 0, mcpapp.ErrUnauthorized
	}
	handler := mcpapp.AccountHandler(nil, authenticate)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer fhm_invalid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
