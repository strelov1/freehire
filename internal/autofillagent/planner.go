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
- Where "combo" is true the field is a custom dropdown whose options are not
  visible yet. Include it, with an empty value, only if you expect the profile to
  answer it; the real options will be read from the page and you will be asked to
  pick one. Leave it out entirely if the profile plainly cannot answer it.
- For a native select, the value must be one of its listed options exactly.
- For a checkbox, the value is "true" or "false". A field carrying "options" is a
  group of checkboxes: the value is one of those options, or several separated by
  commas.
- A field may take part of a profile value (e.g. the city out of a location).
- Address each field by its label, copied exactly as given.

Respond with JSON: {"fills":[{"label":"...","value":"..."}]}`

// choosePrompt is asked once per widget, with the options the page actually
// offered. Kept separate from the planning prompt because the question is
// narrower — one question, a closed list — and a narrower question is answered
// more reliably. "Answer nothing" is spelled out as the expected outcome so that
// declining does not read as failure.
const choosePrompt = `You are answering ONE question on a job-application form by
picking from the options it offers.

You are given the question, the exact options available, and the candidate's
profile. Pick the single option the profile supports.

Rules:
- Return an option copied exactly from the list. Never return anything else.
- If the profile does not answer the question, return an empty choice —
  declining is the correct answer whenever nothing in the profile supports any
  of the options.
- A location option is supported by the candidate's location (a profile of
  "Berlin, Germany" supports the option "Germany"). A visa-sponsorship,
  notice-period, salary or relocation option is supported when the profile
  states the matching field (visa_sponsorship_needed, notice_period,
  desired_salary, willing_to_relocate) and that value is consistent with the
  option.
- Never infer a claim the profile does not state.

Respond with JSON: {"choice":"..."}`

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

// Choose picks one of the widget's real options. An unreadable answer, or one
// naming something outside the list, comes back as "" — the caller then reports
// the question as unanswered, which leaves it for the user rather than committing
// a guess.
func (p LLMPlanner) Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error) {
	if p.Client == nil {
		return "", fmt.Errorf("agent autofill is unavailable: no language model is configured")
	}
	user, err := json.Marshal(map[string]any{
		"question": question.Label,
		"required": question.Required,
		"options":  options,
		"profile":  profile,
	})
	if err != nil {
		return "", err
	}
	raw, err := p.Client.GenerateJSON(ctx, choosePrompt, string(user))
	if err != nil {
		return "", err
	}
	var answer struct {
		Choice string `json:"choice"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return "", fmt.Errorf("the model's choice is not valid JSON: %w", err)
	}
	return answer.Choice, nil
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
