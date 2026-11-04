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
	ok := mcpToolCalls.WithLabelValues("public", toolSearchJobs, "success", "")
	failed := mcpToolCalls.WithLabelValues("public", toolGetJob, "tool_error", "not_found")
	before, beforeFailed := testutil.ToFloat64(ok), testutil.ToFloat64(failed)
	timedBefore := histogramCount(t, "public", toolSearchJobs)

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
	if got := histogramCount(t, "public", toolSearchJobs) - timedBefore; got != 1 {
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
