package mcpapp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// nextHints are the follow-up sentences a search_jobs and a get_job result carry in their
// `next` field. Agents pick tools from the person's request and follow hints in results, so
// this is where a person who only searches hears that freehire does more.
//
// One short sentence each, and the same text for every call of a session: the result is
// replayed into the model's context every turn, so it is stated once per response, never
// per job, and never grows with the result.
type nextHints struct{ search, job string }

// signedOutNext points a connection that cannot call personal tools at signing in, rather
// than naming tools it would be refused.
func signedOutNext(origin string) nextHints {
	hint := "Sign in to your freehire account in this connector to save jobs, get alerts for new " +
		"matches and prepare applications (setup: " + strings.TrimSuffix(origin, "/") + "/mcp)."
	return nextHints{search: hint, job: hint}
}

// signedInNext names only the follow-ups among the tools this session is actually served:
// an API key has no auto-apply, so its hint does not offer it.
func signedInNext(tools []assistant.Tool) nextHints {
	served := func(name string) bool {
		return slices.ContainsFunc(tools, func(t assistant.Tool) bool { return t.Name == name })
	}
	pick := func(items ...[2]string) string {
		var out []string
		for _, item := range items {
			if served(item[0]) {
				out = append(out, item[1])
			}
		}
		if len(out) == 0 {
			return ""
		}
		return "Next: " + strings.Join(out, "; ") + "."
	}
	return nextHints{
		search: pick(
			[2]string{"get_profile", "get_profile first if not read yet"},
			[2]string{"save_job", "save_job to keep a posting"},
			[2]string{"track_job", "track_job to follow its stage"},
			[2]string{"subscribe", "subscribe for alerts on this search"},
		),
		job: pick(
			[2]string{"save_job", "save_job to keep it"},
			[2]string{"track_job", "track_job to follow its stage"},
			[2]string{"auto_apply_start", "auto_apply_start to prepare an application for review"},
			[2]string{"subscribe", "subscribe for alerts on similar searches"},
		),
	}
}

// withNext adds the hint to a JSON-object tool result of the account server, whose
// search_jobs and get_job are the in-app assistant's untyped ones. Anything that is not a
// JSON object, or an empty hint, leaves the result untouched.
func withNext(h mcp.ToolHandler, hint string) mcp.ToolHandler {
	if hint == "" {
		return h
	}
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := h(ctx, req)
		if err != nil || res == nil || res.IsError || len(res.Content) != 1 {
			return res, err
		}
		text, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			return res, err
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(text.Text), &obj) != nil || obj == nil {
			return res, err
		}
		obj["next"], _ = json.Marshal(hint)
		if out, merr := json.Marshal(obj); merr == nil {
			text.Text = string(out)
		}
		return res, err
	}
}

// nextFor picks the hint for one of the two tools that carry it.
func (n nextHints) nextFor(tool string) string {
	switch tool {
	case toolSearchJobs:
		return n.search
	case toolGetJob:
		return n.job
	}
	return ""
}
