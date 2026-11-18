package mcpapp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// Every tool call is counted by surface, tool, outcome and error code, and timed, so Grafana
// sees MCP traffic without depending on any third-party quota.
func TestToolCallIsCountedAndTimed(t *testing.T) {
	ok := mcpToolCalls.WithLabelValues("mixed", toolSearchJobs, "success", "")
	failed := mcpToolCalls.WithLabelValues("mixed", toolGetJob, "tool_error", "not_found")
	before, beforeFailed := testutil.ToFloat64(ok), testutil.ToFloat64(failed)
	timedBefore := histogramCount(t, "mixed", toolSearchJobs)

	if _, err := connect(t, readerThatFinds(t)).CallTool(context.Background(), &mcp.CallToolParams{Name: toolSearchJobs, Arguments: map[string]any{"query": "golang"}}); err != nil {
		t.Fatalf("call: %v", err)
	}
	r := readerThatFinds(t)
	r.jobDetail = func(context.Context, string) (JobResult, error) { return JobResult{}, NotFoundError{What: "posting"} }
	if _, err := connect(t, r).CallTool(context.Background(), &mcp.CallToolParams{Name: toolGetJob, Arguments: map[string]any{"slug": "gone"}}); err != nil {
		t.Fatalf("call: %v", err)
	}

	if got := testutil.ToFloat64(ok) - before; got != 1 {
		t.Errorf("search_jobs success counted %v times, want 1", got)
	}
	if got := testutil.ToFloat64(failed) - beforeFailed; got != 1 {
		t.Errorf("get_job not_found counted %v times, want 1", got)
	}
	if got := histogramCount(t, "mixed", toolSearchJobs) - timedBefore; got != 1 {
		t.Errorf("search_jobs timed %v times, want 1", got)
	}
}

func histogramCount(t *testing.T, surface, tool string) uint64 {
	t.Helper()
	m := &dto.Metric{}
	if err := mcpToolDuration.WithLabelValues(surface, tool).(prometheus.Histogram).Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

// A call the SDK refuses before any handler runs — arguments failing the schema, a tool that
// does not exist — is counted once as rejected, and a caller-chosen name never becomes a label.
func TestRejectedToolCallIsCounted(t *testing.T) {
	invalid := mcpToolCalls.WithLabelValues("mixed", toolSearchJobs, "rejected", "invalid_request")
	unknown := mcpToolCalls.WithLabelValues("mixed", "unknown", "rejected", "invalid_request")
	ran := mcpToolCalls.WithLabelValues("mixed", toolSearchJobs, "success", "")
	beforeInvalid, beforeUnknown, beforeRan := testutil.ToFloat64(invalid), testutil.ToFloat64(unknown), testutil.ToFloat64(ran)

	session := connect(t, readerThatFinds(t))
	_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: toolSearchJobs, Arguments: map[string]any{"limit": "ten"}})
	_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "jane@example.com", Arguments: map[string]any{}})

	if got := testutil.ToFloat64(invalid) - beforeInvalid; got != 1 {
		t.Errorf("schema-refused search_jobs counted %v times, want 1", got)
	}
	if got := testutil.ToFloat64(unknown) - beforeUnknown; got != 1 {
		t.Errorf("unknown tool counted %v times as unknown, want 1", got)
	}
	if got := testutil.ToFloat64(ran) - beforeRan; got != 0 {
		t.Errorf("a refused call was also counted as a success %v times", got)
	}
	ch := make(chan prometheus.Metric, 1024)
	mcpToolCalls.Collect(ch)
	close(ch)
	for m := range ch {
		var out dto.Metric
		_ = m.Write(&out)
		for _, l := range out.GetLabel() {
			if l.GetName() == "tool" && l.GetValue() == "jane@example.com" {
				t.Error("a caller-chosen tool name became a label")
			}
		}
	}
}
