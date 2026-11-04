package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

const suggestDictionaryTermToolName = "suggest_dictionary_term"

func (h *assistantHandlers) dictionaryMCPTools() []assistant.Tool {
	return []assistant.Tool{{
		Name:   suggestDictionaryTermToolName,
		Title:  "Suggest a dictionary term",
		Effect: assistant.EffectAdds,
		Description: "Request review of a missing specialization, skill or company industry, " +
			"as on freehire's dictionary page. This records a suggestion, not a new working filter " +
			"or a vacancy. Existing terms and aliases are refused. Repeated requests share the " +
			"existing queue entry and increase request_count; status is the entry's review state. " +
			"The account shares a limit of 20 requests per UTC day with the website. " +
			"Submit only when the user asks for this suggestion or agrees to its kind and term.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"specialization", "skill", "industry"},
					"description": "The dictionary to extend: a job specialization, skill, or company industry.",
				},
				"term": map[string]any{
					"type":        "string",
					"description": "The missing term in the user's words, up to 80 characters after trimming.",
				},
			},
			"required": []string{"kind", "term"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in dictionaryRecordRequest
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if userID <= 0 {
				return nil, errors.New("a signed-in account is required")
			}
			if h.dictionary == nil {
				return nil, errors.New("dictionary suggestions are not available")
			}
			return h.dictionary.recordTerm(ctx, userID, in)
		},
	}}
}
