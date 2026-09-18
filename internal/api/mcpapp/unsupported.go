package mcpapp

import (
	"net/url"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
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
// Only CLOSED vocabularies are checked. Skills, cities, company slugs and sources have no
// finite list to check against, and writing one here would be a second dictionary drifting
// from the real ones — the failure this repository has already paid for more than once.

// closedVocabularies maps a query parameter to the values the catalogue can actually filter
// on. The lists are the dictionaries themselves, never a copy: a value added to vocab joins
// this check by existing.
var closedVocabularies = map[string][]string{
	"work_mode":       vocab.WorkModeValues,
	"seniority":       vocab.SeniorityValues,
	"category":        vocab.CategoryValues,
	"employment_type": vocab.EmploymentTypeValues,
	"english_level":   vocab.EnglishLevelValues,
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
	for param, allowed := range closedVocabularies {
		for _, value := range in.valuesFor(param) {
			if !known(allowed, value) {
				out = append(out, param+"="+value)
			}
		}
	}
	for _, value := range in.Countries {
		if !isCountryCode(value) {
			out = append(out, "countries="+value)
		}
	}
	// Sorted so the answer is stable: an unordered list would make two identical requests
	// produce two different answers, which is a difference a model will try to interpret.
	slices.Sort(out)
	return out
}

// valuesFor is the one place that knows which field a parameter is read from, so Unsupported
// and QueryValues cannot disagree about what was checked and what was sent.
func (in SearchInput) valuesFor(param string) []string {
	switch param {
	case "work_mode":
		return in.WorkMode
	case "seniority":
		return in.Seniority
	case "category":
		return in.Category
	case "employment_type":
		return in.EmploymentType
	case "english_level":
		return in.EnglishLevel
	}
	return nil
}

// setChecked writes only the values the catalogue can filter on.
//
// A facet with one wrong value keeps its right ones: dropping the whole facet would throw
// away a filter the caller got correct, and answer wider than either reading asked for.
func setChecked(v url.Values, param string, values []string) {
	allowed, checked := closedVocabularies[param]
	if !checked {
		setList(v, param, values)
		return
	}
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if known(allowed, value) {
			kept = append(kept, value)
		}
	}
	setList(v, param, kept)
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
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return true
}
