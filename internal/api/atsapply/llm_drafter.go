package atsapply

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// draftSchemaName labels the shape in the provider's logs.
const draftSchemaName = "auto_apply_question_draft"

var (
	draftSchemaOnce sync.Once
	draftSchema     llmschema.Schema
	draftSchemaErr  error
)

func draftRequestSchema() (llmschema.Schema, error) {
	draftSchemaOnce.Do(func() {
		draftSchema, draftSchemaErr = llmschema.Of[draftAnswer]()
		if draftSchemaErr != nil {
			draftSchemaErr = fmt.Errorf("atsapply: build draft schema: %w", draftSchemaErr)
		}
	})
	return draftSchema, draftSchemaErr
}

// draftAnswer is the model's structured response. Grounded is the model's own admission
// that it found a basis for an answer — false (or a blank Answer) means "nothing to say",
// read by LLMDrafter as ok=false, never as an empty string standing in for an answer.
type draftAnswer struct {
	Answer   string `json:"answer"`
	Grounded bool   `json:"grounded"`
}

const draftSystemPrompt = `You answer ONE job-application question on behalf of a candidate.

Rules, all absolute:
- Use ONLY the facts listed under "What the candidate has stated" below. Never invent a
  fact, a company name, a skill, or a number that is not there.
- If nothing given lets you answer honestly, set "grounded" to false and leave "answer"
  empty. Doing this is a correct, expected outcome — it is not a failure.
- Never answer with an assumption about identity, demographics, compensation, or legal
  work status even if the question seems to ask for one; if the question needs any of
  those, treat it as ungroundable.
- Keep the answer to 1-3 sentences, plain text, in the candidate's own voice (first
  person), suitable for pasting directly into the application form's text box.

Respond with the given JSON schema only.`

// LLMDrafter is the real Drafter, over internal/llm.Client.
type LLMDrafter struct {
	client *llm.Client
}

// NewLLMDrafter wraps an already-bound client (internal/llmkey.Bind, per attempt, per
// candidate — see cmd/auto-apply's wiring). client may be nil, matching every other
// LLM-backed feature's "unconfigured deployment" convention.
func NewLLMDrafter(client *llm.Client) *LLMDrafter {
	return &LLMDrafter{client: client}
}

var _ Drafter = (*LLMDrafter)(nil)

func (d *LLMDrafter) Draft(ctx context.Context, question MergedField, grounding GroundingContext) (string, bool, error) {
	if d.client == nil {
		return "", false, nil
	}
	schema, err := draftRequestSchema()
	if err != nil {
		return "", false, err
	}

	raw, err := d.client.GenerateJSON(ctx, draftSystemPrompt, draftUserPrompt(question, grounding),
		llm.WithSchema(draftSchemaName, schema))
	if err != nil {
		return "", false, fmt.Errorf("atsapply: draft %q: %w", question.ID, err)
	}

	var out draftAnswer
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", false, fmt.Errorf("atsapply: decode draft response for %q: %w", question.ID, err)
	}
	if !out.Grounded || strings.TrimSpace(out.Answer) == "" {
		return "", false, nil
	}
	return out.Answer, true, nil
}

// draftUserPrompt lists the question and the candidate's own publishable facts. Options,
// where the field offers any, are listed so the model can answer IN one of them rather
// than free text the caller would then fail to match (matchOption still re-checks this;
// the prompt is a hint, not the enforcement).
func draftUserPrompt(question MergedField, grounding GroundingContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n", question.Label)
	if len(question.Options) > 0 {
		b.WriteString("This question offers exactly these choices — answer with one of them, verbatim:\n")
		for _, opt := range question.Options {
			fmt.Fprintf(&b, "- %s\n", opt.Label)
		}
	}

	b.WriteString("\nWhat the candidate has stated:\n")
	if len(grounding.Atoms) == 0 {
		b.WriteString("(nothing on record)\n")
	}
	for _, a := range grounding.Atoms {
		fmt.Fprintf(&b, "- %s", a.Claim)
		if a.Context != "" {
			fmt.Fprintf(&b, " (%s)", a.Context)
		}
		if len(a.Skills) > 0 {
			fmt.Fprintf(&b, " [skills: %s]", strings.Join(a.Skills, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}
