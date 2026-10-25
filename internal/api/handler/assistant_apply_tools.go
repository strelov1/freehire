package handler

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// The apply conversation's tools (openspec/changes/auto-apply-chat): the MCP server's
// auto-apply tools, bound to the one application the session is held against.
//
// They are the same tools rather than a second set, so the rules that keep an application
// safe — answers are the candidate's, one review decision, never retry what may already
// have reached the employer — live in one place. Two things differ, and both come from
// binding:
//   - the session names the attempt, so the model is not asked for a job and cannot name
//     another one;
//   - the agents_may_apply switch does not apply to a turn the person sends from their own
//     browser (the session cookie): that is the same authority as the drawer's buttons. A
//     turn sent with an API key is an agent acting outside the browser, and the switch holds
//     for it exactly as on the MCP server — see withPersonInBrowser.
func (h *assistantHandlers) assistantApplyTools(jobID int64) []assistant.Tool {
	slugOf := func(ctx context.Context) (string, error) {
		job, err := h.queries.GetJob(ctx, jobID)
		if err != nil {
			return "", err
		}
		return job.PublicSlug, nil
	}
	return []assistant.Tool{
		bindToJob(h.autoApplyGetTool(), slugOf),
		bindToJob(h.autoApplyReviewTool(), slugOf),
		bindToJob(h.autoApplyRetryTool(), slugOf),
		bindToJob(h.autoApplyCancelTool(), slugOf),
		h.answerBankListTool(),
		h.answerBankSaveTool(),
		h.screeningAnswersGetTool(),
	}
}

// personInBrowserKey marks an assistant turn the person sent from the website itself,
// authenticated by its session cookie — where requireAgentsMayApply has nothing to protect,
// since that switch exists for agents acting outside the person's browser. streamSSE sets it
// from auth.ViaCookie; an API key or the MCP server never does.
type personInBrowserKey struct{}

func withPersonInBrowser(ctx context.Context) context.Context {
	return context.WithValue(ctx, personInBrowserKey{}, true)
}

func agentsMayApplyWaived(ctx context.Context) bool {
	inBrowser, _ := ctx.Value(personInBrowserKey{}).(bool)
	return inBrowser
}

// bindToJob fixes a job_slug tool to the session's job: job_slug leaves the schema, and the
// session's slug is written over whatever the model sends.
func bindToJob(tool assistant.Tool, slugOf func(context.Context) (string, error)) assistant.Tool {
	tool.Schema = withoutJobSlug(tool.Schema)
	run := tool.Run
	tool.Run = func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
		slug, err := slugOf(ctx)
		if err != nil {
			return nil, toolError(err)
		}
		var args map[string]any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, err
			}
		}
		// "null" decodes to a nil map, and a tool with no arguments left is often called so.
		if args == nil {
			args = map[string]any{}
		}
		args["job_slug"] = slug
		bound, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		return run(ctx, userID, bound)
	}
	return tool
}

// withoutJobSlug copies a JSON schema with job_slug removed from its properties and its
// required list. The original is shared with the MCP server's copy of the tool, so it is
// never edited in place.
func withoutJobSlug(schema map[string]any) map[string]any {
	out := maps.Clone(schema)
	if props, ok := schema["properties"].(map[string]any); ok {
		props = maps.Clone(props)
		delete(props, "job_slug")
		out["properties"] = props
	}
	if required, ok := schema["required"].([]string); ok {
		out["required"] = slices.DeleteFunc(slices.Clone(required), func(r string) bool { return r == "job_slug" })
	}
	return out
}
