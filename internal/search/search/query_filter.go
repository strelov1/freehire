package search

import (
	"net/url"
	"strconv"
	"strings"
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
	"source":          "source",
	"company_slug":    "company_slug",
	"regions":         "regions",
	"work_mode":       "work_mode",
	"employment_type": "enrichment.employment_type",
	"education_level": "enrichment.education_level",
	"seniority":       "enrichment.seniority",
	"category":        "enrichment.category",
	"domains":         "enrichment.domains",
	"countries":       "countries",
	"cities":          "cities",
	"company_type":    "enrichment.company_type",
	"company_size":    "enrichment.company_size",
	"salary_currency": "enrichment.salary_currency",
	"salary_period":   "enrichment.salary_period",
	"skills":          "skills",
	"is_tech":         "is_tech",
	// requires_clearance is a true-or-absent boolean (see jobview), so facetEq
	// special-cases the "false" value rather than emitting an equality against a
	// value the index never carries.
	//
	// Being in this map also enrols it in the /jobs/facets distribution (see
	// handler.facetAttributes, which reads this map), which is why the Meilisearch
	// settings patch declaring the attribute filterable MUST reach the live index
	// before a binary carrying this line does — otherwise every facets request 500s.
	"requires_clearance": "requires_clearance",
	"ai_archetype":       "ai_archetype",
	// Derived at index time like the two above, so it filters on the bare attribute.
	// The vocabulary holds one value, which makes `role_type_exclude` the way to ask
	// for postings with no management marker — NOT a positive individual-contributor
	// filter, a distinction the OpenAPI description spells out for integrators.
	"role_type":        "role_type",
	"reality":          "reality.class",
	"collections":      "collections",
	"relocation":       "enrichment.relocation",
	"english_level":    "enrichment.english_level",
	"posting_language": "enrichment.posting_language",
}

// locationFacets are the geography params that describe one user-facing concept
// ("where"), so their included values OR into a single group rather than ANDing
// across facets — selecting the "Global" region and "Brazil" widens the results
// (Global OR Brazil) instead of intersecting them to zero. They share the SPA's
// one Location pane. Excludes stay per-value AND groups like every other facet,
// and a non-location facet (e.g. work_mode) still ANDs with the whole group, so
// "remote AND (Europe OR Brazil)" holds. Geographic AND is nonsensical ("in
// Europe AND in LATAM" is empty), so the `_mode=and` override does not apply here.
var locationFacets = map[string]bool{"regions": true, "countries": true, "cities": true}

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
	if param == RequiresClearanceParam {
		return clearanceFragment(attr, val)
	}
	return Eq(attr, val)
}

// RequiresClearanceParam is the facet whose stored value is true-or-absent: a
// posting that states a government clearance requirement carries it, and every other
// posting — the vast majority — carries no such attribute at all.
const RequiresClearanceParam = "requires_clearance"

// clearanceFragment builds the requires_clearance predicate. "false" cannot be an
// equality: nothing in the index is ever written false, so `requires_clearance =
// false` matches nothing and would silently empty a result set the caller expected to
// be nearly the whole catalogue. Negating the positive is what actually answers
// "everything not marked", including the documents that omit the attribute.
func clearanceFragment(attr, val string) string {
	if val == "false" {
		return "NOT " + EqBool(attr, true)
	}
	return EqBool(attr, true)
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
// NOT fragments. Facets are ANDed, except the location facets (regions/countries/
// cities), whose includes OR into one shared group (see locationFacets). Returns
// nil when no facet is set.
//
// It is pure (no *fiber.Ctx), so the HTTP handler and the notification matcher
// build identical filters from the same canonical query string — the handler
// parses the request query, the matcher parses a saved search's stored query.
func FilterFromValues(v url.Values) any {
	return filterFromValues(v, time.Now())
}

// maxWithinDays is the largest day count the two date bounds honour — a century, which
// is further back than any posting and than the catalogue itself.
//
// It exists for the arithmetic, not for the semantics. time.Duration is int64
// nanoseconds, so `time.Duration(n) * 24 * time.Hour` WRAPS above roughly 106,751 days:
// the cutoff landed in the future and the query matched nothing, which inverted the
// parameter — the most permissive bound anyone could type produced the emptiest result.
const maxWithinDays = 36500

// withinDays reports whether n is a day count the date bounds can honour. A value
// outside the range imposes no restriction, the same way a zero, a negative or a
// non-numeric one does: past a century the bound already means "any age", so dropping
// it says exactly what honouring it would have.
func withinDays(n int) bool { return n > 0 && n <= maxWithinDays }

// filterFromValues is FilterFromValues with the reference time injected, so the
// relative `posted_within_days` cutoff is deterministic under test. The exported
// wrapper supplies time.Now(); only this inner form is unit-tested for the date
// branch.
func filterFromValues(v url.Values, now time.Time) any {
	var groups [][]string
	// Included geography fragments across regions/countries/cities collect here and
	// are appended as one OR group (see locationFacets).
	var locationGroup []string

	for param, attr := range StringFacets {
		included := splitFacetValues(v[param])
		switch {
		case locationFacets[param]:
			for _, val := range included {
				locationGroup = append(locationGroup, facetEq(param, attr, val))
			}
		case len(included) > 0 && v.Get(param+"_mode") == "and":
			// Each value its own AND group: a job must match all of them.
			for _, val := range included {
				groups = append(groups, []string{facetEq(param, attr, val)})
			}
		case len(included) > 0:
			group := make([]string, len(included))
			for i, val := range included {
				group[i] = facetEq(param, attr, val)
			}
			groups = append(groups, group)
		}
		// Excluded values: each is its own AND group so all are filtered out.
		for _, val := range splitFacetValues(v[param+"_exclude"]) {
			groups = append(groups, []string{facetNeq(param, attr, val)})
		}
	}
	if len(locationGroup) > 0 {
		groups = append(groups, locationGroup)
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
	// Both experience bounds read the SAME attribute: `enrichment.experience_years_min`
	// is what the posting asks for, so these are a floor and a ceiling on that one ask,
	// not a min/max pair over two fields. Meili compares only documents carrying the
	// attribute, so either bound drops the postings that state no requirement — the
	// honest reading of "asks for at most N years".
	if n, ok := atoiOK(v.Get("experience_years_min")); ok {
		groups = append(groups, []string{Gte("enrichment.experience_years_min", n)})
	}
	// n >= 0 because a negative ceiling can only match nothing — the attribute is
	// never below zero — so it is a typo, and honouring it would render an empty
	// page that looks like a legitimately narrow search rather than a bad param.
	if n, ok := atoiOK(v.Get("experience_years_max")); ok && n >= 0 {
		groups = append(groups, []string{Lte("enrichment.experience_years_min", n)})
	}

	// Freshness: posted_within_days=N restricts to jobs posted in the last N days,
	// i.e. whose effective posting date (posted_ts, unix seconds) is at or after
	// now - N*86400. A non-positive or non-numeric value imposes no restriction.
	if n, ok := atoiOK(v.Get("posted_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("posted_ts", int(cutoff))})
	}

	// How long the posting has been open: open_within_days=N restricts to jobs whose
	// created_ts (the instant ingest first wrote the row) is at or after now - N*86400.
	// Same shape and same lenient parse as the bound above, and independent of it — a
	// request may carry both, and they AND.
	//
	// The two are NOT interchangeable, which is why both exist. posted_ts follows the
	// date the SOURCE states, and some boards restate it every crawl, so a posting open
	// for months passes a three-day posted bound. created_ts is the system's own
	// observation and cannot be rewritten from outside.
	if n, ok := atoiOK(v.Get("open_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("created_ts", int(cutoff))})
	}

	return Filter(groups...)
}

// splitFacetValues splits each raw query value on comma and flattens the
// result, dropping empty fragments (a stray comma, or a bare `?skills=`) — so
// a repeated key (`skills=go&skills=rust`) and a comma-joined value
// (`skills=go,rust`) resolve to the same set of facet values.
func splitFacetValues(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
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
