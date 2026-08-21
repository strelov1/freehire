package searchintent

import (
	"fmt"
	"strings"
)

// Profile is what the caller has already told us, in the form this package reads it.
//
// It is a plain struct rather than the stored profile type so the package keeps no
// dependency on the account layer: the handler maps one to the other, and the mapping
// is the only place that has to know both. Fields are the caller's own words — the
// dictionaries canonicalise whatever the model makes of them, exactly as they do for a
// typed description.
type Profile struct {
	Specializations []string
	Skills          []string
	// ExcludedSkills are the technologies the caller said they do not want to work
	// with. They must reach the model as things to rule OUT: importing them as wants
	// would build precisely the search they told us to avoid.
	ExcludedSkills []string
	// Locations are their location preferences as written.
	Locations []string
	// Headline and Years come from their structured CV, and are what turn "8 years,
	// mostly Go" into a seniority the profile itself never states.
	Headline string
	Years    string
}

// describe renders the profile as the material the model reads, or "" when there is
// nothing in it. An empty profile is not a search, and a request carrying one is
// refused rather than answered by invention.
func (p *Profile) describe() string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	line := func(label string, values []string) {
		if len(values) > 0 {
			fmt.Fprintf(&b, "- %s: %s\n", label, strings.Join(values, ", "))
		}
	}
	if p.Headline != "" {
		fmt.Fprintf(&b, "- headline: %s\n", p.Headline)
	}
	if p.Years != "" {
		fmt.Fprintf(&b, "- years of experience: %s\n", p.Years)
	}
	line("wants to work as", p.Specializations)
	line("works with", p.Skills)
	line("wants to avoid working with", p.ExcludedSkills)
	line("will work from", p.Locations)
	return b.String()
}
