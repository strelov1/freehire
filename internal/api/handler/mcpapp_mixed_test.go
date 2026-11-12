package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/mcpapp"
)

func TestMixedMCPAnonymousIdentityBoundary(t *testing.T) {
	app := fiber.New()
	app.Post("/", mcpAuthenticate(&assistantHandlers{}, testMCPOrigin, mcpapp.MixedResourceMetadataPath, true), func(c *fiber.Ctx) error {
		if c.Get(mcpapp.AccountUserIDHeader) != "" {
			t.Error("external identity reached handler")
		}
		return c.SendStatus(204)
	})
	for _, authorization := range []string{"", "Basic forged"} {
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/", nil)
		req.Header.Set(mcpapp.AccountUserIDHeader, "11532")
		req.Header.Set("Cookie", "session=forged")
		req.Header.Set("Authorization", authorization)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := 204
		if authorization != "" {
			want = 401
		}
		if resp.StatusCode != want {
			t.Fatalf("auth %q status %d want %d", authorization, resp.StatusCode, want)
		}
	}
}

func TestMixedMCPAnonymousDiscoveryAndStepUp(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCP(app.Group("/api/v1"), middleware{}, &assistantHandlers{}, &mcpappHandlers{origin: testMCPOrigin})
	cases := []struct{ method, params string }{
		{"tools/list", `{}`},
		{"tools/call", `{"name":"my_jobs","arguments":{}}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+tc.method+`","params":`+tc.params+`}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		req.Header.Set(mcpapp.AccountUserIDHeader, "11532")
		req.Header.Set("Cookie", "session=forged")
		// ChatGPT's in-band challenge; other hosts get HTTP 401 (mcpapp.TestMixedAnonymousPersonalCallStatus).
		req.Header.Set("User-Agent", "openai-mcp/1.0.0")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d %s", tc.method, resp.StatusCode, body)
		}
		// Streamable HTTP may wrap the JSON-RPC response in SSE.
		payload := string(body)
		if _, after, ok := strings.Cut(payload, "data: "); ok {
			payload = strings.TrimSpace(after)
		}
		var envelope struct {
			Result struct {
				Tools   []json.RawMessage `json:"tools"`
				IsError bool              `json:"isError"`
				Meta    map[string]any    `json:"_meta"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if tc.method == "tools/list" && len(envelope.Result.Tools) != 54 {
			t.Fatalf("tools=%d want54", len(envelope.Result.Tools))
		}
		if tc.method == "tools/call" && (!envelope.Result.IsError || envelope.Result.Meta["mcp/www_authenticate"] == nil) {
			t.Fatalf("missing OAuth challenge: %s", body)
		}
	}
}
