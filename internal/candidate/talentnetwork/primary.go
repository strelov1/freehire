package talentnetwork

import (
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// PrimaryTitle returns the title of the role that best describes what a candidate does
// now: the current role, or — when nothing is current — the one that ended most
// recently. It is empty when the CV carries no titled role at all, which is a real
// answer and not a failure: HandleBase turns it into the neutral base rather than
// refusing to mint.
//
// The choice is CONTENT-based, never positional. Structured.Experience's ordering
// (newest-first or oldest-first) is nowhere documented and nowhere enforced by the
// extraction prompt or schema, so "the first entry" means nothing — the same reasoning
// resumeextract.Anonymous records for masking by content instead of by array position.
//
// Two roles that are both current is an ordinary situation (a job and an advisory seat,
// or a sloppily filled CV), so the later start wins rather than the function picking
// arbitrarily.
func PrimaryTitle(s resumeextract.Structured) string {
	best := ""
	var bestRank titleRank
	for _, e := range s.Experience {
		if e.Title == "" {
			continue
		}
		r := rankOf(e)
		if best == "" || r.after(bestRank) {
			best, bestRank = e.Title, r
		}
	}
	return best
}

// titleRank orders experience entries by recency. current outranks every ended role
// regardless of dates, because a role that has not ended is what the candidate does now
// even when a later end date appears elsewhere in a muddled history.
type titleRank struct {
	current bool
	end     int // year*12+month, 0 when unknown
	start   int
}

func (r titleRank) after(other titleRank) bool {
	if r.current != other.current {
		return r.current
	}
	if r.end != other.end {
		return r.end > other.end
	}
	return r.start > other.start
}

func rankOf(e resumeextract.Experience) titleRank {
	return titleRank{
		// A nil End means ongoing as surely as Current does: the extraction contract
		// tells the model to set current:true AND end:null for a role that has not
		// ended, so honouring either one alone is the fail-closed reading.
		current: e.Current || e.End == nil,
		end:     months(e.End),
		start:   months(e.Start),
	}
}

// months collapses a PeriodDate to a single comparable integer. A year-only date sorts
// at the start of its year, which is the same assumption perioddate.Format makes when
// it prints one.
func months(d *perioddate.PeriodDate) int {
	if d == nil || d.Year <= 0 {
		return 0
	}
	return d.Year*12 + d.Month
}
