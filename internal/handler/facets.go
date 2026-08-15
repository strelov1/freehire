package handler

import (
	"context"
	"net/url"
	"slices"
	"sort"
	"strings"

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
//
// `only` narrows the set to the named public params. Counting a distribution is
// paid per attribute and the expensive ones are the wide-valued ones: measured on
// prod against `?work_mode=remote`, the full set costs 284ms and returns 99KB,
// dropping `cities` takes it to 233ms, dropping `skills` too 148ms, and a single
// attribute is 10ms. A caller that reads one number off the result — the job
// page's "see also" block reads exactly one per collection — should not pay for
// the other twenty-six.
func facetAttributes(only ...string) []string {
	var want map[string]bool
	if len(only) > 0 {
		want = make(map[string]bool, len(only))
		for _, p := range only {
			want[p] = true
		}
	}
	keep := func(param string) bool { return want == nil || want[param] }

	attrs := make([]string, 0, len(search.StringFacets)+len(facetExtraParams))
	for param, attr := range search.StringFacets {
		if facetsNoDistribution[param] || !keep(param) {
			continue
		}
		attrs = append(attrs, attr)
	}
	for param, e := range facetExtraParams {
		if !keep(param) {
			continue
		}
		attrs = append(attrs, e.attr)
	}
	sort.Strings(attrs)
	return attrs
}

// requestedFacets reads the optional `facets=` param: a comma-separated list of
// public facet params to count, instead of all of them.
//
// An unknown name is an error rather than a silent no-op. The counts this
// endpoint returns are read by key, so a typo would otherwise surface as a
// missing count — indistinguishable from a value Meili's per-facet cap dropped,
// which callers are expected to treat as "unknown" rather than as a bug.
// company_slug is rejected for the same reason it is absent from the full set:
// thousands of values, and the UI uses the typeahead instead.
func requestedFacets(c *fiber.Ctx) ([]string, error) {
	raw := c.Query("facets")
	if raw == "" {
		return nil, nil
	}
	params := strings.Split(raw, ",")
	out := make([]string, 0, len(params))
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		_, isString := search.StringFacets[p]
		_, isExtra := facetExtraParams[p]
		if (!isString && !isExtra) || facetsNoDistribution[p] {
			return nil, fiber.NewError(fiber.StatusBadRequest, "unknown facet: "+p)
		}
		out = append(out, p)
	}
	return out, nil
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
func (h *searchHandlers) JobFacets(c *fiber.Ctx) error {
	if h.facets == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	q := c.Query("q")
	only, err := requestedFacets(c)
	if err != nil {
		return err
	}

	var res search.FacetResult
	if c.QueryBool("disjunctive") {
		// Disjunctive: each facet counted under the full filter minus its own
		// selection, so a selected facet still shows its siblings (the live-modal
		// experience). The total stays the full-filter count.
		//
		// `facets=` is refused here rather than ignored. Disjunctive counting
		// derives one query per facet from the SELECTION, so narrowing the output
		// set would not save the queries — the caller would pay full price for a
		// partial answer, which is the opposite of what asking for it means.
		if len(only) > 0 {
			return fiber.NewError(fiber.StatusBadRequest, "facets= cannot be combined with disjunctive")
		}
		vals := queryValues(c)
		res, err = h.facets.DisjunctiveFacetCounts(c.Context(), q, facetReqs(vals), search.FilterFromValues(vals))
	} else {
		res, err = h.facets.FacetCounts(c.Context(), search.FacetParams{
			Query:  q,
			Filter: buildSearchFilter(c),
			Facets: facetAttributes(only...),
		})
	}
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": facetView(res)})
}

// facetPayload is the public shape of a facet result: distributions and numeric
// stats keyed by the query-param names callers filter by, never by the internal
// Meilisearch attributes.
type facetPayload struct {
	Total  int64                       `json:"total"`
	Facets map[string]map[string]int64 `json:"facets"`
	Stats  map[string]search.FacetStat `json:"stats"`
}

// facetView re-keys a raw facet result to the public param names. It is shared by
// the HTTP endpoint and the assistant's facets tool, so the vocabulary the agent
// filters by is exactly the vocabulary the API publishes.
func facetView(res search.FacetResult) facetPayload {
	param := facetParamByAttr()

	// Distributions, dropping the noisy per-value distribution of the continuous
	// numeric facets (kept only as stats below).
	facets := make(map[string]map[string]int64, len(res.Facets))
	for attr, dist := range res.Facets {
		p, ok := param[attr]
		if !ok || facetExtraParams[p].statOnly {
			continue
		}
		facets[p] = dist
	}

	stats := make(map[string]search.FacetStat, len(res.Stats))
	for attr, st := range res.Stats {
		if p, ok := param[attr]; ok {
			stats[p] = st
		}
	}
	return facetPayload{Total: res.Total, Facets: facets, Stats: stats}
}
