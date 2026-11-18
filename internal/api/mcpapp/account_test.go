package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// connectSignedIn serves tools to user 42 over the server /mcp mounts.
func connectSignedIn(t *testing.T, tools []assistant.Tool) *mcp.ClientSession {
	t.Helper()
	server := newServer(tools, 42, "", nil, false)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestSignedInServer_RunsToolAsTheResolvedUser(t *testing.T) {
	var gotUserID int64
	session := connectSignedIn(t, []assistant.Tool{{
		Name:        "whoami",
		Description: "test tool",
		Schema:      map[string]any{"type": "object"},
		Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
			gotUserID = userID
			return map[string]any{"user_id": userID}, nil
		},
	}})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "whoami", Arguments: map[string]any{}})
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

func TestSignedInServer_ToolErrorBecomesAnErrorResult(t *testing.T) {
	session := connectSignedIn(t, []assistant.Tool{{
		Name:        "failing",
		Description: "always fails",
		Schema:      map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("boom")
		},
	}})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "failing", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error result, got a success")
	}
}

// Handler trusts the user id the Fiber middleware (mountMCPServer in
// internal/api/handler) already validated and stamped onto the request.
func TestMixedHandler_ReadsUserIDFromTrustedHeader(t *testing.T) {
	var gotUserID int64
	tool := assistant.Tool{
		Name:   "whoami",
		Schema: map[string]any{"type": "object"},
		Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
			gotUserID = userID
			return map[string]any{"user_id": userID}, nil
		},
	}
	server := httptest.NewServer(Handler([]assistant.Tool{tool}, "", nil))
	defer server.Close()

	header := http.Header{AccountUserIDHeader: {"42"}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: headerRoundTripper{header}}}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "whoami", Arguments: map[string]any{}})
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

func TestMixedHandler_MalformedTrustedHeaderIsAnInternalError(t *testing.T) {
	// A malformed stamped id means the auth middleware was skipped or misconfigured: a wiring
	// bug, not a caller mistake, so it must read as 500, not as the caller's fault.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	req.Header.Set(AccountUserIDHeader, "not-a-number")
	rec := httptest.NewRecorder()
	Handler(nil, "", nil).ServeHTTP(rec, req)

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

// A read tool and a write tool must be told apart in the annotations a client sees: a client
// asks a person to confirm a write and may run a read without asking, so a missing hint
// turns every search into a confirmation prompt — and a wrong one skips the prompt on a write.
func TestSignedInServer_AnnotatesReadsAndWrites(t *testing.T) {
	noop := func(context.Context, int64, json.RawMessage) (any, error) { return nil, nil }
	session := connectSignedIn(t, []assistant.Tool{
		{Name: "reader", Description: "reads", Schema: map[string]any{"type": "object"}, Effect: assistant.EffectReads, Run: noop},
		{Name: "adder", Description: "adds", Schema: map[string]any{"type": "object"}, Effect: assistant.EffectAdds, Run: noop},
		{Name: "undeclared", Description: "declares nothing", Schema: map[string]any{"type": "object"}, Run: noop},
	})

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]*mcp.ToolAnnotations{}
	for _, tool := range listed.Tools {
		got[tool.Name] = tool.Annotations
	}

	reader := got["reader"]
	if reader == nil || !reader.ReadOnlyHint || reader.DestructiveHint == nil || *reader.DestructiveHint {
		t.Errorf("reader annotations = %+v, want readOnlyHint true and destructiveHint false", reader)
	}
	adder := got["adder"]
	if adder == nil || adder.ReadOnlyHint || adder.DestructiveHint == nil || *adder.DestructiveHint {
		t.Errorf("adder annotations = %+v, want readOnlyHint false and destructiveHint false", adder)
	}
	// A tool that declares nothing is announced as the most dangerous kind.
	undeclared := got["undeclared"]
	if undeclared == nil || undeclared.ReadOnlyHint || undeclared.DestructiveHint == nil || !*undeclared.DestructiveHint {
		t.Errorf("undeclared annotations = %+v, want readOnlyHint false and destructiveHint true", undeclared)
	}
	for name, a := range got {
		if a == nil || a.OpenWorldHint == nil {
			t.Errorf("%s: openWorldHint unset — OpenAI's review rejects a tool that leaves it out", name)
		}
	}
}

// A call the model got wrong is the caller's mistake, not a server fault: the metric that
// counts internal_error must not fill up with bad arguments.
func TestSignedInServer_InvalidArgumentsAreInvalidRequest(t *testing.T) {
	session := connectSignedIn(t, []assistant.Tool{{
		Name:   "strict",
		Schema: map[string]any{"type": "object"},
		Run: func(_ context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct{}
			return nil, assistant.DecodeArgs(raw, &in)
		},
	}})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "strict", Arguments: map[string]any{"bogus": 1}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	diag, _ := res.Meta["freehire.me/diagnostics"].(map[string]any)
	if !res.IsError || diag["error_code"] != "invalid_request" {
		t.Errorf("IsError=%v diagnostics=%v, want an invalid_request error", res.IsError, res.Meta)
	}
}
