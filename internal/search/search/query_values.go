package search

import (
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobreality"
)

// boolValues is the vocabulary of the boolean facets. The true-or-absent ones read anything
// but "false" as true (see trueOrAbsentFragment), so a typo is MISREAD, not dropped:
// `requires_clearance=flase` asks for the postings the caller meant to exclude.
var boolValues = []string{"true", "false"}

// closedFacets maps each closed-vocabulary facet to the check that a value belongs to it,
// reading the dictionary itself, never a copy. A value outside it matches no posting, so
// an empty page would look like an empty market.
//
// skills is here although its vocabulary is not a vocab list: only internal/dict/skilltag
// writes the facet, so a value skilltag.Resolve cannot resolve is carried by no posting.
// That is the filter's own reading (see facetValue), so "golang" is known (it reaches "go").
//
// Facets NOT listed have no finite list, and a copy here would drift from the real one:
//   - cities, company_slug: open by nature; an unknown value can be legitimate.
//   - countries: any ISO code is a valid ask, including one no posting carries yet.
//   - source: the adapter registry grows, and non-adapter sources (telegram) exist.
//   - collections: curated tags kept in data, not in code.
//   - salary_currency, posting_language: open code systems (ISO 4217, ISO 639).
//
// Values are read through splitFacetValues, as filterFromValues does, so the report and
// the filter agree on what was asked.
var closedFacets = map[string]func(string) bool{
	"work_mode": oneOf(vocab.WorkModeValues),
	// RegionUnspecified is a reserved value the filter maps to IS EMPTY.
	"regions":   oneOf(append(slices.Clone(vocab.RegionValues), RegionUnspecified)),
	"seniority": oneOf(vocab.SeniorityValues),
	// ClearanceLevelUnspecified is a reserved value the filter maps to NOT EXISTS.
	"clearance_levels": oneOf(append(slices.Clone(vocab.ClearanceLevelValues), ClearanceLevelUnspecified)),
	// The retired category is translated, not dropped (see retiredCategory).
	"category":        oneOf(append(slices.Clone(vocab.CategoryValues), vocab.RetiredCategoryManagement)),
	"employment_type": oneOf(vocab.EmploymentTypeValues),
	"education_level": oneOf(vocab.EducationLevelValues),
	"company_type":    oneOf(vocab.CompanyTypeValues),
	"company_size":    oneOf(vocab.CompanySizeValues),
	"salary_period":   oneOf(vocab.SalaryPeriodValues),
	"relocation":      oneOf(vocab.RelocationValues),
	"english_level":   oneOf(vocab.EnglishLevelValues),
	"role_type":       oneOf(vocab.RoleTypeValues),
	"source_type":     oneOf(vocab.SourceTypeValues),
	"ai_archetype":    oneOf(vocab.AIArchetypeValues),
	"domains":         oneOf(vocab.DomainValues),
	"perks":           oneOf(vocab.PerkValues),
	"engagement":      oneOf(vocab.EngagementValues),
	"reality": oneOf([]string{
		jobreality.ClassFresh, jobreality.ClassStale, jobreality.ClassLikelyEvergreen,
	}),
	"skills": func(val string) bool {
		_, ok := skilltag.Resolve(val)
		return ok
	},
	// is_tech filters on the served "tech" / "non_tech" string (jobview.isTechFacet), not a boolean.
	"is_tech":              oneOf([]string{"tech", "non_tech"}),
	RequiresClearanceParam: oneOf(boolValues),
	ViaRecruiterParam:      oneOf(boolValues),
	// EquityNotStated is a reserved value the filter maps to NOT EXISTS.
	EquityParam:             oneOf(append(slices.Clone(vocab.EquityValues), EquityNotStated)),
	AIInterviewParam:        oneOf(boolValues),
	AutoApplyAvailableParam: oneOf(boolValues),
}

// checkedScalars are the scalar filters and modifiers whose values IgnoredValues checks.
// The older numeric bounds still drop bad values silently.
var checkedScalars = map[string]func(string) bool{
	ApplyQuestionsMaxParam: func(val string) bool {
		_, ok := applyQuestionsMax(val)
		return ok
	},
	// Anything but a case-insensitive true leaves the modifier unset, which widens the
	// answer, so a mis-spelled value is named.
	WorkModeRemoteAnywhereParam:             oneOf(boolValues),
	"visa_sponsorship":                      oneOf(boolValues),
	SeniorityIncludeUnspecifiedParam:        oneOf(boolValues),
	ExperienceIncludeUnspecifiedParam:       oneOf(boolValues),
	ExperienceOnlySeniorityUnspecifiedParam: oneOf(boolValues),
}

// UnknownFacetValue reports whether value falls outside the closed vocabulary of a facet
// param (or its `_exclude` form), i.e. the value IgnoredValues would name. False for an
// open facet. A surface that DROPS such values (the MCP app) uses it to agree with the report.
func UnknownFacetValue(param, value string) bool {
	known, closed := closedFacets[strings.TrimSuffix(param, "_exclude")]
	return closed && !known(value)
}

// IgnoredValues reports, per query param, the values of a closed-vocabulary facet (or its
// `_exclude` form) outside the vocabulary. nil when there are none.
//
// Like UnknownParams it reports and never refuses: a saved search may carry a since-retired
// value, and silence made `work_mode=foo` look like an empty market. The filter is left
// alone; the value still reaches it.
//
// Values are compared trimmed and case-insensitively, as the MCP app does, so a value the
// filter could match is never named as ignored.
//
// Bounded like UnknownParams (reported values are copied from an anonymous request): at most
// maxUnknownParamsReported params of as many values each, in sorted order.
func IgnoredValues(v url.Values) map[string][]string {
	var params []string
	out := map[string][]string{}
	report := func(param string, vals []string, known func(string) bool) {
		var bad []string
		for _, val := range vals {
			if !known(val) && !slices.Contains(bad, val) {
				bad = append(bad, val)
			}
		}
		if len(bad) == 0 {
			return
		}
		sort.Strings(bad)
		if len(bad) > maxUnknownParamsReported {
			bad = bad[:maxUnknownParamsReported]
		}
		out[param] = bad
		params = append(params, param)
	}
	for facet, known := range closedFacets {
		for _, param := range []string{facet, facet + "_exclude"} {
			report(param, splitFacetValues(v[param]), known)
		}
	}
	// The remote reach reads each value as the location facet it names.
	for param, facet := range remoteReachFacets {
		if known, closed := closedFacets[facet]; closed {
			report(param, splitFacetValues(v[param]), known)
		}
	}
	// A comma-joined pair is not an and/or.
	validMode := oneOf([]string{"and", "or"})
	for facet := range StringFacets {
		param := facet + "_mode"
		if raw, ok := v[param]; ok && len(raw) > 0 {
			report(param, raw[:1], validMode)
		}
	}
	// A scalar is read whole (v.Get), so it is checked whole, not split.
	for param, valid := range checkedScalars {
		if raw, ok := v[param]; ok && len(raw) > 0 {
			report(param, raw[:1], valid)
		}
	}
	if len(params) == 0 {
		return nil
	}
	sort.Strings(params)
	for _, param := range params[min(len(params), maxUnknownParamsReported):] {
		delete(out, param)
	}
	return out
}

// IgnoredNumericBounds reports numeric bounds that FilterFromValues would drop.
// Callers that require a strict match can reject them before searching.
func IgnoredNumericBounds(v url.Values) []UnknownParam {
	var ignored []UnknownParam
	for _, param := range []string{
		"salary_min", "salary_max", "experience_years_min", "experience_years_max", "posted_within_days", "open_within_days",
	} {
		if !v.Has(param) {
			continue
		}
		n, valid := atoiOK(v.Get(param))
		switch param {
		case "experience_years_max":
			valid = valid && n >= 0
		case "posted_within_days", "open_within_days":
			valid = valid && withinDays(n)
		}
		if !valid {
			ignored = append(ignored, UnknownParam{Param: param})
		}
	}
	return ignored
}

// oneOf checks a value against a list, trimmed and case-insensitively.
func oneOf(allowed []string) func(string) bool {
	return func(value string) bool {
		value = strings.TrimSpace(value)
		return slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, value) })
	}
}
