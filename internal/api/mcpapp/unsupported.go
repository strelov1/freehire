package mcpapp

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/search/search"
)

// A filter value we hold no facet for is dropped from the query and named in the answer,
// rather than filtered on.
//
// Measured against production on 2026-09-18: `/jobs/search?category=ai` answers 0 results
// and reports nothing ignored. "ai" is not one of our categories, so the fragment matches
// no posting at all — and the caller cannot tell that from an empty catalogue. Over REST
// that is a client's problem to debug; here the client is a language model that will
// summarise the zero as "freehire has no AI jobs" and say it to a person with confidence.
//
// So the value comes out of the query and goes into the answer by name. The free text still
// runs, which for "ai" is the search that was wanted in the first place, and the model is
// told which word was not understood instead of being handed a false negative.
//
// Only CLOSED vocabularies are checked, and which ones are closed is the search core's
// answer (search.UnknownFacetValue), not this package's: the REST surface names the same
// values in meta.ignored_values, and two lists of "what is known" would drift. That
// includes skills — the facet is written by the skill dictionary alone, so a skill it
// cannot resolve ("golangg") matches nothing, while an alias it does resolve ("golang")
// is kept and reaches the postings tagged "go". Cities, company slugs and sources have no
// finite list to check against; list_filter_values is how a model learns their values.

// checkedFacets names each query parameter whose values must belong to a closed vocabulary,
// with the field the input carries it in.
//
// ONE table, deliberately. It was a map of vocabularies beside a switch resolving the field,
// and that pair had a silent failure mode: a facet added to the map without its matching
// case reads NO values, so every value passes and the check is switched off for exactly the
// facet it was added for. Both readers — the query builder and the reporter — walk this
// slice, so a new facet joins both by being written once.
var checkedFacets = []struct {
	param  string
	values func(SearchInput) []string
}{
	{"work_mode", func(in SearchInput) []string { return in.WorkMode }},
	{"seniority", func(in SearchInput) []string { return in.Seniority }},
	{"category", func(in SearchInput) []string { return in.Category }},
	{"employment_type", func(in SearchInput) []string { return in.EmploymentType }},
	{"english_level", func(in SearchInput) []string { return in.EnglishLevel }},
	{"regions", func(in SearchInput) []string { return in.Regions }},
	{"relocation", func(in SearchInput) []string { return in.Relocation }},
	{"skills", func(in SearchInput) []string { return in.Skills }},
	{"skills_exclude", func(in SearchInput) []string { return in.ExcludeSkills }},
}

// Unsupported names the filter values this input carries that the catalogue cannot filter
// on, each as `param=value` so the model learns which word to replace.
//
// The `param=value` form differs from the REST surface's `ignored_params`, which lists bare
// parameter names. The audience is why: a client debugging its own request knows what it
// sent, while a model composing a follow-up needs to be told which of the three categories
// it passed was the wrong one.
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

	// Sorted so the answer is stable: an unordered list would make two identical requests
	// produce two different answers, which is a difference a model will try to interpret.
	slices.Sort(out)
	return out
}

// unsupportedCountries names the values that are not shaped like a country code. Both tool
// inputs report it, so it is written once — the company search went out without any such
// check at all, which is what a second copy of a rule invites.
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
// The comparison folds case because the search itself does: measured against production on
// 2026-09-18, `countries=de` and `countries=DE` both answer 59,942. A check stricter than
// the thing it guards would reject values that work, which is a worse failure than the one
// it prevents — it would report a correct filter as unsupported.
func known(allowed []string, value string) bool {
	value = strings.TrimSpace(value)
	return slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, value) })
}

// keepKnown drops the values of a facet param the catalogue cannot filter on.
//
// A facet with one wrong value keeps its right ones: dropping the whole facet would throw
// away a filter the caller got correct, and answer wider than either reading asked for.
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

// isCountryCode accepts the shape of an ISO 3166-1 alpha-2 code rather than a list of them.
// The mistake worth catching is "germany" for "DE" — a country NAME where a code belongs —
// and that is a shape error. Policing which codes exist would be a third geography
// dictionary, and internal/dict/location already owns that question.
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
