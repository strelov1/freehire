package search

import (
	"net/url"
	"strconv"
	"time"
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
	"education_level":  "enrichment.education_level",
	"seniority":        "enrichment.seniority",
	"category":         "enrichment.category",
	"domains":          "enrichment.domains",
	"countries":        "countries",
	"cities":           "cities",
	"company_type":     "enrichment.company_type",
	"company_size":     "enrichment.company_size",
	"salary_currency":  "enrichment.salary_currency",
	"salary_period":    "enrichment.salary_period",
	"skills":           "skills",
	"collections":      "collections",
	"relocation":       "enrichment.relocation",
	"english_level":    "enrichment.english_level",
	"posting_language": "enrichment.posting_language",
}

// RegionUnspecified is the reserved value of the `regions` facet that selects
// jobs with no resolved geography (an empty regions array) rather than a real
// region code. The SPA's "Not specified" region chip serializes to it. It maps to
// Meilisearch's IS EMPTY (IS NOT EMPTY when excluded), so it ORs with real region
// values in the same facet group and supports exclude like any region — replacing
// the former materialized `remote_unspecified` boolean with a query-time predicate.
const RegionUnspecified = "none"

// facetEq builds a facet's include fragment: an equality, except the regions
// unspecified sentinel, which becomes IS EMPTY.
func facetEq(param, attr, val string) string {
	if param == "regions" && val == RegionUnspecified {
		return IsEmpty(attr)
	}
	return Eq(attr, val)
}

// facetNeq builds a facet's exclude fragment: an inequality, except the regions
// unspecified sentinel, which becomes IS NOT EMPTY.
func facetNeq(param, attr, val string) string {
	if param == "regions" && val == RegionUnspecified {
		return IsNotEmpty(attr)
	}
	return Neq(attr, val)
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
	return filterFromValues(v, time.Now())
}

// filterFromValues is FilterFromValues with the reference time injected, so the
// relative `posted_within_days` cutoff is deterministic under test. The exported
// wrapper supplies time.Now(); only this inner form is unit-tested for the date
// branch.
func filterFromValues(v url.Values, now time.Time) any {
	var groups [][]string

	for param, attr := range StringFacets {
		if included := nonEmpty(v[param]); len(included) > 0 {
			if v.Get(param+"_mode") == "and" {
				// Each value its own AND group: a job must match all of them.
				for _, val := range included {
					groups = append(groups, []string{facetEq(param, attr, val)})
				}
			} else {
				group := make([]string, len(included))
				for i, val := range included {
					group[i] = facetEq(param, attr, val)
				}
				groups = append(groups, group)
			}
		}
		// Excluded values: each is its own AND group so all are filtered out.
		for _, val := range nonEmpty(v[param+"_exclude"]) {
			groups = append(groups, []string{facetNeq(param, attr, val)})
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

	// Freshness: posted_within_days=N restricts to jobs posted in the last N days,
	// i.e. whose effective posting date (posted_ts, unix seconds) is at or after
	// now - N*86400. A non-positive or non-numeric value imposes no restriction.
	if n, ok := atoiOK(v.Get("posted_within_days")); ok && n > 0 {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("posted_ts", int(cutoff))})
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
