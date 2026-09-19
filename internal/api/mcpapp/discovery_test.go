package mcpapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// The transport is STATELESS, and this file is why.
//
// Measured against production on 2026-09-18, from a packet capture of OpenAI's own scanner:
// it sends ONE request — `server/discover`, declaring protocol version 2026-07-28 — and
// then stops. That is the SEP-2575 lifecycle: no handshake, the version and client identity
// travel in `_meta` on every request.
//
// Under the SESSION-based transport the SDK answers that request, and its answer lists
// supportedVersions topping out at 2025-11-25 while marking the result complete. The SDK
// says why in a comment beside the code: the stateful StreamableHTTPHandler "cannot
// actually serve the new protocol", so it filters the version out — and a server that
// answers a question in a language it then says it does not speak is exactly what the
// portal reports as "MCP server/discover response was inconsistent".
//
// Stateless mode is the library's own answer, not a shim around it. It fits this surface
// exactly: four read-only tools, request in and answer out, no server-to-client call and
// nothing to remember between requests.

// theScannersRequest is the discover call captured from production, verbatim but for the
// client name. A fixture copied from the wire, so a future SDK change that breaks the real
// caller breaks this test rather than passing a shape only we send.
const theScannersRequest = `{"jsonrpc":"2.0","id":"openai-mcp-discover","method":"server/discover",` +
	`"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
	`"io.modelcontextprotocol/clientInfo":{"name":"openai-mcp","version":"1.0.0"},` +
	`"io.modelcontextprotocol/clientCapabilities":{"experimental":{"openai/visibility":{"enabled":true}},` +
	`"extensions":{"io.modelcontextprotocol/ui":{"mimeTypes":["text/html;profile=mcp-app"]}}}}}}`

func post(t *testing.T, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	Handler(readerThatFinds(t)).ServeHTTP(rec, req)
	return rec
}

// rpcResult pulls the JSON-RPC result out of a response, whichever framing it arrived in.
func rpcResult(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	payload := rec.Body.String()
	for _, line := range strings.Split(payload, "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			payload = after
			break
		}
	}

	var envelope struct {
		Result map[string]any  `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("not JSON-RPC: %v — %s", err, rec.Body.String())
	}
	if envelope.Error != nil {
		t.Fatalf("answered an error: %s", envelope.Error)
	}
	return envelope.Result
}

func TestDiscoveryAdmitsTheVersionTheCallerAsksFor(t *testing.T) {
	// The whole defect in one assertion. Answering `server/discover` while omitting the
	// caller's own protocol version from supportedVersions leaves it nowhere to go, and it
	// is reported as an inconsistent response rather than as an unsupported version.
	rec := post(t, theScannersRequest, map[string]string{
		"Mcp-Protocol-Version": "2026-07-28",
		"Mcp-Method":           "server/discover",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — %s", rec.Code, rec.Body.String())
	}

	result := rpcResult(t, rec)
	versions := make([]string, 0)
	for _, v := range result["supportedVersions"].([]any) {
		versions = append(versions, v.(string))
	}
	if !slices.Contains(versions, "2026-07-28") {
		t.Errorf("supportedVersions = %v, want the version the caller declared", versions)
	}
	if result["capabilities"] == nil {
		t.Error("no capabilities in the discover result")
	}
}

func TestAStatelessCallerListsToolsWithoutAHandshake(t *testing.T) {
	// The point of the lifecycle: one request, an answer, no session to carry. A caller that
	// never sends `initialize` must still be able to read what this server offers.
	rec := post(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{`+
		`"io.modelcontextprotocol/protocolVersion":"2026-07-28",`+
		`"io.modelcontextprotocol/clientInfo":{"name":"t","version":"1"},`+
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`,
		map[string]string{"Mcp-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/list"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — %s", rec.Code, rec.Body.String())
	}
	tools, ok := rpcResult(t, rec)["tools"].([]any)
	if !ok || len(tools) != 4 {
		t.Fatalf("got %d tools without a handshake, want 4", len(tools))
	}
}

func TestTheHandshakeLifecycleStillWorks(t *testing.T) {
	// Stateless does not mean the old clients are turned away: `initialize` is still
	// answered, which is what every MCP client shipping today sends first. Losing that
	// would take the server off the air for everyone in order to please one caller.
	rec := post(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"freehire"`) {
		t.Errorf("initialize did not reach the server: %s", rec.Body.String())
	}
}

func TestAHandshakeClientCanStillListToolsInOneGo(t *testing.T) {
	// The shape an ordinary client uses today: initialize, then tools/list. Under the
	// session transport the second call carried an Mcp-Session-Id; stateless issues none,
	// so the call must succeed without one — otherwise the change trades one broken caller
	// for every other.
	rec := post(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — %s", rec.Code, rec.Body.String())
	}
	tools, ok := rpcResult(t, rec)["tools"].([]any)
	if !ok || len(tools) != 4 {
		t.Fatalf("got %d tools, want 4 — %s", len(tools), rec.Body.String())
	}
}
