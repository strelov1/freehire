package handler

import (
	"context"
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

// facetAttributes is the full list of index attributes to request facets for:
// every string facet (the same attributes search.StringFacets filters on) plus
// the extras. Sorted for a deterministic request. This is the single source
// shared with the search filter vocabulary — a new facet added to
// search.StringFacets is counted here automatically.
func facetAttributes() []string {
	attrs := make([]string, 0, len(search.StringFacets)+len(facetExtraParams))
	for _, attr := range search.StringFacets {
		attrs = append(attrs, attr)
	}
	for _, e := range facetExtraParams {
		attrs = append(attrs, e.attr)
	}
	sort.Strings(attrs)
	return attrs
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

	res, err := a.facets.FacetCounts(c.Context(), search.FacetParams{
		Query:  c.Query("q"),
		Filter: buildSearchFilter(c),
		Facets: facetAttributes(),
	})
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
