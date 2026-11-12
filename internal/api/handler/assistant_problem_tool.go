package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/problemreport"
)

func (h *assistantHandlers) problemMCPTools() []assistant.Tool {
	return []assistant.Tool{{
		Name:   "report_problem",
		Title:  "Report an operation problem",
		Effect: assistant.EffectAdds,
		Description: "Report a freehire operation you could not complete, a missing capability, or unexpected behavior for the team to triage. " +
			"Give concise summaries of the user's goal, expected result, and actual failure. Include a tool name and error code when known. " +
			"Do not include conversations, CV contents, credentials, tokens, personal contact details, or URL query strings. " +
			"A successful receipt means the report was recorded for review; it does not mean the issue is fixed or that a reply is guaranteed. " +
			"Repeated matching open reports for this account increase request_count. " +
			"Use only for a problem met while doing what the user asked, and for a missing capability only when the user asked for it. " +
			"Never file a test, sample or demonstration report: this tool needs no trial run. " +
			"Never report a failure of report_problem itself recursively.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category":   map[string]any{"type": "string", "enum": []string{"tool_error", "missing_capability", "other"}},
				"goal":       map[string]any{"type": "string", "description": "Intended goal, 1 to 500 characters after trimming."},
				"expected":   map[string]any{"type": "string", "description": "Expected result, 1 to 1000 characters after trimming."},
				"actual":     map[string]any{"type": "string", "description": "Observed failure, 1 to 2000 characters after trimming. Summarize errors without sensitive payloads."},
				"tool_name":  map[string]any{"type": "string", "description": "Optional tool identifier, up to 80 letters, digits, underscores, dots, colons or hyphens."},
				"error_code": map[string]any{"type": "string", "description": "Optional error code identifier, up to 80 characters."},
			},
			"required": []string{"category", "goal", "expected", "actual"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var input problemreport.Input
			if err := assistant.DecodeArgs(raw, &input); err != nil {
				return nil, err
			}
			if h.problems == nil {
				return nil, unavailable("problem reporting")
			}
			receipt, err := h.problems.Record(ctx, userID, input)
			if errors.Is(err, problemreport.ErrInvalid) {
				return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
			return receipt, err
		},
	}}
}
