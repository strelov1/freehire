package ojcp

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/job/ghost"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// OJCP has no field for "this posting may no longer be a real opening", so the
// posting-reality verdict travels in `agent_notes`, the spec's free-text channel to an
// agent, until the standard grows a field.
//
// The wording observes facts about a POSTING, never an employer's intent: the strongest
// thing it may say is that the posting is likely inactive, since an agent relays it onward.
// Keyed by ghost's exported constants, not string literals, so a renamed level cannot
// silently yield "" with no failing test.
var realityVerdict = map[string]string{
	ghost.LevelLikely:   "likely to be inactive",
	ghost.LevelPossible: "possibly inactive",
}

// criterionSentence renders a criterion code as something a person can read; a code like
// `evergreen_posting` is jargon once an agent relays it to a candidate.
//
// An unmapped code falls back to its own value: awkward, but better than silently omitting
// the evidence behind the verdict.
var criterionSentence = map[string]string{
	ghost.CriterionEvergreenPosting:   "the posting has been open unusually long",
	ghost.CriterionATSAbsent:          "it is no longer on the employer's own ATS",
	ghost.CriterionSilentApplications: "applicants report no response",
	ghost.CriterionUserReports:        "people have reported it",
}

// agentNotesFor renders the posting-reality verdict as a sentence, or "" when there is no
// verdict. Silence is deliberate: "no concerns" would assert a check we never ran.
func agentNotesFor(j jobview.Job) string {
	g := j.Ghost
	if g == nil {
		return ""
	}
	verdict, ok := realityVerdict[g.Level]
	if !ok {
		return ""
	}

	note := fmt.Sprintf(
		"Posting-reality signal (freehire): this posting is %s — %d of %d checks fired",
		verdict, len(g.Criteria), g.CriteriaTotal,
	)
	if reasons := criterionSentences(g.Criteria); len(reasons) > 0 {
		note += ": " + strings.Join(reasons, "; ")
	}
	return note + ". These are observations about the posting, not a statement about the employer."
}

func criterionSentences(codes []string) []string {
	sentences := make([]string, 0, len(codes))
	for _, code := range codes {
		if sentence, ok := criterionSentence[code]; ok {
			sentences = append(sentences, sentence)
			continue
		}
		sentences = append(sentences, code)
	}
	return sentences
}
