package mcpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unsafe"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

func TestAnalyticsLogsSafeSearchQualityAndConversation(t *testing.T) {
	records := captureToolLogs(t)
	reader := readerThatFinds(t)
	reader.searchJobs = func(context.Context, SearchInput) (SearchJobsResult, error) {
		return SearchJobsResult{Jobs: []JobSummary{}, Total: 0, IgnoredParams: []string{"private-filter"}, Semantic: SemanticUnavailable}, nil
	}
	session := connect(t, reader)
	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: toolSearchJobs, Arguments: map[string]any{"query": "private-query@example.test"},
		Meta: mcp.Meta{"freehire.me/analytics": map[string]any{"conversation_id": "019a5000-0000-7000-8000-000000000001"},
			"io.modelcontextprotocol/clientInfo": map[string]any{"name": "Claude Code", "version": "private-version"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	start, finish := nextToolLog(t, records), nextToolLog(t, records)
	if start["conversation_id"] == nil || start["conversation_id"] != finish["conversation_id"] {
		t.Fatalf("missing conversation correlation: %v", finish)
	}
	if finish["client"] != "claude" || finish["returned_results"] != float64(0) || finish["ignored_filters"] != true || finish["semantic"] != "unavailable" {
		t.Fatalf("missing safe dimensions: %v", finish)
	}
	if n, ok := finish["response_bytes"].(float64); !ok || n <= 0 {
		t.Fatalf("response size missing: %v", finish)
	}
	encoded, _ := json.Marshal(finish)
	for _, private := range []string{"private-query", "private-filter", "private-version", "019a5000-0000-7000-8000-000000000001"} {
		if strings.Contains(string(encoded), private) {
			t.Errorf("leaked %s", private)
		}
	}
}

func TestOutputValidationFailureIsNotCountedAsSuccess(t *testing.T) {
	records := captureToolLogs(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	server.AddReceivingMiddleware(toolCallMiddleware("mixed"))
	AddObservedTool(server, "mixed", &mcp.Tool{Name: "invalid_output", OutputSchema: map[string]any{"type": "object", "required": []string{"count"}, "properties": map[string]any{"count": map[string]any{"type": "integer"}}}},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]any, error) {
			return nil, map[string]any{"count": "wrong"}, nil
		})
	ok := mcpToolCalls.WithLabelValues("mixed", "invalid_output", "success", "")
	failed := mcpToolCalls.WithLabelValues("mixed", "invalid_output", "error", "internal_error")
	beforeOK, beforeFailed := testutil.ToFloat64(ok), testutil.ToFloat64(failed)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "invalid_output", Arguments: map[string]any{}})
	if err == nil && !result.IsError {
		t.Fatal("invalid result accepted")
	}
	if testutil.ToFloat64(ok) != beforeOK || testutil.ToFloat64(failed)-beforeFailed != 1 {
		t.Fatal("final SDK failure was counted as a success")
	}
	nextToolLog(t, records)
	if finish := nextToolLog(t, records); finish["outcome"] != "error" {
		t.Fatalf("logged handler success before SDK validation: %v", finish)
	}
}

func TestConversationCorrelationRejectsFreeTextAndKeepsCallsSeparate(t *testing.T) {
	call := func(id string) *callMetadata {
		return metadataForCall(&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{
			"freehire.me/analytics":              map[string]any{"conversation_id": id},
			"io.modelcontextprotocol/clientInfo": map[string]any{"name": "private-client@example.test"},
		}}})
	}
	first := call("019a5000-0000-7000-8000-000000000001")
	if first.conversation == "" || first.conversation != call("019A5000-0000-7000-8000-000000000001").conversation {
		t.Fatal("a stable handle lost correlation")
	}
	if first.conversation == call("019a5000-0000-7000-8000-000000000002").conversation {
		t.Fatal("different conversations were merged")
	}
	for _, invalid := range []string{"", "private-goal@example.test", "00000000-0000-0000-0000-000000000000", "urn:uuid:019a5000-0000-7000-8000-000000000001"} {
		if got := call(invalid); got.conversation != "" || got.client != "other" {
			t.Fatalf("untrusted metadata retained: %+v", got)
		}
	}
}

func TestAcceptedProblemReportLogsCategoryWithoutGoal(t *testing.T) {
	records := captureToolLogs(t)
	session := connectAccountCatalogue(t, []assistant.Tool{{Name: "report_problem", Schema: map[string]any{"type": "object"}, Run: func(context.Context, int64, json.RawMessage) (any, error) { return map[string]any{"id": 1}, nil }}}, nil)
	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "report_problem", Arguments: map[string]any{"category": "missing_capability", "goal": "private-user-goal"}})
	if err != nil {
		t.Fatal(err)
	}
	nextToolLog(t, records)
	finish := nextToolLog(t, records)
	raw, _ := json.Marshal(finish)
	if finish["report_category"] != "missing_capability" || strings.Contains(string(raw), "private-user-goal") {
		t.Fatalf("missing safe report category: %s", raw)
	}
}

func TestClientAttributionUsesFirstRecognizedSignal(t *testing.T) {
	for _, tc := range []struct{ name, metadata, agent, want string }{
		{"generic metadata falls back", "Generic MCP Client", "Claude-User/1.0", "claude"},
		{"known metadata wins", "Codex", "Claude-User/1.0", "codex"},
		{"unrecognized header preserves other", "Generic MCP Client", "unrecognized", "other"},
		{"missing header preserves other", "Generic MCP Client", "", "other"},
		{"header alone", "", "Claude-User/1.0", "claude"},
		{"no signals", "", "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{
				"io.modelcontextprotocol/clientInfo": map[string]any{"name": tc.metadata},
			}}, Extra: &mcp.RequestExtra{Header: http.Header{"User-Agent": []string{tc.agent}}}}
			if got := metadataForCall(req).client; got != tc.want {
				t.Fatalf("client=%q, want %q", got, tc.want)
			}
		})
	}
}

// The User-Agent reaches clientFamily through Fiber's adaptor as a string over fasthttp's
// reused request buffer. A family that aliases it becomes a metric label whose bytes change
// under the registry: the next scrape then finds duplicates and /metrics answers 500.
func TestClientFamilyDoesNotAliasItsInput(t *testing.T) {
	buffer := []byte("codex-cli/1.0")
	got := clientFamily(unsafe.String(&buffer[0], len(buffer)))
	copy(buffer, "nodetest/9.99")
	if got != "codex" {
		t.Fatalf("family changed with the request buffer: %q", got)
	}
}

func TestClientAttributionFallsBackAfterGenericHandshake(t *testing.T) {
	records := captureToolLogs(t)
	server := httptest.NewServer(Handler(nil, "", readerThatFinds(t)))
	defer server.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "Generic MCP Client", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: withHeaders{http.Header{"User-Agent": []string{"Claude-User/1.0"}, AccountUserIDHeader: []string{"42"}}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_job", Arguments: map[string]any{"slug": "a-job"}})
	if err != nil {
		t.Fatal(err)
	}
	nextToolLog(t, records)
	if record := nextToolLog(t, records); record["client"] != "claude" {
		t.Fatalf("generic handshake hid client header: %v", record)
	}
}

func TestMultiMegabyteToolResultHasFiniteSizeBucket(t *testing.T) {
	bucketCount := func() uint64 {
		t.Helper()
		metric := &dto.Metric{}
		if err := mcpResponseBytes.WithLabelValues("mixed", "get_job").(prometheus.Histogram).Write(metric); err != nil {
			t.Fatal(err)
		}
		for _, bucket := range metric.GetHistogram().GetBucket() {
			if bucket.GetUpperBound() == 8<<20 {
				return bucket.GetCumulativeCount()
			}
		}
		return 0
	}
	before := bucketCount()
	reader := readerThatFinds(t)
	original := reader.jobDetail
	reader.jobDetail = func(ctx context.Context, slug string) (JobResult, error) {
		out, err := original(ctx, slug)
		out.Description = strings.Repeat("x", 2<<20)
		return out, err
	}
	result, err := connect(t, reader).CallTool(t.Context(), &mcp.CallToolParams{Name: "get_job", Arguments: map[string]any{"slug": "large-job"}})
	if err != nil || result.IsError {
		t.Fatalf("valid large result failed: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= 1<<20 || len(encoded) > 8<<20 {
		t.Fatalf("fixture result size=%d", len(encoded))
	}
	if bucketCount()-before != 1 {
		t.Fatal("large valid result has no finite size observation")
	}
}
