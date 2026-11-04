package autofillagent

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// jevDeclineKey is offered to Jev alongside the page's real options, standing in for
// Choose's own "decline" outcome. Its wording cannot collide with a real form option —
// no ATS renders a combobox choice phrased as a description of itself declining to answer.
const jevDeclineKey = "none of these / not stated"

const (
	// jevAnswerKey names the single question this package ever asks Jev, used both to
	// build the request and to read the matching answer back out of the response.
	jevAnswerKey = "answer"
)

// jevChooseInstructions mirrors choosePrompt's rules (see planner.go), narrowed to the
// single question Jev is asked. The spike that validated this wording against the live
// API is recorded in openspec/changes/autofill-choose-jev-provider/design.md.
func jevChooseInstructions(label string) string {
	return fmt.Sprintf(`You are answering ONE question on a job-application form by
picking from the options it offers.

Question: %s

Pick the single option the profile supports. If nothing in the profile supports any
option, pick %q — declining is the correct answer whenever nothing in the profile
supports any of the options. Never infer a claim the profile does not state.`, label, jevDeclineKey)
}

// jevChooser answers one Choose question through the Typesafe AI Jev model's
// /v1/systemone endpoint. One attempt, no retries: a slow or failing Jev call falls back
// to the LLM at the JevPlanner level, so retrying here would only delay that fallback.
type jevChooser struct{ client *jev.Client }

// newJevChooser takes jev.WithBaseURL to point at a test server instead of the live API.
func newJevChooser(apiKey string, opts ...jev.Option) *jevChooser {
	return &jevChooser{client: jev.New(apiKey, opts...)}
}

// Choose asks Jev to pick one of options for question, grounded in profile. It returns ""
// for both the decline answer and any sentinel it could not recognize — the caller
// (driveWidget) already re-validates the result against the offered list and the profile,
// so a hallucinated value outside jevDeclineKey or options is caught there, not here.
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
