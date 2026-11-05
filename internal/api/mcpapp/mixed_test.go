package mcpapp

import (
	"context"
	"encoding/json"
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
			ss, err := newMixedServer(tools, userID, testOrigin, readerThatFinds(t)).Connect(ctx, serverTransport, nil)
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
