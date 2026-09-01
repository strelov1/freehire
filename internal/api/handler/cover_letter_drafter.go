package handler

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// letterBankPort is the experience bank as this feature needs it: evidence retrieval and the
// candidate projection. Naming both halves in one interface is what lets a surface hold ONE
// field for one object - it previously arrived under two names on each of two handlers, four
// names for the same store, and a reader had to check whether they could differ.
type letterBankPort interface {
	coverletter.Retriever
	candidateProfiler
}

// coverLetterDeps is the cover-letter surface's dependencies travelling as one value. Both
// handler types take it through their constructor rather than by assignment afterwards: the
// bank, the store and the chain all exist before either handler is built, and
// handler/AGENTS.md reserves post-construction assignment for dependencies that genuinely do
// not. Passing them as three parameters instead would be the same clump written out.
type coverLetterDeps struct {
	letters *coverletter.Store
	chain   *coverletter.Analyzer
	bank    letterBankPort
}

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
	bank    letterBankPort
	resume  *resume.Store
	chain   *coverletter.Analyzer
	letters *coverletter.Store
}

// ready reports whether every dependency is wired. A partially-wired harness must refuse in
// the caller's own vocabulary rather than panic inside it — on the tool's path that panic
// would land in a detached SSE-writer goroutine, where no error path is listening.
func (d letterDrafter) ready() bool {
	return d.jobs != nil && d.fit != nil && d.bank != nil && d.chain != nil && d.letters != nil
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
	// REQUIRES a cached analysis; it does not produce one. Producing is Ensure's, and Ensure
	// takes the coalescing Request the autopilot assembles — a letter has no business
	// assembling that, and no business paying for an analysis the candidate did not ask for.
	// An absent analysis therefore surfaces as ErrNoAnalysis, which the shared error mapper
	// renders as "run the fit analysis first": a state the candidate can act on in the same
	// workspace, one tab over.
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
	candidate := candidateProfileFrom(ctx, d.resume, d.bank, userID)

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

// letterReleaseTimeout bounds the detached release. Generous for two small statements, short
// enough that a wedged database cannot pile up goroutines behind it. Same figure and same
// reasoning as the assistant turn's.
const letterReleaseTimeout = 5 * time.Second

// releaseLetterCharge gives back what a draft took when it produced nothing usable.
//
// It runs on a DETACHED context on purpose, the rule releaseTurn documents beside itself: a
// client that disconnects mid-draft cancels the request context, the chain fails with it, and
// a release on that same context could not even open its transaction — leaving the candidate
// charged for a letter they never got, in exactly the case this exists for. A three-call chain
// is the likeliest place in this feature to lose the client.
//
// Safe to call blind: an empty reference releases nothing.
func releaseLetterCharge(plans *plan.Store, userID int64, ref string) {
	if plans == nil || ref == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), letterReleaseTimeout)
	defer cancel()
	if err := plans.Release(ctx, userID, plan.FeatureCoverLetter, ref); err != nil {
		log.Printf("plan: releasing a cover letter for user %d: %v", userID, err)
	}
}

// letterAttempt names the drafting attempt about to happen, from the stored draft's own
// timestamp. A retry of the same request computes the same string and takes nothing more; a
// redraft happens after a successful save moved that timestamp, so it computes a new one and
// pays again — which is right, because a redraft is a second set of model calls.
//
// Both entry points use it. The tool used a constant, which made every redraft from chat free
// forever while the endpoint charged for each — the two diverging on the one axis they were
// built to share.
func letterAttempt(ctx context.Context, letters *coverletter.Store, userID, jobID int64) string {
	if letters == nil {
		return "first"
	}
	stored, err := letters.Get(ctx, userID, jobID)
	if err != nil || stored == nil {
		// An unreadable draft must not make two attempts share a reference: charging twice is
		// recoverable, charging once for many is not.
		return "first"
	}
	return stored.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

// coverLetterRef names one drafting attempt in the usage ledger. The ledger's uniqueness index
// is on (user_id, feature, ref) for a consume, so this string is what makes a retry idempotent.
func coverLetterRef(jobID int64, attempt string) string {
	return "cover-letter#" + strconv.FormatInt(jobID, 10) + "#" + attempt
}

// chargeLetter takes one cover-letter allowance and reports what happened, without deciding
// how a refusal is phrased — an endpoint owes a 402, a tool owes the model a sentence, and
// that difference is the one thing the two entry points must NOT share.
//
// Returns (ref, refused, err): a non-empty ref is a charge this request actually took and owes
// a release; refused is a ceiling reached. A deployment with no meter charges nothing and
// runs, and an unreadable ledger fails open — the rule every meter here follows, because a
// ledger that cannot be read must not withhold work the candidate is entitled to.
func chargeLetter(ctx context.Context, plans *plan.Store, userID, jobID int64, attempt string) (string, bool, plan.Decision) {
	if plans == nil {
		return "", false, plan.Decision{}
	}
	ref := coverLetterRef(jobID, attempt)
	d, err := plans.Consume(ctx, userID, plan.FeatureCoverLetter, ref)
	switch {
	case err == nil && d.Charge == 0:
		return "", false, d // already paid for under this reference
	case err == nil:
		return ref, false, d
	case isRefusal(err):
		return "", true, d
	default:
		log.Printf("plan: charging a cover letter for user %d: %v", userID, err)
		return "", false, d
	}
}

// citedAtom is one piece of evidence as the surface shows it: the id the letter stored, and
// the claim a reader actually checks.
//
// The claim is resolved HERE rather than left to the client. A client that has to look ids up
// can forget to — and did: the workspace threaded an optional lookup table that nothing ever
// passed, so every citation rendered the same placeholder while types, tests and linters all
// stayed green. The citation list is this feature's whole claim to honesty; it cannot depend
// on a caller remembering to populate it.
type citedAtom struct {
	ID string `json:"id"`
	// Claim is empty when the atom is gone — an owner may delete evidence a stored letter
	// still cites. The letter as sent still said what it said, so the id survives and the
	// surface says the achievement is no longer in the bank.
	Claim string `json:"claim,omitempty"`
}

// citedAtomsOf resolves a letter's citations against the owner's bank, in the letter's own
// order. Best-effort: an unreadable bank yields ids without claims rather than failing a read
// that is otherwise complete.
func citedAtomsOf(ctx context.Context, bank letterBankPort, userID int64, ids []uuid.UUID) []citedAtom {
	out := make([]citedAtom, 0, len(ids))
	if len(ids) == 0 {
		return out
	}
	claims := make(map[uuid.UUID]string)
	if bank != nil {
		if atoms, err := bank.ListAtoms(ctx, userID); err == nil {
			for _, a := range atoms {
				claims[a.ID] = a.Claim
			}
		} else {
			log.Printf("coverletter: resolving citations for user %d: %v", userID, err)
		}
	}
	for _, id := range ids {
		out = append(out, citedAtom{ID: id.String(), Claim: claims[id]})
	}
	return out
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
