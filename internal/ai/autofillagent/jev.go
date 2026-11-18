package autofillagent

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// jevDeclineKey is offered to Jev alongside the real options as Choose's "decline"
// outcome. Its wording cannot collide with a real form option.
const jevDeclineKey = "none of these / not stated"

const (
	// jevAnswerKey names the single question asked of Jev, in the request and the response.
	jevAnswerKey = "answer"
)

// jevChooseInstructions mirrors choosePrompt's rules (see planner.go), narrowed to
// the single question Jev is asked.
func jevChooseInstructions(label string) string {
	return fmt.Sprintf(`You are answering ONE question on a job-application form by
picking from the options it offers.

Question: %s

Pick the single option the profile supports. If nothing in the profile supports any
option, pick %q — declining is the correct answer whenever nothing in the profile
supports any of the options. Never infer a claim the profile does not state.`, label, jevDeclineKey)
}

// jevChooser answers one Choose question through the Typesafe AI Jev model's
// /v1/systemone endpoint. No retries: JevPlanner falls back to the LLM on failure.
type jevChooser struct{ client *jev.Client }

// newJevChooser takes jev.WithBaseURL to point at a test server instead of the live API.
func newJevChooser(apiKey string, opts ...jev.Option) *jevChooser {
	return &jevChooser{client: jev.New(apiKey, opts...)}
}

// Choose asks Jev to pick one of options for question, grounded in profile. It returns ""
// for the decline answer and for any sentinel it does not recognize; the caller
// re-validates against the offered list and the profile.
func (j *jevChooser) Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error) {
	criteria := make(map[string]string, len(options)+1)
	for _, opt := range options {
		criteria[opt] = opt
	}
	criteria[jevDeclineKey] = "nothing in the profile supports any of the listed options"

	answers, err := j.client.Ask(ctx, map[string]any{"profile": profile}, map[string]jev.Question{
		jevAnswerKey: {Type: "choice", Instructions: jevChooseInstructions(question.Label), Criteria: criteria},
	})
	if err != nil {
		return "", fmt.Errorf("jev choose: %w", err)
	}
	answer, ok := answers[jevAnswerKey]
	if !ok {
		return "", fmt.Errorf("jev choose: response carried no %q answer", jevAnswerKey)
	}
	if answer.Choice == jevDeclineKey {
		return "", nil
	}
	return answer.Choice, nil
}
