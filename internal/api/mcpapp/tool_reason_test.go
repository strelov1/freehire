package mcpapp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func reasonResult(reason string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Meta: mcp.Meta{diagnosticsMetaKey: map[string]any{"error_code": "permission_denied", reasonKey: reason}}}
}

func TestToolErrorLogsBoundedSingleLineReasonOnly(t *testing.T) {
	cases := []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{"collapsed", reasonResult("first line\n\tsecond\u200b line\x07"), "first line second line"},
		{"truncated", reasonResult(strings.Repeat("x", 500)), strings.Repeat("x", maxLoggedReasonRunes) + "..."},
		{"result text is never logged", &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "private-error-secret"}}}, ""},
		{"success", &mcp.CallToolResult{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records := captureToolLogs(t)
			observeTool(context.Background(), "mixed", "x")(tc.result, nil)
			nextToolLog(t, records)
			record := nextToolLog(t, records)
			got, present := record["error"].(string)
			if got != tc.want || (tc.want == "" && present) {
				t.Fatalf("error = %q (present %v), want %q", got, present, tc.want)
			}
		})
	}
}
