package ojcpmcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/strelov1/freehire/internal/api/ojcp"
)

func ojcpCalls(t *testing.T, tool, outcome, code string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "freehire_mcp_tool_calls_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["surface"] == "ojcp" && labels["tool"] == tool && labels["outcome"] == outcome && labels["error_code"] == code {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestOJCPCountsSuccessfulAndRejectedCallsWithoutPostHog(t *testing.T) {
	before := ojcpCalls(t, "search_jobs", "success", "")
	rejected := ojcpCalls(t, "unknown", "rejected", "invalid_request")
	session := connectOJCP(t, &recordingReader{})
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_jobs", Arguments: map[string]any{"query": "go"}})
	if err != nil || result.IsError {
		t.Fatalf("search failed: %v %v", result, err)
	}
	_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "private-missing-tool@example.test"})
	if ojcpCalls(t, "search_jobs", "success", "")-before != 1 || ojcpCalls(t, "unknown", "rejected", "invalid_request")-rejected != 1 {
		t.Fatal("OJCP calls are missing or duplicated")
	}
}

type missingJobReader struct{ recordingReader }

func (missingJobReader) JobDetail(context.Context, string) (ojcp.JobDetailResponse, error) {
	return ojcp.JobDetailResponse{}, NotFoundError{What: "posting"}
}

func TestOJCPNotFoundDoesNotBecomeAnInternalFailure(t *testing.T) {
	before := ojcpCalls(t, "get_job_detail", "error", "invalid_request")
	session := connectOJCP(t, &missingJobReader{})
	_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_job_detail", Arguments: map[string]any{"job_id": "gone"}})
	if err == nil {
		t.Fatal("not found lost its protocol error")
	}
	if ojcpCalls(t, "get_job_detail", "error", "invalid_request")-before != 1 {
		t.Fatal("protocol not-found classified as an internal failure")
	}
}
