package pipeline

import (
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
)

// outOfCatalogue reports whether a crawled posting does not belong on an IT job board
// and must not be persisted. A generic ATS board carries a company's whole hiring, not
// its engineering org, so most of what a crawl returns is nurses, cooks and cashiers.
//
// Technical evidence vetoes the rejection, and that ordering is not optional. The
// non-tech dictionary matches its terms anywhere in a title, and it was written on the
// explicit contract that it "is consulted only after the tech category dictionary is
// silent, so tech evidence always wins" (internal/classify/nontech.go) — which is how
// jobderive.deriveIsTech uses it. Dropping that precedence turns the same dictionary
// into a far stronger claim than it was built to support: "DevOps Engineer (HVAC IoT
// Platform)", "Backend Engineer — Teller Systems" and "Senior Software Engineer - Cook
// County Health" all carry is_tech=true and all match a term (hvac, teller, cook).
// Labelling those non-tech was harmless because a label is reversible; deleting them
// is not, and a rejected posting never reaches Postgres, so nothing records that it
// happened.
//
// Below that veto the rule is the non-tech TITLE dictionary, deliberately not the
// tri-state is_tech. A business role — sales, recruiting, finance — resolves
// is_tech=false through its category, but whether it belongs in the catalogue depends
// on whether the company is an IT company, which the crawl cannot know and the prune
// worker decides later. Rejecting on is_tech==false would quietly drop every sales and
// recruiting job at every employer on the board.
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

// log reports the board's rejections once, with the share and a sample of the titles.
// A board rejecting nearly everything is the signature of a dictionary term that is
// too broad, and since boards are crawled hourly that has to be visible within the
// hour — long before the next pruning pass acts on the same dictionary.
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
