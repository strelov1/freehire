package mcpapp

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/search/search"
)

// A filter value we hold no facet for is dropped from the query and named in the answer,
// rather than filtered on.
//
// An unknown value (`category=ai`) would match no posting and read as an empty catalogue,
// and a language model would tell the person "freehire has no AI jobs". So the value leaves
// the query and is named in the answer; the free text still runs.
//
// Only CLOSED vocabularies are checked, and which are closed is the search core's answer
// (search.UnknownFacetValue), so this and REST's meta.ignored_values cannot drift. That
// includes skills: one the dictionary cannot resolve ("golangg") matches nothing, while an
// alias it resolves ("golang") is kept. Cities, company slugs and sources have no finite
// list; list_filter_values is how a model learns their values.

// checkedFacets names each query parameter whose values must belong to a closed vocabulary,
// with the field the input carries it in.
//
// ONE table, deliberately: a map beside a field-resolving switch fails silently when a facet
// lacks its case (every value passes). The query builder and the reporter both walk this
// slice, so a new facet joins both by being written once.
var checkedFacets = []struct {
	param  string
	values func(SearchInput) []string
}{
	{"work_mode", func(in SearchInput) []string { return in.WorkMode }},
	{"seniority", func(in SearchInput) []string { return in.Seniority }},
	{"category", func(in SearchInput) []string { return in.Category }},
	{"employment_type", func(in SearchInput) []string { return in.EmploymentType }},
	{"engagement", func(in SearchInput) []string { return in.Engagement }},
	{"english_level", func(in SearchInput) []string { return in.EnglishLevel }},
	{"regions", func(in SearchInput) []string { return in.Regions }},
	{"relocation", func(in SearchInput) []string { return in.Relocation }},
	{"skills", func(in SearchInput) []string { return in.Skills }},
	{"skills_exclude", func(in SearchInput) []string { return in.ExcludeSkills }},
}

// Unsupported names the filter values this input carries that the catalogue cannot filter
// on, each as `param=value` so the model learns which word to replace.
//
// The `param=value` form differs from REST's bare `ignored_params` names: a model composing
// a follow-up needs to know which value was wrong.
func (in SearchInput) Unsupported() []string {
	var out []string
	for _, facet := range checkedFacets {
		for _, value := range facet.values(in) {
			if search.UnknownFacetValue(facet.param, value) {
				out = append(out, facet.param+"="+value)
			}
		}
	}
	out = append(out, unsupportedCountries(in.Countries)...)

	// Sorted so identical requests give identical answers.
	slices.Sort(out)
	return out
}

// unsupportedCountries names the values that are not shaped like a country code. Both tool
// inputs report it, so it is written once.
func unsupportedCountries(values []string) []string {
	var out []string
	for _, value := range values {
		if !isCountryCode(value) {
			out = append(out, "countries="+value)
		}
	}
	return out
}

// known reports whether the catalogue can filter on this value.
//
// The comparison folds case because the search does (`countries=de` equals `DE`); a stricter
// check would report a working filter as unsupported.
func known(allowed []string, value string) bool {
	value = strings.TrimSpace(value)
	return slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, value) })
}

// keepKnown drops the values of a facet param the catalogue cannot filter on.
//
// A facet with one wrong value keeps its right ones, so the answer is not wider than asked.
func keepKnown(param string, values []string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if !search.UnknownFacetValue(param, value) {
			kept = append(kept, value)
		}
	}
	return kept
}

// countryCodes keeps the values that are shaped like a country code. A country NAME left in
// filters on a value no posting carries and answers zero.
func countryCodes(values []string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if isCountryCode(value) {
			kept = append(kept, value)
		}
	}
	return kept
}

// isCountryCode accepts the shape of an ISO 3166-1 alpha-2 code, catching "germany" for "DE".
// Which codes exist is internal/dict/location's question, not a third dictionary.
func isCountryCode(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if !isASCIILetter(r) {
			return false
		}
	}
	return true
}

func isASCIILetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}
