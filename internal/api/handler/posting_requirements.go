package handler

import (
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// postingRequirements is what a posting asks for, in its own words: the model's enrichment
// list, or the markup parser's derived column when the model stated none.
//
// It reads through jobview rather than off the row because those two producers are
// reconciled in exactly ONE place — jobview.FromDomain — and a second copy of that rule
// here is the drift its own documentation warns about. This lives in the handler and not
// beside the readers that want it (fitanalysis, in the candidate block) because candidate
// sits below job in the layering and may not import jobview at all.
//
// A row the projection cannot read yields no requirements rather than an error. The
// tailoring and cover-letter contexts are worth serving without them; refusing the whole
// context over an unreadable enrichment blob would trade a complete answer for none.
func postingRequirements(job db.Job) []enrich.Requirement {
	view, err := jobview.FromRow(job)
	if err != nil {
		return nil
	}
	return view.Enrichment.Requirements
}
