package talentnetwork

import (
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// PrimaryTitle returns the title of the role that best describes what a candidate does
// now: the current role, or the one that ended most recently. It is empty when the CV
// has no titled role, which HandleBase handles with its neutral base.
//
// The choice is CONTENT-based, never positional: Experience ordering is not enforced
// (see resumeextract.Anonymous). When two roles are current, the later start wins.
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
// regardless of dates, even in a muddled history.
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
		// The extraction contract sets current:true AND end:null for an ongoing role;
		// honouring either alone is the fail-closed reading.
		current: e.Current || e.End == nil,
		end:     months(e.End),
		start:   months(e.Start),
	}
}

// months collapses a PeriodDate to a comparable integer; a year-only date sorts at the
// start of its year, as perioddate.Format assumes.
func months(d *perioddate.PeriodDate) int {
	if d == nil || d.Year <= 0 {
		return 0
	}
	return d.Year*12 + d.Month
}
