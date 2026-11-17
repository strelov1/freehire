package handler

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// evidencedRequirement is a requirement plus what the candidate's bank already holds for it. The
// evidence is named, not inlined: the agent needs the id to cite, and tool results are replayed every turn.
type evidencedRequirement struct {
	matchanalysis.Requirement
	Evidence []requirementEvidence `json:"evidence"`
}

type requirementEvidence struct {
	ID           string `json:"id"`
	Claim        string `json:"claim"`
	CanWriteCV   bool   `json:"can_write_cv"`
	NeedsContext bool   `json:"needs_context,omitempty"`
	NeedsMetrics bool   `json:"needs_metrics,omitempty"`
}

// agentTailorContext is the tailoring context as the AGENT reads it, deliberately narrower than what
// the endpoint serves the page. Dimension comments, strengths, gaps and recommendation are left out:
// the agent cannot edit a CV from them, restating the verdict is what it is told not to do, and the
// candidate already sees them in the panel. The agent gets the vacancy, the score in one line for tone,
// and the requirements with the bank's answer attached.
type agentTailorContext struct {
	Job          fitanalysis.TailoringJob `json:"job"`
	Verdict      string                   `json:"verdict"`
	OverallScore int                      `json:"overall_score"`
	MissingHave  []evidencedRequirement   `json:"missing_have"`
	MissingGap   []evidencedRequirement   `json:"missing_gap"`
}

// evidencePerRequirement bounds what one requirement may carry: enough to write a bullet from and to
// see the bank is not empty; the rest is one experience_search away.
const evidencePerRequirement = 3

// withBankEvidence answers, for every requirement, the question the agent would otherwise spend a tool
// round on: does the bank hold anything for this? Retrieval is a linear scan over the caller's own
// atoms with no model call, cheaper than the round it replaces. A bank that cannot be read degrades to
// no evidence rather than an error: the requirements are worth reading either way.
func (h *assistantHandlers) withBankEvidence(ctx context.Context, userID int64, base fitanalysis.TailoringContext) agentTailorContext {
	return agentTailorContext{
		Job:          base.Job,
		Verdict:      base.Verdict,
		OverallScore: base.OverallScore,
		MissingHave:  h.evidenceFor(ctx, userID, base.MissingHave),
		MissingGap:   h.evidenceFor(ctx, userID, base.MissingGap),
	}
}

func (h *assistantHandlers) evidenceFor(ctx context.Context, userID int64, reqs []matchanalysis.Requirement) []evidencedRequirement {
	out := make([]evidencedRequirement, 0, len(reqs))
	for _, r := range reqs {
		// Always a list, never nil: "looked and found nothing" decides whether to ask the candidate, and an
		// absent field reads as "did not look".
		found := []requirementEvidence{}
		if h.experience != nil {
			matches, err := h.experience.Retrieve(ctx, userID,
				experience.Query{Text: r.Text}, evidencePerRequirement)
			if err != nil {
				log.Printf("assistant: bank retrieval for %q: %v", r.Text, err)
			}
			for _, m := range matches {
				nc, nm := experience.Richness(m.Atom)
				found = append(found, requirementEvidence{
					ID:           m.Atom.ID.String(),
					Claim:        m.Atom.Claim,
					CanWriteCV:   m.Atom.Provenance.Publishable(),
					NeedsContext: nc,
					NeedsMetrics: nm,
				})
			}
		}
		out = append(out, evidencedRequirement{Requirement: r, Evidence: found})
	}
	return out
}

// cvGetTool reads the tailored CV document the session is editing, minus the contact block: this
