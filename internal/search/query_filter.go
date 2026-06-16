package search

import (
	"net/url"
	"strconv"
)

// StringFacets maps an equality-facet query param to its index attribute. It is
// the single source of truth for the search index's string-facet vocabulary,
// shared by the HTTP search/facets handlers and the notification matcher.
// Enrichment facets live under the nested "enrichment" object, so they filter on
// a dot path; geography (regions/countries), work_mode and skills are resolved
// facets served top-level, so they filter on a bare attribute. Repeated params
// (?seniority=a&seniority=b) are ORed.
var StringFacets = map[string]string{
	"source":           "source",
	"company_slug":     "company_slug",
	"regions":          "regions",
	"work_mode":        "work_mode",
	"employment_type":  "enrichment.employment_type",
	"seniority":        "enrichment.seniority",
	"category":         "enrichment.category",
	"domains":          "enrichment.domains",
	"countries":        "countries",
	"company_type":     "enrichment.company_type",
	"company_size":     "enrichment.company_size",
	"salary_currency":  "enrichment.salary_currency",
	"salary_period":    "enrichment.salary_period",
	"skills":           "skills",
	"relocation":       "enrichment.relocation",
	"english_level":    "enrichment.english_level",
	"posting_language": "enrichment.posting_language",
}

// FilterFromValues turns the facet params of a parsed search query into a
// Meilisearch filter. Within a facet, included values are ORed by default (or
// ANDed when `<param>_mode=and`); excluded values (`<param>_exclude=...`) become
// NOT fragments. Facets are ANDed. Returns nil when no facet is set.
//
// It is pure (no *fiber.Ctx), so the HTTP handler and the notification matcher
// build identical filters from the same canonical query string — the handler
// parses the request query, the matcher parses a saved search's stored query.
func FilterFromValues(v url.Values) any {
	var groups [][]string

	for param, attr := range StringFacets {
		if included := nonEmpty(v[param]); len(included) > 0 {
			if v.Get(param+"_mode") == "and" {
				// Each value its own AND group: a job must match all of them.
				for _, val := range included {
					groups = append(groups, []string{Eq(attr, val)})
				}
			} else {
				group := make([]string, len(included))
				for i, val := range included {
					group[i] = Eq(attr, val)
				}
				groups = append(groups, group)
			}
		}
		// Excluded values: each is its own AND group so all are filtered out.
		for _, val := range nonEmpty(v[param+"_exclude"]) {
			groups = append(groups, []string{Neq(attr, val)})
		}
	}

	if raw := v.Get("visa_sponsorship"); raw != "" {
		groups = append(groups, []string{EqBool("enrichment.visa_sponsorship", raw == "true")})
	}

	if n, ok := atoiOK(v.Get("salary_min")); ok {
		groups = append(groups, []string{Gte("enrichment.salary_min", n)})
	}
	if n, ok := atoiOK(v.Get("salary_max")); ok {
		groups = append(groups, []string{Lte("enrichment.salary_max", n)})
	}
	if n, ok := atoiOK(v.Get("experience_years_min")); ok {
		groups = append(groups, []string{Gte("enrichment.experience_years_min", n)})
	}

	return Filter(groups...)
}

// nonEmpty drops empty strings so a bare `?seniority=` emits no fragment.
func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// atoiOK reports whether a query value is a valid integer, so a missing or
// non-numeric value emits no bogus numeric fragment.
func atoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
