package handler

import (
	"context"
	"encoding/json"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/screeninganswers"
	"github.com/strelov1/freehire/internal/vocab"
)

// assistantScreeningTools are registered under EVERY preset, the same rule the bank tools
// follow: a candidate mentions their notice period or salary expectation whenever it comes
// up in conversation, not on a schedule, so the only version of this feature that works is
// the one that is listening at the time.
func (h *assistantHandlers) assistantScreeningTools() []assistant.Tool {
	return []assistant.Tool{h.screeningAnswersSetTool()}
}

// screeningAnswersSetTool records the candidate's own answers to the screening questions
// that repeat across ATS application forms — work authorization, visa sponsorship, desired
// salary, notice period, willingness to relocate, 18+ — so the browser extension's
// autofill agent can use them later. It decodes straight into screeninganswers.Answers:
// the same wire shape the manual-edit endpoint accepts, so a field the call omits is a
// field Update leaves untouched, identically on both write paths.
func (h *assistantHandlers) screeningAnswersSetTool() assistant.Tool {
	return assistant.Tool{
		Name: "screening_answers_set",
		Description: "Record the candidate's own answer to one or more of the six screening questions " +
			"that repeat across job-application forms: which countries they are authorized to work in, " +
			"whether they need visa sponsorship, their desired salary, their notice period, whether they " +
			"are willing to relocate, and whether they are 18 or older. Call this the moment the candidate " +
			"states one of these facts, so it is available to fill their next application form. Only pass " +
			"fields the candidate actually stated — never guess or infer one. A field you leave out keeps " +
			"whatever was already recorded; it is not cleared.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"authorized_countries": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Countries the candidate is authorized to work in, as names or codes (e.g. \"Germany\", \"US\").",
				},
				"visa_sponsorship_needed": map[string]any{
					"type":        "boolean",
					"description": "Whether the candidate needs an employer to sponsor a work visa.",
				},
				"desired_salary_amount": map[string]any{
					"type":        "integer",
					"description": "The candidate's stated desired salary figure.",
				},
				"desired_salary_currency": map[string]any{
					"type":        "string",
					"description": "The three-letter ISO 4217 currency of the desired salary (e.g. \"USD\", \"EUR\").",
				},
				"desired_salary_period": map[string]any{
					"type":        "string",
					"enum":        vocab.SalaryPeriodValues,
					"description": "The period the desired salary figure is stated over.",
				},
				"notice_period_days": map[string]any{
					"type":        "integer",
					"description": "How many days' notice the candidate must give their current employer.",
				},
				"willing_to_relocate": map[string]any{
					"type":        "boolean",
					"description": "Whether the candidate is willing to relocate for a role.",
				},
				"age_18_or_older": map[string]any{
					"type":        "boolean",
					"description": "Whether the candidate is 18 years of age or older.",
				},
			},
			"additionalProperties": false,
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in screeninganswers.Answers
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			return h.screeningAnswers.Update(ctx, userID, in)
		},
	}
}
