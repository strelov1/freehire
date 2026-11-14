package mcpapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/observability"
)

func TestAccountToolFailureHasSafeDiagnostics(t *testing.T) {
	records := captureToolLogs(t)
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.SetUserValue("diagnostics.request_id", "server-request-123")
	tool := assistant.Tool{Name: "probe", Run: func(context.Context, int64, json.RawMessage) (any, error) {
		return nil, fmt.Errorf("private-query-token: %w", pgx.ErrNoRows)
	}}
	result, err := loggedTool("mixed", tool.Name, accountToolHandler(tool, 42))(ctx, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}})
	if err != nil || !result.IsError {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	start, finish := nextToolLog(t, records), nextToolLog(t, records)
	for _, record := range []map[string]any{start, finish} {
		if record["request_id"] != "server-request-123" {
			t.Errorf("request correlation missing: %v", record)
		}
	}
	if finish["error_code"] != "not_found" {
		t.Errorf("safe cause missing: %v", finish)
	}
	diagnostics, _ := result.Meta["freehire.me/diagnostics"].(map[string]any)
	if diagnostics["request_id"] != "server-request-123" || diagnostics["error_code"] != "not_found" {
		t.Errorf("client diagnostics missing: %v", result.Meta)
	}
	if raw, _ := json.Marshal(finish); strings.Contains(string(raw), "private-query-token") {
		t.Fatalf("error text leaked: %s", raw)
	}
}

func TestHTTPToolErrorCorrelatesWithResponseAndResolvedIdentity(t *testing.T) {
	records := captureToolLogs(t)
	tool := assistant.Tool{Name: "probe", Schema: map[string]any{"type": "object"}, Run: func(context.Context, int64, json.RawMessage) (any, error) {
		return nil, fiber.NewError(403, "private-permission-details")
	}}
	app := fiber.New()
	app.Use(observability.RequestIDs())
	app.All("/mcp", func(c *fiber.Ctx) error {
		// Stand in for resolved auth, keeping the real Fiber -> HTTP -> SDK path.
		observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "api_key", UserID: 42, KeyID: 73, Scope: "full"})
		c.Request().Header.Set(AccountUserIDHeader, "42")
		return c.Next()
	}, adaptor.HTTPHandler(Handler([]assistant.Tool{tool}, "", nil)))
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"probe","arguments":{}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("X-Request-ID", "client-controlled-secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP status = %d", resp.StatusCode)
	}
	id := resp.Header.Get("X-Request-ID")
	if id == "" || id == "client-controlled-secret" {
		t.Fatalf("untrusted request id: %q", id)
	}
	recorder := httptest.NewRecorder()
	if _, err := io.Copy(recorder.Body, resp.Body); err != nil {
		t.Fatal(err)
	}
	result := rpcResult(t, recorder)
	if result["isError"] != true {
		t.Fatalf("tool error hidden: %v", result)
	}
	meta, _ := result["_meta"].(map[string]any)
	diagnostics, _ := meta["freehire.me/diagnostics"].(map[string]any)
	if diagnostics["request_id"] != id || diagnostics["error_code"] != "permission_denied" {
		t.Errorf("incorrect response diagnostics: %v", diagnostics)
	}
	start, finish := nextToolLog(t, records), nextToolLog(t, records)
	for _, record := range []map[string]any{start, finish} {
		if record["request_id"] != id || record["key_id"] != float64(73) || record["credential_type"] != "api_key" {
			t.Errorf("lost context at HTTP adaptor: %v", record)
		}
		if raw, _ := json.Marshal(record); strings.Contains(string(raw), "client-controlled-secret") || strings.Contains(string(raw), "private-permission-details") {
			t.Fatalf("private data in log: %s", raw)
		}
	}
	if finish["outcome"] != "tool_error" || finish["error_code"] != "permission_denied" {
		t.Errorf("HTTP 200 hid failure: %v", finish)
	}
}
