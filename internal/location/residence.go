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

// ParseResidence maps a candidate's location line to where they are: the job geography
// parser's output minus the work-mode hint and minus the global region.
//
// The global exclusion is stated as "a person is never located globally" rather than as
// "do not inherit the bare-remote fallback", because global reaches the result by TWO
// independent paths — the fallback in Parse (a remote marker that resolved no place) and
// the dictionary's own "worldwide"/"anywhere"/"по всему миру" entries. A rule phrased
// against the fallback alone would let the second one through.
//
// Everything else survives untouched, including a macro-region with no country ("EU /
// Remote" -> eu), which is a true claim about where someone is. Like Parse, this never
// guesses: a location the curated dictionary cannot resolve yields an empty Residence.
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
