package mcpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// deferredTools are account tools served but left out of tools/list: each had under 50
// calls in 30 days (freehire_mcp_tool_calls_total, 2026-11-18) while every session paid for
// its schema. find_tools finds them and call_tool runs them. A direct call by name still
// works: freehire-cli and the npm freehire-mcp call unsubscribe and update_subscription so.
// Never defer a tool the instructions or a prompt name (TestNoNamedToolIsDeferred).
var deferredTools = map[string]bool{
	"unsave_job": true, "report_job": true, "update_subscription": true, "unsubscribe": true,
	"answer_bank_list": true, "answer_bank_save": true, "answer_bank_delete": true,
	"screening_answers_set": true, "auto_apply_retry": true, "auto_apply_cancel": true,
	// The experience bank and tailored CVs (2026-11-20): new here, served unlisted from the start.
	"experience_list": true, "experience_add_employment": true, "experience_add_achievement": true,
	"experience_update_employment": true, "experience_update_achievement": true,
	"experience_remove_achievement": true, "experience_remove_employment": true,
	"cv_list": true, "cv_get": true, "cv_context": true, "cv_tailor": true, "cv_edit": true, "cv_render": true,
}

// isDeferred also covers every interview-buddy tool: 13 of them, about one call a month.
func isDeferred(name string) bool { return deferredTools[name] || strings.HasPrefix(name, "buddy_") }

const (
	toolFindTools = "find_tools"
	toolCallTool  = "call_tool"
)

const accountDeferredInstructions = "Unlisted tools: find_tools, then call_tool. "

type findToolsInput struct {
	Query string `json:"query,omitempty" jsonschema:"Words describing the task, such as 'pause alert' or 'mock interview'. Empty lists every tool."`
}

type foundTool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
	ReadOnly    bool   `json:"read_only"`
}

type findToolsOutput struct {
	Tools []foundTool `json:"tools"`
}

type callToolInput struct {
	Name      string         `json:"name" jsonschema:"The tool's name."`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"The tool's arguments, matching its input_schema."`
}

// findDeferred returns the deferred tools, those matching the most query words first. With
// no query, or no match, it returns all of them: the set is small, and an empty answer would
// hide that a tool exists.
func findDeferred(tools []assistant.Tool, query string) []foundTool {
	words := strings.Fields(strings.ToLower(query))
	type hit struct {
		tool  assistant.Tool
		score int
	}
	var hits []hit
	for _, tool := range tools {
		if !isDeferred(tool.Name) {
			continue
		}
		text := strings.ToLower(tool.Name + " " + tool.Title + " " + tool.Description)
		score := 0
		for _, word := range words {
			if strings.Contains(text, word) {
				score++
			}
		}
		hits = append(hits, hit{tool, score})
	}
	if slices.ContainsFunc(hits, func(h hit) bool { return h.score > 0 }) {
		hits = slices.DeleteFunc(hits, func(h hit) bool { return h.score == 0 })
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	out := make([]foundTool, 0, len(hits))
	for _, h := range hits {
		out = append(out, foundTool{Name: h.tool.Name, Title: h.tool.Title, Description: h.tool.Description,
			InputSchema: h.tool.Schema, ReadOnly: h.tool.Effect == assistant.EffectReads})
	}
	return out
}

// addToolSearch serves find_tools and call_tool when this session is served a deferred tool.
// call_tool is never run as itself for a deferred target: deferMiddleware rewrites the call
// into one of the target, so the target's handler, logs and metrics run under its own name.
func addToolSearch(server *mcp.Server, tools []assistant.Tool) {
	served := map[string]bool{}
	for _, tool := range tools {
		if isDeferred(tool.Name) {
			served[tool.Name] = true
		}
	}
	if len(served) == 0 {
		return
	}
	addCatalogueTool(server, &mcp.Tool{
		Name: toolFindTools, Title: "Find account tools", Annotations: ReadOnly(),
		Description: "Account tools not in the tool list (alert edits and deletion, the screening answer bank, " +
			"the experience bank, tailored CVs, mock-interview buddies and others), each with its description and input schema, best matches to the query first.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in findToolsInput) (*mcp.CallToolResult, findToolsOutput, error) {
		return nil, findToolsOutput{Tools: findDeferred(tools, in.Query)}, nil
	})
	// Annotated for the worst tool it can run: a client should ask before each call.
	destructive, openWorld := true, true
	addCatalogueTool(server, &mcp.Tool{
		Name: toolCallTool, Title: "Run an account tool",
		Annotations: &mcp.ToolAnnotations{Title: "Run an account tool", DestructiveHint: &destructive, OpenWorldHint: &openWorld},
		Description: "Runs an account tool that is not in the tool list, by its name, with its arguments; returns that tool's result.",
	}, func(context.Context, *mcp.CallToolRequest, callToolInput) (*mcp.CallToolResult, any, error) {
		// Only a name or field the caller got wrong reaches here: a refusal, not a fault of ours.
		const reason = "this runs only the tools find_tools returns; a listed tool is called directly"
		result := errorResult(reason)
		result.Meta = mcp.Meta{diagnosticsMetaKey: map[string]any{"error_code": "invalid_request", reasonKey: reason}}
		return result, nil, nil
	})
	server.AddReceivingMiddleware(deferMiddleware(served))
}

// deferMiddleware hides the served deferred tools from tools/list and turns call_tool for one
// of them into a call of that tool. It must be added after ObserveServer, so it runs outside
// it and the observed call already carries the target's name.
func deferMiddleware(served map[string]bool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && method == "tools/call" && call.Params.Name == toolCallTool {
				var in struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				// A field the schema does not declare (a typo of "arguments") leaves the call as
				// call_tool, whose schema validation rejects it, rather than running the target with {}.
				decoder := json.NewDecoder(bytes.NewReader(call.Params.Arguments))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&in) == nil && served[in.Name] {
					call.Params.Name, call.Params.Arguments = in.Name, in.Arguments
					if len(in.Arguments) == 0 || string(in.Arguments) == "null" {
						call.Params.Arguments = json.RawMessage("{}")
					}
				}
			}
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			if list, ok := res.(*mcp.ListToolsResult); ok && method == "tools/list" {
				list.Tools = slices.DeleteFunc(list.Tools, func(tool *mcp.Tool) bool { return served[tool.Name] })
			}
			return res, err
		}
	}
}
