package mcpapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

func TestStatelessServersDoNotAdvertiseListChanges(t *testing.T) {
	for name, handler := range staticServerHandlers(t) {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, statelessRequest(t.Context(), theScannersRequest, "server/discover"))
			result := rpcResult(t, rec)
			caps := result["capabilities"].(map[string]any)
			for _, list := range []string{"tools", "prompts", "resources"} {
				if cap, ok := caps[list].(map[string]any); ok && cap["listChanged"] == true {
					t.Errorf("%s advertises changes to its per-request list", list)
				}
			}
			if name == "mixed" {
				for _, list := range []string{"prompts", "resources"} {
					if caps[list] == nil {
						t.Errorf("%s capability is missing", list)
					}
				}
			}
			if caps["tools"] == nil {
				t.Error("tools capability is missing")
			}
		})
	}
}

func TestStatelessListSubscriptionCompletesWithoutWaiting(t *testing.T) {
	const body = `{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen","params":{` +
		`"notifications":{"toolsListChanged":true,"promptsListChanged":true,"resourcesListChanged":true},` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`
	for name, handler := range staticServerHandlers(t) {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			req := statelessRequest(ctx, body, "subscriptions/listen")
			parsed, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			req.URL = parsed
			req.RequestURI = ""
			req.Host = req.URL.Host
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			// The SDK acknowledges a subscription before waiting. Reading to EOF
			// distinguishes a completed request from an indefinitely idle stream.
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("subscription did not complete: %v", err)
			}
			var acknowledged, completed bool
			for line := range strings.SplitSeq(string(payload), "\n") {
				data, ok := strings.CutPrefix(line, "data:")
				if !ok {
					continue
				}
				var envelope struct {
					Params json.RawMessage `json:"params"`
					Method string          `json:"method"`
					Result json.RawMessage `json:"result"`
					Error  json.RawMessage `json:"error"`
				}
				if err := json.Unmarshal([]byte(data), &envelope); err != nil {
					t.Fatal(err)
				}
				if len(envelope.Error) != 0 {
					t.Fatalf("subscription error: %s", envelope.Error)
				}
				if envelope.Method == "notifications/subscriptions/acknowledged" {
					var params mcp.SubscriptionsAcknowledgedParams
					if err := json.Unmarshal(envelope.Params, &params); err != nil {
						t.Fatal(err)
					}
					n := params.Notifications
					if n.ToolsListChanged || n.PromptsListChanged || n.ResourcesListChanged || len(n.ResourceSubscriptions) != 0 {
						t.Fatalf("static server accepted subscriptions: %+v", n)
					}
					acknowledged = true
				}
				completed = completed || len(envelope.Result) != 0
			}
			if !acknowledged || !completed {
				t.Fatalf("acknowledged = %v, completed = %v", acknowledged, completed)
			}
		})
	}
}

func staticServerHandlers(t *testing.T) map[string]http.Handler {
	t.Helper()
	return map[string]http.Handler{
		"mixed": MixedHandler([]assistant.Tool{}, "", readerThatFinds(t)),
	}
}

func statelessRequest(ctx context.Context, body, method string) *http.Request {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	req.Header.Set(AccountUserIDHeader, "42")
	return req
}
