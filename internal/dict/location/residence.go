package location

// globalRegion is the region code meaning "open anywhere". It is a legitimate answer for
// a job and never one for a person, which is the whole reason Residence exists.
const globalRegion = "global"

// Residence is where a PERSON is, derived from the free-text location line of a CV.
//
// It is deliberately a distinct type from Geo rather than a flag on Parse. The same
// location string means opposite things on the two sides: for a job it answers "where is
// the work", for a résumé "where is the person". A flag can be defaulted wrong at a call
// site; two types cannot be confused at one. Residence carries no work mode at all —
// how someone is willing to work is a preference that lives on their profile
// (location_preferences.work_modes), not a fact about where they are.
type Residence struct {
	Countries []string
	Regions   []string
	Cities    []string
}

// ParseResidence maps a candidate's location line to where they are: Parse's output minus the
// work-mode hint and the global region.
//
// Global is excluded explicitly because Parse's "worldwide"/"anywhere" entries (see
// nameToRegion) still resolve to it, and a CV line like "Remote - Worldwide" names no place a
// person lives.
//
// Everything else survives, including a macro-region with no country ("EU / Remote" -> eu).
// Like Parse, it never guesses: an unresolvable location yields an empty Residence.
func ParseResidence(location string) Residence {
	geo := Parse(location)
	return Residence{
		Countries: geo.Countries,
		Regions:   withoutGlobal(geo.Regions),
		Cities:    geo.Cities,
	}
}

// withoutGlobal drops the global region, returning nil when nothing else remains so an
// all-global result is indistinguishable from no geography at all.
func withoutGlobal(regions []string) []string {
	var out []string
	for _, r := range regions {
		if r != globalRegion {
			out = append(out, r)
		}
	}
	return out
}
