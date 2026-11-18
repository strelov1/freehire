package atsapply

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/jev"
)

const (
	// jevSelectAnswerKey names the single question this file ever asks Jev.
	jevSelectAnswerKey = "answer"
	// jevSelectDeclineKey covers both "nothing supports any option" and "categorically
	// excluded topic", since the caller only needs answered or not. Its wording cannot collide
	// with a real option label, as with jevDeclineKey (autofillagent/jev.go).
	jevSelectDeclineKey = "none of these / not stated"
)

// jevSelectInstructions mirrors draftSystemPrompt's rules (llm_drafter.go) for a single
// select question. The categorical exclusion travels with every call as a second safeguard
// in case the draftable sensitive/geography label gate missed a compensation or
// eligibility question.
func jevSelectInstructions(label string) string {
	return fmt.Sprintf(`You answer ONE job-application question on behalf of a candidate, by
picking the single option the candidate's own stated facts support.

Rules, all absolute:
- Use ONLY the facts listed under "What the candidate has stated". Never invent a fact.
- If nothing given lets you answer honestly, pick %q — this is a correct, expected
  outcome, not a failure.
- Never answer with an assumption about identity, demographics, compensation, or legal
  work status even if a stated fact seems to suggest one — treat every question in those
  categories as ungroundable regardless of what else is stated, and pick %q.

Question: %s`, jevSelectDeclineKey, jevSelectDeclineKey, label)
}

type jevSelectChooser struct{ client *jev.Client }

// newJevSelectChooser takes jev.WithBaseURL to point at a test server instead of the live API.
func newJevSelectChooser(apiKey string, opts ...jev.Option) *jevSelectChooser {
	return &jevSelectChooser{client: jev.New(apiKey, opts...)}
}

// Choose asks Jev to pick one of options for question, grounded in facts. ok=false for the
// decline sentinel or a blank choice; any other answer is ok=true even if it names no
// offered option, since matchOption re-validates it downstream.
func (j *jevSelectChooser) Choose(ctx context.Context, question MergedField, options []string, facts []experience.Atom) (string, bool, error) {
	criteria := make(map[string]string, len(options)+1)
	for _, opt := range options {
		criteria[opt] = opt
	}
	criteria[jevSelectDeclineKey] = "nothing stated supports any option, or this question is a categorically excluded topic"

	claims := make([]string, 0, len(facts))
	for _, a := range facts {
		claim := a.Claim
		if a.Context != "" {
			claim = claim + " (" + a.Context + ")"
		}
		claims = append(claims, claim)
	}

	answers, err := j.client.Ask(ctx, map[string]any{"facts": claims}, map[string]jev.Question{
		jevSelectAnswerKey: {Type: "choice", Instructions: jevSelectInstructions(question.Label), Criteria: criteria},
	})
	if err != nil {
		return "", false, fmt.Errorf("atsapply: jev select: %w", err)
	}
	answer, ok := answers[jevSelectAnswerKey]
	if !ok {
		return "", false, fmt.Errorf("atsapply: jev select: response carried no %q answer", jevSelectAnswerKey)
	}
	if answer.Choice == jevSelectDeclineKey || strings.TrimSpace(answer.Choice) == "" {
		return "", false, nil
	}
	return answer.Choice, true, nil
}
