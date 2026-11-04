package mcpapp

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
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

// A failed account call is reported with the server's own metadata only: the personal data
// a caller can put in its User-Agent, its client name, its Mcp-Session-Id or a tool's error
// text never leaves,
// and no $exception event is sent.
func TestFailedAccountCallSendsOnlyServerMetadata(t *testing.T) {
	sent := recordPostHog(t)
	const email = "jane@example.com"

	tool := assistant.Tool{
		Name:   "failing",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("applicant " + email + " answered 'visa: H1B'")
		},
	}
	server := httptest.NewServer(AccountHandler([]assistant.Tool{tool}, ""))
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

	got := sent()
	for _, want := range []string{`"$mcp_tool_call"`, `"$mcp_server_name":"freehire-account"`, `"$mcp_tool_name":"failing"`, `"$mcp_is_error":true`} {
		if !strings.Contains(got, want) {
			t.Errorf("PostHog payload lacks %s; got %s", want, got)
		}
	}
	// The server is stateless, so go-sdk ignores a caller-sent Mcp-Session-Id and the SDK
	// generates ses_<UUIDv7>. Were the header honoured (MCPGODEBUG=allowsessionsinstateless=1),
	// the session and distinct id would be a non-cryptographic hash of it: ses_ + 32 hex digits.
	minted := regexp.MustCompile(`"\$session_id":"(ses_[0-9a-f-]{36})"`).FindStringSubmatch(got)
	if minted == nil {
		t.Errorf("want a generated $session_id, not one derived from the caller; got %s", got)
	} else if !strings.Contains(got, `"distinct_id":"`+minted[1]+`"`) {
		t.Errorf("distinct_id is not the generated session %s; got %s", minted[1], got)
	}
	for _, leak := range []string{email, "H1B", `"$exception"`, `"$mcp_error_message"`, `"$mcp_client_user_agent"`, `"$mcp_client_name"`} {
		if strings.Contains(got, leak) {
			t.Errorf("PostHog payload carries %s; got %s", leak, got)
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
