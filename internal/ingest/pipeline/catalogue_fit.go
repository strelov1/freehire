package pipeline

import (
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

// outOfCatalogue reports whether a crawled posting does not belong in the catalogue and
// must not be persisted. The catalogue is broader than IT: a generic ATS board carries a
// company's whole hiring, and the nurses, cooks and cashiers on it are kept whenever the
// title dictionary files them under a category. What is refused is a non-technical title
// no category reaches — search would exclude it anyway (search.CategoryUnresolved).
//
// Technical evidence vetoes the rejection, and that ordering is not optional. The
// non-tech dictionary matches its terms anywhere in a title, and it was written on the
// explicit contract that it "is consulted only after the tech category dictionary is
// silent, so tech evidence always wins" (internal/dict/classify/nontech.go) — which is how
// jobderive.deriveIsTech uses it. Dropping that precedence turns the same dictionary
// into a far stronger claim than it was built to support: "DevOps Engineer (HVAC IoT
// Platform)", "Backend Engineer — Teller Systems" and "Senior Software Engineer - Cook
// County Health" all carry is_tech=true and all match a term (hvac, teller, cook).
// Labelling those non-tech was harmless because a label is reversible; deleting them
// is not, and a rejected posting never reaches Postgres, so nothing records that it
// happened.
//
// Below that veto the rule is the non-tech TITLE dictionary, deliberately not the
// tri-state is_tech. Every categorised non-technical role — sales, finance, healthcare,
// retail — resolves is_tech=false through its category and belongs in the catalogue
// all the same; rejecting on is_tech==false would drop all of them.
//
// This is catalogue policy, not derivation, which is why it lives here rather than in
// jobderive: the aggregate still constructs a non-technical posting without complaint,
// and the write paths that are never re-crawled — Telegram extraction, user
// submissions, link-source imports — go through that factory untouched. Rejection is
// only safe on a crawled board, where removing an over-broad dictionary term re-admits
// the postings on the next pass.
func outOfCatalogue(j job.Job) bool {
	f := j.Fields()
	return outOfCatalogueTitle(f.Title, f.IsTech != nil && *f.IsTech)
}

// outOfCatalogueTitle is the same rule stated over the two fields it actually reads, for the
// liveness-refresh path — which has a title and the stored row's tech evidence but no job.Job,
// because a hydrating adapter re-lists a posting without re-fetching its content. Passing the
// STORED evidence is what keeps the two paths equivalent: judged on the content-less listing
// instead, 1.7% of the titles the catalogue holds as technical would be turned away.
func outOfCatalogueTitle(title string, hasTechEvidence bool) bool {
	return classify.ConfirmedNonTech(title, hasTechEvidence)
}

// listingRejected is the same rule asked of a posting BEFORE its body is bought — the
// DetailGate answer a CoverageGated adapter consults so it does not pay for a body the filter
// is going to throw away. A rejected posting is never stored, so it is never seen, so without
// this a hydrating crawl re-buys every non-technical body on every run.
//
// It normalizes the body-less listing exactly as the write path will, so the answer is the
// write path's answer rather than a second statement of the policy. That only holds while the
// rule reads nothing the body could change, and it does not: is_tech is decided by the title
// and the adapter's structured category/hint, and the one description-derived input
// (NonTechFromDescription) can only fill a NON-technical category, which never supplies the
// tech evidence that vetoes a rejection. TestListingRejectedAgreesWithTheHydratedPosting pins
// that. A listing that does not construct answers false — the body is bought, and the write
// path decides as it always has.
func listingRejected(e sources.CompanyEntry) func(sources.Job) bool {
	return func(j sources.Job) bool {
		dj, err := normalizeJob(e, j, nil)
		if err != nil {
			return false
		}
		return outOfCatalogue(dj)
	}
}

// loggedRejectionSamples is how many rejected titles a board's log line carries. A
// count alone says how much was dropped but not what — and "what" is the only way to
// tell an over-broad dictionary term from a board that is genuinely all nurses.
const loggedRejectionSamples = 2

// rejections accumulates a board's catalogue-filter outcome for its one log line.
// Candidates counts only postings that actually reached the filter, so stream removals —
// which cannot be rejected — stay out of the denominator: without that, a feed emitting a
// thousand removals and five rejected postings would report 0% and hide the very signature
// the line exists to surface. A liveness refresh DOES reach the filter (it can be rejected,
// which is how a stored non-tech row ages out), so it counts as a candidate.
type rejections struct {
	rejected   int
	candidates int
	samples    []string
}

func (r *rejections) candidate() { r.candidates++ }

func (r *rejections) reject(title string) {
	r.rejected++
	if len(r.samples) < loggedRejectionSamples {
		r.samples = append(r.samples, title)
	}
}

// merge folds another writer's rejections for the same board into r, keeping the sample bound.
func (r *rejections) merge(o rejections) {
	r.rejected += o.rejected
	r.candidates += o.candidates
	for _, title := range o.samples {
		if len(r.samples) >= loggedRejectionSamples {
			break
		}
		r.samples = append(r.samples, title)
	}
}

// log reports the board's rejections once, with the share and a sample of the titles.
// A board rejecting nearly everything is the signature of a dictionary term that is
// too broad, and since boards are crawled hourly that has to be visible within the
// hour.
func (r *rejections) log(e sources.CompanyEntry) {
	if r.rejected == 0 {
		return
	}
	// A rejection is only counted for a posting that reached the filter, so
	// candidates is at least rejected and the division is safe.
	log.Printf("ingest: %s board %q (%s): rejected %d/%d postings as non-technical (%.0f%%), e.g. %s",
		e.Provider, e.Board, e.Company, r.rejected, r.candidates,
		100*float64(r.rejected)/float64(r.candidates), strings.Join(r.samples, "; "))
}
