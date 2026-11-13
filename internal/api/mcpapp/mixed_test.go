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

func TestMixedAuthenticationBeforeAccountExecution(t *testing.T) {
	for name, userID := range map[string]int64{"anonymous": 0, "oauth": 42} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			tools := []assistant.Tool{{Name: "my_jobs", Schema: map[string]any{"type": "object"}, Run: func(_ context.Context, uid int64, _ json.RawMessage) (any, error) {
				calls++
				if uid != 42 {
					t.Fatalf("wrong user %d", uid)
				}
				return map[string]any{"jobs": []string{}}, nil
			}}}
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			ctx := context.Background()
			ss, err := newMixedServer(tools, userID, testOrigin, readerThatFinds(t), false).Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = ss.Close() }()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(listed.Tools) != 12 {
				t.Fatalf("tools=%d, want 11 public + 1 account", len(listed.Tools))
			}
			for _, tool := range listed.Tools {
				schemes, err := json.Marshal(tool.Meta["securitySchemes"])
				if err != nil {
					t.Fatal(err)
				}
				want := `[{"type":"noauth"}]`
				if tool.Name == "my_jobs" {
					want = `[{"scopes":[],"type":"oauth2"}]`
				}
				if string(schemes) != want {
					t.Errorf("%s auth metadata=%s want%s", tool.Name, schemes, want)
				}
			}
			public, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_jobs", Arguments: map[string]any{"query": "Go"}})
			if err != nil || public.IsError {
				t.Fatalf("public call: %v %v", public, err)
			}
			if account, _ := public.Meta[accountMetaKey].(map[string]any); account["signed_in"] != (userID != 0) {
				t.Fatalf("widget account flag=%v want signed_in=%v", public.Meta, userID != 0)
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "my_jobs", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if userID == 0 {
				if !result.IsError || calls != 0 {
					t.Fatalf("unauthorized execution: result=%v calls=%d", result, calls)
				}
				challenge, ok := result.Meta["mcp/www_authenticate"].([]any)
				if !ok || len(challenge) != 1 || !strings.Contains(challenge[0].(string), "/.well-known/oauth-protected-resource/api/v1/mcp") {
					t.Fatalf("missing step-up challenge: %v", result.Meta)
				}
			} else if result.IsError || calls != 1 {
				t.Fatalf("authenticated call: %v calls=%d", result, calls)
			}
		})
	}
}

// Hosts other than ChatGPT open sign-in only on HTTP 401; ChatGPT reads the in-band result.
func TestMixedAnonymousPersonalCallStatus(t *testing.T) {
	tools := []assistant.Tool{{Name: "my_jobs", Schema: map[string]any{"type": "object"}, Run: func(context.Context, int64, json.RawMessage) (any, error) {
		t.Fatal("personal tool ran anonymously")
		return nil, nil
	}}}
	handler := MixedHandler(tools, testOrigin, readerThatFinds(t))
	cases := []struct {
		name, agent, tool string
		want              int
	}{
		{"other host personal", "codex-mcp-client/0.167.0", "my_jobs", http.StatusUnauthorized},
		{"chatgpt personal", "openai-mcp/1.0.0", "my_jobs", http.StatusOK},
		{"other host public", "codex-mcp-client/0.167.0", "search_jobs", http.StatusOK},
		// These hosts offer sign-in only while a connector is added, so even a public call asks.
		{"grok public", "grok-connectors-manager/0.1.0", "search_jobs", http.StatusUnauthorized},
		{"claude.ai public", "Claude-User", "search_jobs", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":{"query":"Go"}}}`
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", "2025-06-18")
			req.Header.Set("User-Agent", tc.agent)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want %d: %s", rec.Code, tc.want, rec.Body)
			}
			challenge := rec.Header().Get("WWW-Authenticate")
			if tc.want == http.StatusUnauthorized && !strings.Contains(challenge, MixedResourceMetadataPath) {
				t.Fatalf("challenge=%q", challenge)
			}
		})
	}
}

func TestMixedInstructions_AnonymousIsNotSentToGetProfile(t *testing.T) {
	tools := []assistant.Tool{{Name: "get_profile", Schema: map[string]any{"type": "object"}}}
	for userID, wantLine := range map[int64]bool{0: false, 42: true} {
		session := connectServer(t, newMixedServer(tools, userID, testOrigin, readerThatFinds(t), false))
		got := strings.Contains(session.InitializeResult().Instructions, "Call get_profile before the first search")
		if got != wantLine {
			t.Errorf("user %d: profile-first line present = %v, want %v", userID, got, wantLine)
		}
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
	srv := httptest.NewServer(MixedHandler(tools, testOrigin, readerThatFinds(t)))
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
	for name, headers := range map[string]headerTransport{
		"anonymous": {"User-Agent": "freehire-cli/1.4.0"},
		"signed in": {"User-Agent": "freehire-cli/1.4.0", AccountUserIDHeader: "42"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mixedInstructionsOver(t, headers); !strings.HasSuffix(got, cliSessionInstructions) {
				t.Fatalf("instructions do not end with the CLI sentence: %q", got)
			}
		})
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
