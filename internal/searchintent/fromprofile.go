package searchintent

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/vocab"
)

// FromProfile builds a search from a saved profile WITHOUT calling a model.
//
// The profile is already written in the filter's own vocabulary: a specialization is a
// category value, a skill is a canonical tag, the location block holds work modes and
// ISO codes — all of it validated on the way in. Handing that to a model to "interpret"
// would pay to guess at something already stated exactly, and give it the chance to
// guess wrong. The mapping is the whole feature here.
//
// It still runs the values through resolve. They were stored against a vocabulary that
// has changed since, and a category that has been retired must be reported like any
// other value nothing places — not applied because a database row still holds it.
func FromProfile(p Profile) Result {
	// resolve refuses only a facet name no filter has, and every name written here is
	// one this package resolves — so the error cannot fire. Discarding it keeps the
	// deterministic path free of an error return that no caller could ever act on.
	out, _ := intent{
		Facets: map[string][]string{
			"category":  p.Specializations,
			"skills":    p.Skills,
			"work_mode": p.WorkModes,
			// Where they will work FROM, and where they would MOVE to, are both places
			// they asked for. Where they merely LIVE is not: someone in Lisbon who is
			// open to relocating did not ask for jobs in Portugal, and importing their
			// address would answer the opposite of their question.
			"regions":   codesOfKind(regionCode, p.RemoteFrom, p.RelocateTo),
			"countries": codesOfKind(countryCode, p.RemoteFrom, p.RelocateTo),
			"cities":    codesOfKind(cityName, p.RemoteFrom, p.RelocateTo),
		},
		Exclude: map[string][]string{"skills": p.ExcludedSkills},
	}.resolve()

	out.Summary = summarize(out)
	return out
}

// The three geography kinds a profile's lists mix together. A stored list holds region
// codes, ISO country codes and free-text city names side by side, and each belongs to a
// different facet — so they are sorted by what they ARE rather than by which list they
// came from.
type geoKind int

const (
	regionCode geoKind = iota
	countryCode
	cityName
)

// codesOfKind picks the values of one geographic kind out of the lists given. Whatever
// it lets through still faces the facet's own resolver, so a misfiled value is dropped
// and reported rather than filtered on.
func codesOfKind(kind geoKind, lists ...[]string) []string {
	var out []string
	for _, list := range lists {
		for _, raw := range list {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			var is bool
			switch kind {
			case regionCode:
				is = containsFold(vocab.RegionValues, value)
			case countryCode:
				is = !containsFold(vocab.RegionValues, value) &&
					location.NormalizeCountry(value) != ""
			case cityName:
				is = !containsFold(vocab.RegionValues, value) &&
					location.NormalizeCountry(value) == ""
			}
			if is {
				out = append(out, value)
			}
		}
	}
	return out
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// summarize writes the sentence the dialog shows instead of the raw values. A result
// the model built brings its own; this one has to compose it, and composing it from
// what RESOLVED — not from what the profile said — is what keeps the sentence and the
// filter describing the same search.
func summarize(r Result) string {
	var parts []string
	add := func(format string, values []string) {
		if len(values) > 0 {
			parts = append(parts, fmt.Sprintf(format, strings.Join(values, ", ")))
		}
	}
	add("%s roles", r.Facets["category"])
	add("using %s", r.Facets["skills"])
	add("%s", r.Facets["work_mode"])
	add("in %s", append(append([]string{}, r.Facets["regions"]...),
		append(r.Facets["countries"], r.Facets["cities"]...)...))
	add("not %s", r.Exclude["skills"])
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ") + "."
}
