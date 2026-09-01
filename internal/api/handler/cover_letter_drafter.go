package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// letterDrafter is the one path from a vacancy to a stored cover letter. Both entry points —
// the endpoint and the assistant tool — assemble one and call it, so what a letter is built
// from cannot drift between the button and the chat.
//
// It deliberately does NOT meter. The two callers refuse differently and must: an endpoint
// answers 402, while a tool has to hand the model a sentence it can relay, because the turn
// was already paid for and still owes an answer. Folding that in would force one of them to
// speak the other's language.
type letterDrafter struct {
	jobs    jobReader
	fit     *fitanalysis.Service
	bank    coverletter.Retriever
	profile candidateProfiler
	resume  *resume.Store
	chain   *coverletter.Analyzer
	letters *coverletter.Store
}

// ready reports whether every dependency is wired. A partially-wired harness must refuse in
// the caller's own vocabulary rather than panic inside it — on the tool's path that panic
// would land in a detached SSE-writer goroutine, where no error path is listening.
func (d letterDrafter) ready() bool {
	return d.jobs != nil && d.fit != nil && d.bank != nil && d.profile != nil &&
		d.chain != nil && d.letters != nil
}

// draft runs the chain for one (candidate, vacancy) and stores the result.
//
// Returns (nil, nil) when the chain produced nothing — an unconfigured gateway — which the
// caller reads as "release whatever was charged and say so". Every other failure comes back
// as an error, so a caller can never overwrite a stored letter with an empty one.
func (d letterDrafter) draft(
	ctx context.Context, client *llm.Client, userID, jobID int64, band coverletter.Band,
) (*coverletter.Letter, error) {
	job, err := d.jobs.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	// Produces an analysis when none is cached, as the assistant's interview_context tool and
	// the autopilot's run plan already do, and is not charged for it: a candidate who asks for
	// a letter on a vacancy they never analysed gets one.
	tailoring, err := d.fit.TailoringContext(ctx, userID, job)
	if err != nil {
		return nil, err
	}
	atoms, err := coverletter.Gather(ctx, d.bank, userID, tailoring.MissingHave)
	if err != nil {
		return nil, err
	}
	// The BANK layered over the structured résumé, not the file's structure alone. The ATS
	// report judges the document, so it reads the file; a letter speaks for the candidate, so
	// it reads what the candidate has — and a letter built from the file alone could not cite
	// an achievement banked from chat.
	candidate := candidateProfileFrom(ctx, d.resume, d.profile, userID)

	// The atoms go in UNFILTERED: the provenance gate lives inside Draft so that no caller can
	// apply a weaker one, or forget to apply it at all.
	letter, err := d.chain.As(client).Draft(ctx, coverletter.Input{
		Context:         tailoring,
		Candidate:       candidate,
		Atoms:           atoms,
		Band:            band,
		PostingLanguage: job.PostingLanguage,
	})
	if err != nil || letter == nil {
		return nil, err
	}
	if err := d.letters.Save(ctx, userID, jobID, *letter, modelIDOf(client)); err != nil {
		return nil, err
	}
	return letter, nil
}

// modelIDOf names the model a draft is stamped with. Empty on a deployment with no gateway,
// which Stored.Stale then reads as "matches" — a letter cannot be stale against a model that
// does not exist.
func modelIDOf(client *llm.Client) string {
	if client == nil {
		return ""
	}
	return client.ModelID()
}
