package telegram

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// JevJudge is the VacancyJudge backed by TypeSafe's Jev: one yes/no question per post,
// answered with a score in ~0.2s. It is a discrete decision over a fixed question, which
// is what Jev is for and what a generative model is a slow, costly way to ask.
//
// Measured 2026-10-21 on prod posts, with a model reading each one as the reference: of 90
// real vacancies among 150 the markers had declined, it admitted all 90 and 3 non-vacancies
// beside them; of 144 vacancies among 150 the markers admitted, it would have declined one.
//
// The transport is internal/platform/jev; this file owns the question and the threshold.
type JevJudge struct{ client *jev.Client }

const (
	// jevQuestionKey names the one question in the request and its answer.
	jevQuestionKey = "vacancy"

	// jevThreshold is the score at which a post counts as a vacancy. The measurement above
	// separated cleanly: the same admissions at 0.3 and 0.5, two vacancies lost at 0.7.
	jevThreshold = 0.5

	// jevMaxRunes bounds what one question sends. A Telegram post holds up to 4096
	// characters and the opening is where a vacancy says it is one; the measurement sent
	// the first 3000.
	jevMaxRunes = 3000

	jevInstructions = "Does this Telegram post advertise at least one job vacancy — an employer or " +
		"recruiter offering a position someone can apply to? Answer no for a job SEEKER's post, " +
		"news, articles, courses, events, memes, or chat."
)

// NewJevJudge builds a judge authenticated with apiKey; jev.WithBaseURL points it at a
// test server.
func NewJevJudge(apiKey string, opts ...jev.Option) *JevJudge {
	return &JevJudge{client: jev.New(apiKey, opts...)}
}

// IsVacancy asks whether text advertises a vacancy. Any answer that is not a score is an
// error, never a "no" — the caller admits on an error, and a "no" is final.
func (j *JevJudge) IsVacancy(ctx context.Context, text string) (bool, error) {
	if r := []rune(text); len(r) > jevMaxRunes {
		text = string(r[:jevMaxRunes])
	}
	answers, err := j.client.Ask(ctx, map[string]string{"post": text}, map[string]jev.Question{
		jevQuestionKey: {Type: "noul", Instructions: jevInstructions},
	})
	if err != nil {
		return false, fmt.Errorf("jev: %w", err)
	}
	answer, ok := answers[jevQuestionKey]
	if !ok || answer.Noul == nil {
		return false, fmt.Errorf("jev: response carries no score for %q", jevQuestionKey)
	}
	return *answer.Noul >= jevThreshold, nil
}

var _ VacancyJudge = (*JevJudge)(nil)

// JudgeFromKey returns the Jev judge for apiKey, or nil when there is no key — and nil as
// an INTERFACE, which is what the runners test for. Returning a nil *JevJudge here would
// hand them a non-nil judge that panics on its first question, so both binaries build the
// judge through this rather than each spelling the condition.
func JudgeFromKey(apiKey string) VacancyJudge {
	if apiKey == "" {
		return nil
	}
	return NewJevJudge(apiKey)
}
