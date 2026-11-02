package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func reportJobTool(t *testing.T, h *assistantHandlers) func(string) error {
	t.Helper()
	tools := h.reportMCPTools()
	if len(tools) != 1 || tools[0].Name != reportJobToolName {
		t.Fatalf("reportMCPTools = %v, want exactly %s", tools, reportJobToolName)
	}
	return func(args string) error {
		_, err := tools[0].Run(context.Background(), 1, json.RawMessage(args))
		return err
	}
}

func TestReportJobTool_RefusesWithoutASlug(t *testing.T) {
	err := reportJobTool(t, &assistantHandlers{})(`{"reason":"spam"}`)
	if err == nil || !strings.Contains(err.Error(), "slug is required") {
		t.Fatalf("err = %v, want a slug-required refusal", err)
	}
}

// A deployment without the report queue must answer with a refusal the agent can relay,
// not a nil dereference inside the MCP handler.
func TestReportJobTool_RefusesWhenReportsAreNotWired(t *testing.T) {
	err := reportJobTool(t, &assistantHandlers{})(`{"slug":"x-1","reason":"spam"}`)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v, want a not-available refusal", err)
	}
}
