package mcpapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// deferredSession serves my_jobs (listed) and two deferred tools, recording every account call.
func deferredSession(t *testing.T) (*mcp.ClientSession, *[]string) {
	t.Helper()
	var calls []string
	tool := func(name, description string) assistant.Tool {
		return assistant.Tool{Name: name, Description: description, Schema: map[string]any{"type": "object"},
			Run: func(_ context.Context, uid int64, args json.RawMessage) (any, error) {
				if uid != 42 {
					t.Fatalf("wrong user %d", uid)
				}
				calls = append(calls, name+" "+string(args))
				return map[string]any{"ok": true}, nil
			}}
	}
	tools := []assistant.Tool{
		tool("my_jobs", "Saved and tracked jobs."),
		tool("unsubscribe", "Stop one job alert by its id."),
		tool("buddy_join", "Join the mock-interview buddies."),
	}
	return connectServer(t, newServer(tools, 42, testOrigin, readerThatFinds(t), false)), &calls
}

func TestDeferredToolsAreNotListed(t *testing.T) {
	session, _ := deferredSession(t)
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	served := map[string]bool{}
	for _, tool := range listed.Tools {
		served[tool.Name] = true
	}
	for _, name := range []string{"my_jobs", toolFindTools, toolCallTool} {
		if !served[name] {
			t.Errorf("%s is not listed", name)
		}
	}
	for _, name := range []string{"unsubscribe", "buddy_join"} {
		if served[name] {
			t.Errorf("deferred %s is listed", name)
		}
	}
}

func TestFindToolsRanksTheServedDeferredTools(t *testing.T) {
	session, _ := deferredSession(t)
	find := func(query string) []string {
		t.Helper()
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: toolFindTools, Arguments: map[string]any{"query": query}})
		if err != nil || res.IsError {
			t.Fatalf("find_tools %q: %v %v", query, res, err)
		}
		var out findToolsOutput
		raw, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tool := range out.Tools {
			if tool.InputSchema == nil {
				t.Errorf("%s has no input schema", tool.Name)
			}
			names = append(names, tool.Name)
		}
		return names
	}
	if got := strings.Join(find("stop alert"), ","); got != "unsubscribe" {
		t.Errorf("stop alert: %s", got)
	}
	// No match, or no query, lists every deferred tool, so the agent always sees what exists.
	for _, query := range []string{"", "zzz"} {
		if got := strings.Join(find(query), ","); got != "unsubscribe,buddy_join" {
			t.Errorf("%q: %s", query, got)
		}
	}
}

func TestCallToolRunsADeferredToolUnderItsOwnName(t *testing.T) {
	session, calls := deferredSession(t)
	ctx := t.Context()
	counted := mcpToolCalls.WithLabelValues(surfaceMCP, "unsubscribe", "success", "")
	before := testutil.ToFloat64(counted)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolCallTool,
		Arguments: map[string]any{"name": "unsubscribe", "arguments": map[string]any{"id": 7}}})
	if err != nil || res.IsError {
		t.Fatalf("call_tool: %v %v", res, err)
	}
	if delta := testutil.ToFloat64(counted) - before; delta != 1 {
		t.Errorf("unsubscribe counted %v times, want 1", delta)
	}
	// Still callable directly: freehire-cli and the npm server call it by name.
	if res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "buddy_join", Arguments: map[string]any{}}); err != nil || res.IsError {
		t.Fatalf("direct call: %v %v", res, err)
	}
	if got := strings.Join(*calls, "|"); got != `unsubscribe {"id":7}|buddy_join {}` {
		t.Errorf("calls: %s", got)
	}
	// A listed tool, or one this session is not served, is not run through call_tool.
	for _, name := range []string{"my_jobs", "buddy_leave", "search_jobs"} {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolCallTool, Arguments: map[string]any{"name": name}})
		if err != nil || !res.IsError {
			t.Errorf("call_tool %s: %v %v, want a tool error", name, res, err)
		}
	}
	if len(*calls) != 2 {
		t.Errorf("refused calls ran: %v", *calls)
	}
}

func TestNoDeferredToolsMeansNoToolSearch(t *testing.T) {
	session := connectServer(t, newServer([]assistant.Tool{{Name: "my_jobs", Schema: map[string]any{"type": "object"}}},
		42, testOrigin, readerThatFinds(t), false))
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == toolFindTools || tool.Name == toolCallTool {
			t.Errorf("%s listed with nothing deferred", tool.Name)
		}
	}
}

// A tool the instructions or a prompt names must be listed, or the agent is told to call
// something it cannot see.
func TestNoNamedToolIsDeferred(t *testing.T) {
	var tools []assistant.Tool
	for _, name := range []string{"get_profile", "get_setup_status", "add_cv", "update_profile", "auto_apply_start",
		"auto_apply_get", "auto_apply_review", "get_plan", "subscribe", "list_subscriptions", "subscription_updates",
		"list_unread_mail", "suggest_dictionary_term", "report_problem", "my_jobs", "unsubscribe"} {
		tools = append(tools, assistant.Tool{Name: name})
	}
	text := accountInstructions(tools)
	for _, spec := range accountPrompts(tools) {
		text += " " + spec.render(nil)
	}
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return r != '_' && (r < 'a' || r > 'z') }) {
		if strings.Contains(word, "_") && isDeferred(word) {
			t.Errorf("%s is named in the instructions or a prompt but deferred", word)
		}
	}
	if !strings.Contains(text, toolFindTools) || !strings.Contains(text, toolCallTool) {
		t.Errorf("instructions do not name the tool search: %q", text)
	}
}
