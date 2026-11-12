package mcpapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

func connectServer(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// nextOf calls a tool and returns the `next` of its structured or text JSON result.
func nextOf(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s: %v %+v", tool, err, res)
	}
	var body struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil {
		t.Fatal(err)
	}
	return body.Next
}

func namedTools(names ...string) []assistant.Tool {
	var tools []assistant.Tool
	for _, name := range names {
		tools = append(tools, assistant.Tool{
			Name: name, Schema: map[string]any{"type": "object"},
			Run: func(context.Context, int64, json.RawMessage) (any, error) {
				return map[string]any{"jobs": []string{}}, nil
			},
		})
	}
	return tools
}

func TestNextHint_OnTheCatalogueTools(t *testing.T) {
	signedIn := namedTools("get_profile", "save_job", "track_job", "subscribe", "auto_apply_start")

	anon := connectServer(t, newMixedServer(signedIn, 0, testOrigin, readerThatFinds(t)))
	got := nextOf(t, anon, "search_jobs", map[string]any{"query": "Go"})
	if !strings.Contains(got, "Sign in") || strings.Contains(got, "save_job") {
		t.Errorf("anonymous hint must point at signing in, not at tools it cannot call: %q", got)
	}

	user := connectServer(t, newMixedServer(signedIn, 42, testOrigin, readerThatFinds(t)))
	for tool, args := range map[string]map[string]any{
		"search_jobs": {"query": "Go"},
		"get_job":     {"slug": "senior-go-engineer-acme-abc123"},
	} {
		got := nextOf(t, user, tool, args)
		if !strings.Contains(got, "save_job") || !strings.Contains(got, "subscribe") || strings.Count(got, ".") != 1 {
			t.Errorf("%s: signed-in hint = %q, want one sentence naming save_job and subscribe", tool, got)
		}
	}
}

func TestNextHint_OnTheAccountServerFollowsServedTools(t *testing.T) {
	keyed := namedTools("search_jobs", "get_job", "save_job", "subscribe")
	session := connectAccountCatalogue(t, keyed, nil)
	got := nextOf(t, session, "get_job", map[string]any{})
	if !strings.Contains(got, "save_job") || strings.Contains(got, "auto_apply_start") {
		t.Errorf("a key without auto-apply must not be offered it: %q", got)
	}

	granted := connectAccountCatalogue(t, namedTools("search_jobs", "get_job", "save_job", "auto_apply_start"), nil)
	if got := nextOf(t, granted, "get_job", map[string]any{}); !strings.Contains(got, "auto_apply_start") {
		t.Errorf("a grant is offered auto-apply: %q", got)
	}
	// save_job alone must not drag track_job into the hint.
	if got := nextOf(t, session, "get_job", map[string]any{}); strings.Contains(got, "track_job") {
		t.Errorf("a session without track_job is offered it: %q", got)
	}
	// Only the two result-bearing tools carry a hint.
	other := connectAccountCatalogue(t, namedTools("my_jobs", "save_job"), nil)
	if got := nextOf(t, other, "my_jobs", map[string]any{}); got != "" {
		t.Errorf("my_jobs carries a hint: %q", got)
	}
}

// An anonymous answer shortened by the cap carries the cap notice, which already says to
// connect the account; the next hint stays out so the agent is told once.
func TestNextHint_NotRepeatedByTheAnonymousCapNotice(t *testing.T) {
	anon := connectServer(t, newMixedServer(nil, 0, testOrigin, readerThatFinds(t)))
	res, err := anon.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_jobs", Arguments: map[string]any{"query": "Go", "limit": 50}})
	if err != nil || res.IsError {
		t.Fatal(err, res)
	}
	var body struct{ Notice, Next string }
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil {
		t.Fatal(err)
	}
	if body.Notice == "" || body.Next != "" {
		t.Errorf("notice=%q next=%q, want the notice alone", body.Notice, body.Next)
	}
}
