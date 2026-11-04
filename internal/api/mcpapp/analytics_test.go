package mcpapp

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	posthog "github.com/posthog/posthog-go"
	"github.com/posthog/posthog-go/posthogmcp"
)

// A tool call on the public server reaches PostHog as $mcp_tool_call, named after this
// server, once the client is closed — which is what the server's shutdown does.
func TestToolCallIsReportedToPostHog(t *testing.T) {
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
	defer ingest.Close()

	client, err := posthog.NewWithConfig("phc_test", posthog.Config{Endpoint: ingest.URL})
	if err != nil {
		t.Fatal(err)
	}
	EnableAnalytics(posthogmcp.New(client))
	t.Cleanup(func() { analytics.Store(nil) })

	session := connect(t, readerThatFinds(t))
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolSearchJobs,
		Arguments: map[string]any{"query": "golang"},
	}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	sent := strings.Join(bodies, "\n")
	mu.Unlock()
	for _, want := range []string{`"$mcp_tool_call"`, `"$mcp_tool_name":"` + toolSearchJobs + `"`, `"$mcp_server_name":"freehire"`} {
		if !bytes.Contains([]byte(sent), []byte(want)) {
			t.Errorf("PostHog payload lacks %s; got %s", want, sent)
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
