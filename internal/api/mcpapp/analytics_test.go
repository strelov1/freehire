package mcpapp

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	posthog "github.com/posthog/posthog-go"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// recordPostHog enables analytics against a fake ingestion host and returns a function that
// flushes the client and returns everything it sent.
func recordPostHog(t *testing.T) func() string {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip: %v", err)
				return
			}
			body = zr
		}
		raw, _ := io.ReadAll(body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ingest.Close)

	client, err := posthog.NewWithConfig("phc_test", posthog.Config{Endpoint: ingest.URL})
	if err != nil {
		t.Fatal(err)
	}
	EnableAnalytics(client)
	t.Cleanup(func() { serverAnalytics.Store(nil) })

	return func() string {
		if err := client.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(bodies, "\n")
	}
}

// A tool call on the public server reaches PostHog as $mcp_tool_call, named after this
// server, once the client is closed — which is what the server's shutdown does.
func TestToolCallIsReportedToPostHog(t *testing.T) {
	sent := recordPostHog(t)

	session := connect(t, readerThatFinds(t))
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolSearchJobs,
		Arguments: map[string]any{"query": "golang"},
	}); err != nil {
		t.Fatalf("call: %v", err)
	}

	got := sent()
	for _, want := range []string{`"$mcp_tool_call"`, `"$mcp_tool_name":"` + toolSearchJobs + `"`, `"$mcp_server_name":"freehire"`} {
		if !strings.Contains(got, want) {
			t.Errorf("PostHog payload lacks %s; got %s", want, got)
		}
	}
}

// A failed account call is reported, but its error text — the account's own data — is not:
// neither in the tool-call event nor as a $exception event.
func TestFailedAccountCallSendsNoErrorText(t *testing.T) {
	sent := recordPostHog(t)
	const secret = "applicant jane@example.com answered 'visa: H1B'"

	tool := assistant.Tool{
		Name:   "failing",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New(secret)
		},
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server, err := NewAccountServer([]assistant.Tool{tool}, 42, "").Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "failing", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatal("want an error result")
	}

	got := sent()
	if !strings.Contains(got, `"$mcp_tool_call"`) || !strings.Contains(got, `"$mcp_server_name":"freehire-account"`) {
		t.Errorf("the failed account call was not reported; got %s", got)
	}
	for _, leak := range []string{"jane@example.com", "H1B", `"$exception"`, `"$mcp_error_message"`} {
		if strings.Contains(got, leak) {
			t.Errorf("PostHog payload carries %s; got %s", leak, got)
		}
	}
}

// Without EnableAnalytics the server sends nothing and still answers.
func TestToolCallWithoutAnalytics(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolSearchJobs,
		Arguments: map[string]any{"query": "golang"},
	}); err != nil {
		t.Fatalf("call: %v", err)
	}
}
