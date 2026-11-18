package mcpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

func TestMixedServesEveryToolToTheSignedInUser(t *testing.T) {
	calls := 0
	tools := []assistant.Tool{{Name: "my_jobs", Schema: map[string]any{"type": "object"}, Run: func(_ context.Context, uid int64, _ json.RawMessage) (any, error) {
		calls++
		if uid != 42 {
			t.Fatalf("wrong user %d", uid)
		}
		return map[string]any{"jobs": []string{}}, nil
	}}}
	session := connectServer(t, newServer(tools, 42, testOrigin, readerThatFinds(t), false))
	ctx := t.Context()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 12 {
		t.Fatalf("tools=%d, want 11 catalogue + 1 account", len(listed.Tools))
	}
	// No tool is public any more: the catalogue needs the same sign-in as the account.
	for _, tool := range listed.Tools {
		schemes, err := json.Marshal(tool.Meta["securitySchemes"])
		if err != nil {
			t.Fatal(err)
		}
		if want := `[{"scopes":[],"type":"oauth2"}]`; string(schemes) != want {
			t.Errorf("%s auth metadata=%s want %s", tool.Name, schemes, want)
		}
	}
	if res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_jobs", Arguments: map[string]any{"query": "Go"}}); err != nil || res.IsError {
		t.Fatalf("catalogue call: %v %v", res, err)
	}
	if res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "my_jobs", Arguments: map[string]any{}}); err != nil || res.IsError || calls != 1 {
		t.Fatalf("account call: %v %v calls=%d", res, err, calls)
	}
}

// The authentication middleware answers a request without a credential 401 before the
// handler; one that still arrives without an identity is a wiring error, not an anonymous
// session.
func TestMixedHandlerRefusesARequestWithNoIdentity(t *testing.T) {
	handler := Handler(nil, testOrigin, readerThatFinds(t))
	for name, header := range map[string]string{"missing": "", "malformed": "abc", "zero": "0"} {
		t.Run(name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_jobs","arguments":{"query":"Go"}}}`
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if header != "" {
				req.Header.Set(AccountUserIDHeader, header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d want 500: %s", rec.Code, rec.Body)
			}
		})
	}
}

// headerTransport stamps fixed headers on every request, as the freehire CLI's HTTP client
// does with its User-Agent and as the authentication middleware does with the account id.
type headerTransport map[string]string

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func mixedInstructionsOver(t *testing.T, headers headerTransport, names ...string) string {
	t.Helper()
	if len(names) == 0 {
		names = []string{"get_profile"}
	}
	var tools []assistant.Tool
	for _, name := range names {
		tools = append(tools, assistant.Tool{Name: name, Schema: map[string]any{"type": "object"}})
	}
	srv := httptest.NewServer(Handler(tools, testOrigin, readerThatFinds(t)))
	defer srv.Close()
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: headers}}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	return session.InitializeResult().Instructions
}

func TestMixedInstructions_TheCLIIsToldItIsOnTheShell(t *testing.T) {
	if len(cliSessionInstructions) > 256 {
		t.Fatalf("CLI sentence is %d chars, budget 256", len(cliSessionInstructions))
	}
	for _, want := range []string{"freehire search", "--json", "freehire cv render", "--out"} {
		if !strings.Contains(cliSessionInstructions, want) {
			t.Errorf("CLI sentence does not name %q: %s", want, cliSessionInstructions)
		}
	}
	got := mixedInstructionsOver(t, headerTransport{"User-Agent": "freehire-cli/1.4.0", AccountUserIDHeader: "42"})
	if !strings.HasSuffix(got, cliSessionInstructions) {
		t.Fatalf("instructions do not end with the CLI sentence: %q", got)
	}
}

func TestMixedInstructions_OtherHostsDoNotHearOfTheCLI(t *testing.T) {
	for _, agent := range []string{"", "openai-mcp/1.0.0", "codex-mcp-client/0.167.0", "Go-http-client/1.1"} {
		t.Run(agent, func(t *testing.T) {
			got := mixedInstructionsOver(t, headerTransport{"User-Agent": agent, AccountUserIDHeader: "42"})
			if strings.Contains(got, "freehire CLI") || strings.Contains(got, cliSessionInstructions) {
				t.Fatalf("instructions mention the CLI: %q", got)
			}
		})
	}
}

// The account paragraphs end in a full stop with no trailing space when report_problem is
// served (every OAuth session), so the CLI sentence must bring its own separator.
func TestMixedInstructions_TheCLISentenceIsASeparateSentence(t *testing.T) {
	got := mixedInstructionsOver(t, headerTransport{"User-Agent": "freehire-cli/1.4.0", AccountUserIDHeader: "42"},
		"get_profile", "report_problem")
	if !strings.HasSuffix(got, ". "+cliSessionInstructions) {
		t.Fatalf("CLI sentence is not separated from the one before it: %q", got[max(0, len(got)-len(cliSessionInstructions)-40):])
	}
}
