package handler

import (
	"context"
	"net/url"
	"slices"
	"sort"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

// facetCounter is the analytics backend the facets handler depends on. It is
// deliberately separate from searcher: counting facet distributions is a
// distinct responsibility from returning ranked hits, so the handler depends
// only on the method it uses. *search.Client satisfies both; a nil counter
// means search is unconfigured and the endpoint reports 503.
type facetCounter interface {
	FacetCounts(ctx context.Context, p search.FacetParams) (search.FacetResult, error)
	// DisjunctiveFacetCounts counts each facet under its own reduced filter (see
	// the disjunctive mode of JobFacets) plus the total under the full filter.
	DisjunctiveFacetCounts(ctx context.Context, query string, reqs []search.FacetReq, totalFilter any) (search.FacetResult, error)
}

// facetExtra describes a facetable attribute that is not a string-equality facet
// in search.StringFacets. statOnly marks a continuous numeric facet exposed only
// as min/max stats: Meili always also returns a per-value distribution for a
// faceted attribute, but a bucket per distinct salary is noise, so it is dropped.
type facetExtra struct {
	attr     string
	statOnly bool
}

// facetExtraParams maps a public query-param to its facetExtra for the boolean
// visa facet (distribution kept) and the continuous numeric facets (stats only).
// Single source of truth for which extras are stat-only.
var facetExtraParams = map[string]facetExtra{
	"visa_sponsorship":     {attr: "enrichment.visa_sponsorship"},
	"salary_min":           {attr: "enrichment.salary_min", statOnly: true},
	"salary_max":           {attr: "enrichment.salary_max", statOnly: true},
	"experience_years_min": {attr: "enrichment.experience_years_min", statOnly: true},
}

// facetsNoDistribution are string facets we filter on but never request a value
// distribution for. company_slug has thousands of values; the sidebar company
// filter is a server-backed typeahead (GET /api/v1/companies, count-ordered), not
// a facet distribution — and Meili would only return a capped, alphabetical slice
// anyway. It stays in search.StringFacets so `?company_slug=` filtering still
// works; we just stop computing a distribution the UI no longer reads.
var facetsNoDistribution = map[string]bool{"company_slug": true}

// facetAttributes is the list of index attributes to request facets for: every
// string facet (the same attributes search.StringFacets filters on), minus the
// ones in facetsNoDistribution, plus the extras. Sorted for a deterministic
// request. This is the single source shared with the search filter vocabulary —
// a new facet added to search.StringFacets is counted here automatically.
func facetAttributes() []string {
	attrs := make([]string, 0, len(search.StringFacets)+len(facetExtraParams))
	for param, attr := range search.StringFacets {
		if facetsNoDistribution[param] {
			continue
		}
		attrs = append(attrs, attr)
	}
	for _, e := range facetExtraParams {
		attrs = append(attrs, e.attr)
	}
	sort.Strings(attrs)
	return attrs
}

// locationFacetParams are the geography facets that share ONE OR-group in
// FilterFromValues (their included values widen the results together, not
// intersect). Disjunctive counting of any one of them must drop the whole group's
// contribution, not just that param — otherwise selecting a country would zero
// every sibling region (the reverse of what disjunctive mode is for).
var locationFacetParams = []string{"regions", "countries", "cities"}

// facetReqs builds one disjunctive request per distribution attribute: each
// counted under the filter with its own facet's params removed, so a facet's
// selection doesn't hide its alternatives. For a location facet, the whole
// location OR-group is removed (see locationFacetParams).
func facetReqs(vals url.Values) []search.FacetReq {
	param := facetParamByAttr()
	attrs := facetAttributes()
	reqs := make([]search.FacetReq, 0, len(attrs))
	for _, attr := range attrs {
		drop := []string{param[attr]}
		if slices.Contains(locationFacetParams, param[attr]) {
			drop = locationFacetParams
		}
		reqs = append(reqs, search.FacetReq{
			Attr:   attr,
			Filter: search.FilterFromValues(withoutParams(vals, drop)),
		})
	}
	return reqs
}

// withoutParams returns a copy of vals with each named facet's params dropped (the
// bare param plus its `_exclude` / `_mode` variants), leaving every other facet
// intact.
func withoutParams(vals url.Values, params []string) url.Values {
	drop := make(map[string]bool, len(params)*3)
	for _, p := range params {
		drop[p], drop[p+"_exclude"], drop[p+"_mode"] = true, true, true
	}
	out := make(url.Values, len(vals))
	for k, v := range vals {
		if drop[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// facetParamByAttr inverts the facet vocabulary (index attribute → public query
// param) so the response is keyed the way clients filter: "enrichment.seniority"
// is exposed as "seniority", hiding the index's internal dot-path structure.
func facetParamByAttr() map[string]string {
	m := make(map[string]string, len(search.StringFacets)+len(facetExtraParams))
	for param, attr := range search.StringFacets {
		m[attr] = param
	}
	for param, e := range facetExtraParams {
		m[e.attr] = param
	}
	return m
}

// JobFacets reports the count of vacancies per facet value under the given
// filters (the same query params as SearchJobs), instead of a page of jobs. It
// is public like the other job reads. The response is keyed by the public facet
// param names. Response: {"data": {total, facets, stats}}.
func (a *API) JobFacets(c *fiber.Ctx) error {
	if a.facets == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	q := c.Query("q")
	var res search.FacetResult
	var err error
	if c.QueryBool("disjunctive") {
		// Disjunctive: each facet counted under the full filter minus its own
		// selection, so a selected facet still shows its siblings (the live-modal
		// experience). The total stays the full-filter count.
		vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
		res, err = a.facets.DisjunctiveFacetCounts(c.Context(), q, facetReqs(vals), search.FilterFromValues(vals))
	} else {
		res, err = a.facets.FacetCounts(c.Context(), search.FacetParams{
			Query:  q,
			Filter: buildSearchFilter(c),
			Facets: facetAttributes(),
		})
	}
	if err != nil {
		return err
	}

	param := facetParamByAttr()

	// Re-key distributions to public param names, dropping the noisy per-value
	// distribution of the continuous numeric facets (kept only as stats below).
	facets := make(map[string]map[string]int64, len(res.Facets))
	for attr, dist := range res.Facets {
		p, ok := param[attr]
		if !ok || facetExtraParams[p].statOnly {
			continue
		}
		facets[p] = dist
	}

	// Re-key numeric stats to public param names.
	stats := make(map[string]search.FacetStat, len(res.Stats))
	for attr, st := range res.Stats {
		if p, ok := param[attr]; ok {
			stats[p] = st
		}
	}

	return c.JSON(fiber.Map{"data": fiber.Map{
		"total":  res.Total,
		"facets": facets,
		"stats":  stats,
	}})
}
