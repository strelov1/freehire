package ojcp

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/job/ghost"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// OJCP has no field for "this posting may no longer be a real opening". `agent_notes` is
// the spec's free-text channel to an agent, so the posting-reality verdict travels there
// until the standard grows a field of its own — which is the follow-on RFC this
// implementation is meant to argue for.
//
// The wording is bound by the same doctrine as the interface's: the system observes facts
// about a POSTING and never an employer's intent, so the strongest thing it may say is
// that the posting is likely inactive. An agent will relay this to a candidate, and quite
// possibly to the employer; a sentence that reads as an accusation would be relayed as one.
// Keyed by ghost's own exported constants rather than by string literals: renaming a level
// there would otherwise leave this map silently returning "" for every affected posting,
// with no test failing because a test written against literals agrees with the bug.
var realityVerdict = map[string]string{
	ghost.LevelLikely:   "likely to be inactive",
	ghost.LevelPossible: "possibly inactive",
}

// criterionSentence renders a criterion code as something a person can read. An agent
// relays this note to a candidate, and `evergreen_posting` reaching them as-is is
// undefined jargon — the interface has always had to RENDER these rather than pass them
// through, which is why ghost exports the vocabulary at all.
//
// An unmapped code falls back to its own value: a criterion added to ghost and not named
// here reads awkwardly, which is better than a note that silently omits the evidence
// behind its own verdict.
var criterionSentence = map[string]string{
	ghost.CriterionEvergreenPosting:   "the posting has been open unusually long",
	ghost.CriterionATSAbsent:          "it is no longer on the employer's own ATS",
	ghost.CriterionSilentApplications: "applicants report no response",
	ghost.CriterionUserReports:        "people have reported it",
}

// agentNotesFor renders the posting-reality verdict as a sentence, or "" when there is no
// verdict. Silence is deliberate: most of the catalogue carries none, and a note saying
// "no concerns" would assert a check we never ran.
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
