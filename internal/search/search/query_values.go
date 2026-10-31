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

// boolValues is the vocabulary of the boolean facets. The true-or-absent ones read
// anything but "false" as true (see trueOrAbsentFragment), so a typo there is not
// dropped but MISREAD — `requires_clearance=flase` asks for exactly the postings the
// caller meant to exclude — which is the stronger reason to name it.
var boolValues = []string{"true", "false"}

// closedFacets maps each facet whose values come from a closed vocabulary to the check
// that a value belongs to it — the dictionary itself, never a copy, so a value added to
// the dictionary joins this check by existing. A value outside it matches no posting, so
// the filter narrows the answer to nothing and the empty page reads exactly like an empty
// market.
//
// skills is in the table although its vocabulary is not a vocab list: the facet is written
// by internal/dict/skilltag alone (measured 2026-10-27, 98 stray values carried by 147
// postings beside 12.27M canonical tags), so a value skilltag cannot resolve is a value
// no posting carries. The check is skilltag.Resolve, the same reading the filter makes
// (see facetValue), so "golang" is known — it reaches the postings tagged "go".
//
// The facets NOT listed have no finite list to check against, and writing one here would
// be a second dictionary drifting from the real one:
//   - cities, company_slug: open by nature; an unknown value can be legitimate.
//   - countries: any ISO code is a valid ask, including one no posting carries yet.
//   - source: the adapter registry grows, and non-adapter sources (telegram) exist.
//   - collections: curated tags kept in data, not in code.
//   - salary_currency, posting_language: open code systems (ISO 4217, ISO 639).
//
// Every value is read through splitFacetValues, the same way filterFromValues reads it,
// so the report and the filter cannot disagree about what was asked.
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
	"reality": oneOf([]string{
		jobreality.ClassFresh, jobreality.ClassStale, jobreality.ClassLikelyEvergreen,
	}),
	"skills": func(val string) bool {
		_, ok := skilltag.Resolve(val)
		return ok
	},
	"is_tech":               oneOf(boolValues),
	RequiresClearanceParam:  oneOf(boolValues),
	ViaRecruiterParam:       oneOf(boolValues),
	AIInterviewParam:        oneOf(boolValues),
	AutoApplyAvailableParam: oneOf(boolValues),
}

// UnknownFacetValue reports whether value falls outside the closed vocabulary of a facet
// param (or its `_exclude` form) — the value IgnoredValues would name. false for a param
// with no closed vocabulary: an open facet has nothing to be unknown against.
//
// It is how a surface that DROPS such a value (the MCP app, whose reader is a language
// model) asks the same question this package's report answers, so the two cannot
// disagree about which values are known.
func UnknownFacetValue(param, value string) bool {
	known, closed := closedFacets[strings.TrimSuffix(param, "_exclude")]
	return closed && !known(value)
}

// IgnoredValues reports, per query param, the values of a closed-vocabulary facet (or its
// `_exclude` form) that fall outside the vocabulary. nil when there are none.
//
// It is the value-level counterpart of UnknownParams and follows the same rule: report,
// never refuse. A saved search or a shared link may carry a value the vocabulary has since
// retired, and a 400 would break it; staying silent is what made `work_mode=foo` answer
// `total: 0` indistinguishable from a market with no such postings. The filter is left
// alone — the value still reaches it — so the report describes the answer rather than
// changing it.
//
// A value is compared trimmed and case-insensitively, the same check the MCP app makes
// (mcpapp's known): a value the filter could match must never be named as ignored, and a
// near-miss in spelling is the caller's to fix, not ours to guess.
//
// Bounded like UnknownParams — every reported value is copied from a public, anonymous
// request into the response — to maxUnknownParamsReported params of at most
// maxUnknownParamsReported values each, chosen in sorted order so the report is stable.
func IgnoredValues(v url.Values) map[string][]string {
	var params []string
	out := map[string][]string{}
	for facet, known := range closedFacets {
		for _, param := range []string{facet, facet + "_exclude"} {
			var bad []string
			for _, val := range splitFacetValues(v[param]) {
				if !known(val) && !slices.Contains(bad, val) {
					bad = append(bad, val)
				}
			}
			if len(bad) == 0 {
				continue
			}
			sort.Strings(bad)
			if len(bad) > maxUnknownParamsReported {
				bad = bad[:maxUnknownParamsReported]
			}
			out[param] = bad
			params = append(params, param)
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

// oneOf is the check for a vocabulary given as a list. A value is compared trimmed and
// case-insensitively (see IgnoredValues).
func oneOf(allowed []string) func(string) bool {
	return func(value string) bool {
		value = strings.TrimSpace(value)
		return slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, value) })
	}
}
