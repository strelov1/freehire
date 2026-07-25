package autofillagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strelov1/freehire/internal/llm"
)

// systemPrompt tells the model its one job. The instructions about inventing and
// about comboboxes are guidance only — both are enforced downstream (groundedIn
// here, the combo skip in the extension), because a prompt is not a guarantee.
const systemPrompt = `You map a candidate's profile onto a job-application form.

You are given the form's fields (each with its label, type, and whether it is a
custom-widget combobox) and the candidate's profile. Return the values to write.

Rules:
- Only use values present in the profile. If a field has no basis in the profile,
  leave it out. Never invent, guess, or write a placeholder.
- Skip fields where "combo" is true: they are custom dropdowns that cannot be
  filled yet.
- For a native select, the value must be one of its listed options exactly.
- For a checkbox, the value is "true" or "false".
- A field may take part of a profile value (e.g. the city out of a location).
- Address each field by its label, copied exactly as given.

Respond with JSON: {"fills":[{"label":"...","value":"..."}]}`

// Planner backed by the LLM. Its client is nil when the LLM is unconfigured;
// Plan then reports the feature is off rather than silently filling nothing.
type LLMPlanner struct {
	Client *llm.Client
}

func (p LLMPlanner) Plan(ctx context.Context, fields []Field, profile Profile) ([]Fill, error) {
	if p.Client == nil {
		return nil, fmt.Errorf("agent autofill is unavailable: no language model is configured")
	}
	user, err := json.Marshal(map[string]any{"fields": fields, "profile": profile})
	if err != nil {
		return nil, err
	}
	raw, err := p.Client.GenerateJSON(ctx, systemPrompt, string(user))
	if err != nil {
		return nil, err
	}
	return parsePlan(raw)
}

// parsePlan reads the model's answer. Entries without a label are dropped rather
// than failing the whole run: a partly usable plan still fills most of the form.
func parsePlan(raw string) ([]Fill, error) {
	var answer struct {
		Fills []Fill `json:"fills"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, fmt.Errorf("the model's plan is not valid JSON: %w", err)
	}
	plan := make([]Fill, 0, len(answer.Fills))
	for _, fill := range answer.Fills {
		if fill.Label != "" {
			plan = append(plan, fill)
		}
	}
	return plan, nil
}
