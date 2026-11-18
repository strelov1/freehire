package mcpapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/strelov1/freehire/internal/ai/assistant"
)

// Local analytics must not retain personal fields from headers, names or error text.
func TestFailedAccountCallLogsOnlySafeMetadata(t *testing.T) {
	records := captureToolLogs(t)
	const email = "jane@example.com"

	tool := assistant.Tool{
		Name:   "failing",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("applicant " + email + " answered 'visa: H1B'")
		},
	}
	server := httptest.NewServer(Handler([]assistant.Tool{tool}, "", readerThatFinds(t)))
	t.Cleanup(server.Close)

	header := http.Header{}
	header.Set(AccountUserIDHeader, "42")
	header.Set("User-Agent", "agent for "+email)
	header.Set("Mcp-Session-Id", email)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client of " + email, Version: "0"}, nil).
		Connect(context.Background(), &mcp.StreamableClientTransport{
			Endpoint:   server.URL,
			HTTPClient: &http.Client{Transport: withHeaders{header}},
		}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "failing", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatal("want an error result")
	}
	// A caller-authored tool name must be normalized before it enters telemetry.
	_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "visa_H1B", Arguments: map[string]any{}})

	events := []map[string]any{}
	for range 4 {
		events = append(events, nextToolLog(t, records))
	}
	encoded, _ := json.Marshal(events)
	got := string(encoded)
	if events[1]["tool"] != "failing" || events[1]["outcome"] != "tool_error" || events[3]["tool"] != "unknown" || events[3]["outcome"] != "rejected" {
		t.Fatalf("missing local analytics: %s", got)
	}
	for _, leak := range []string{email, "H1B", "visa_H1B"} {
		if strings.Contains(got, leak) {
			t.Errorf("private payload leaked: %s", got)
		}
	}
}

// withHeaders adds fixed headers to every request the MCP client sends.
type withHeaders struct{ header http.Header }

func (w withHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for key, values := range w.header {
		r.Header[key] = values
	}
	return http.DefaultTransport.RoundTrip(r)
}
