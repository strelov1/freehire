package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// facetCacheTTL bounds how stale a cached facet distribution may be. Facet counts
// are identical for every caller and change only when ingest lands a wave (a few
// times an hour), so a minute of staleness is invisible to a user while collapsing
// the repeated recomputation that the see-also block and crawler traffic drive —
// the endpoint measured at 55% of API handler time. It is deliberately short: the
// win is in absorbing bursts of identical requests, not in holding a count for long.
const facetCacheTTL = time.Minute

// cachedFacetCounts wraps facetCounter.FacetCounts with a best-effort read-through
// cache. A nil cache, a miss, or any cache error all fall straight through to a
// live count — the cache can never fail or refuse the request, only skip work.
// The key is a hash of the exact FacetParams (query, filter, requested attributes),
// so two requests share a cached result only when they would compute the identical
// one. The disjunctive path is intentionally not routed here: it is the interactive
// live-modal experience, lower volume, and its per-facet fan-out keys poorly.
func (h *searchHandlers) cachedFacetCounts(ctx context.Context, p search.FacetParams) (search.FacetResult, error) {
	return cachedFacetCounts(ctx, h.facets, h.cache, p)
}

// cachedFacetCounts is the cache-through facet read, a function rather than only a method so
// the ChatGPT app's job_market_stats shares the website's cache slots instead of computing
// the same distributions twice. A nil cache reads straight through.
func cachedFacetCounts(ctx context.Context, f facetCounter, c cache.Cache, p search.FacetParams) (search.FacetResult, error) {
	if c == nil {
		return countFacets(ctx, f, p)
	}

	key := facetCacheKey(p)
	if cached, found, err := cache.GetJSON[search.FacetResult](ctx, c, key); err == nil && found {
		return cached, nil
	}

	res, err := countFacets(ctx, f, p)
	if err != nil {
		return res, err
	}
	// Store is best-effort: a failed write just means the next request recomputes.
	_ = cache.SetJSON(ctx, c, key, res, facetCacheTTL)
	return res, nil
}

// companyAttr is the index attribute behind the company_slug facet.
var companyAttr = search.StringFacets[companyFacetParam]

// countFacets runs the facet query and, when the company distribution is among
// the requested attributes, the exact distinct-company count beside it. They are
// two concurrent requests and must stay two: `distinct` in the facet request
// would collapse every facet count to 1 (see search.Client.DistinctCount).
func countFacets(ctx context.Context, f facetCounter, p search.FacetParams) (search.FacetResult, error) {
	if !slices.Contains(p.Facets, companyAttr) {
		return f.FacetCounts(ctx, p)
	}
	var (
		res       search.FacetResult
		companies int64
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		res, err = f.FacetCounts(gctx, p)
		return err
	})
	g.Go(func() (err error) {
		companies, err = f.DistinctCount(gctx, p.Query, p.Filter, companyAttr)
		return err
	})
	if err := g.Wait(); err != nil {
		return search.FacetResult{}, err
	}
	res.Companies = &companies
	return res, nil
}

// facetCacheKey derives a deterministic cache key from the params that fully
// determine a facet result. It marshals FacetParams to JSON and hashes it, so the
// key is bounded in length regardless of how many filter values a request carries.
// FacetParams marshals in practice (Query string, Filter [][]string, Facets
// []string); the error branch is a guard, not a real path, and its fixed key just
// means any unmarshalable request degrades to sharing one cache slot rather than
// panicking.
//
// The filter must be canonicalized first. search.FilterFromValues builds the
// [][]string by ranging over a map (query_filter.go), so its group order — and the
// value order inside the location OR-group — is randomized per request. Left as-is,
// the SAME multi-facet request would hash to different keys on successive calls and
// miss its own cache entry, defeating the cache on exactly the filtered traffic it
// targets. AND across groups and OR within one are both order-independent, so
// sorting is safe for keying and never touches the filter actually sent to Meili.
func facetCacheKey(p search.FacetParams) string {
	p.Filter = canonicalFilter(p.Filter)
	raw, err := json.Marshal(p)
	if err != nil {
		return "jobfacets:v1:unkeyable"
	}
	sum := sha256.Sum256(raw)
	return "jobfacets:v1:" + hex.EncodeToString(sum[:])
}

// canonicalFilter returns f with a stable ordering when it is the [][]string the
// facet filter uses (sorted within each group, then across groups), so an
// order-only difference cannot change the cache key. Any other shape is returned
// unchanged — the key still reflects it, just without canonicalization.
func canonicalFilter(f any) any {
	groups, ok := f.([][]string)
	if !ok {
		return f
	}
	out := make([][]string, len(groups))
	for i, g := range groups {
		g = slices.Clone(g)
		slices.Sort(g)
		out[i] = g
	}
	slices.SortFunc(out, func(a, b []string) int {
		return slices.Compare(a, b)
	})
	return out
}

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
	// DistinctCount is the exact number of distinct values of attr under the
	// filter — how many companies a search spans, past the facet value cap.
	DistinctCount(ctx context.Context, query string, filter any, attr string) (int64, error)
}

// companyNamer resolves display names for company slugs in one read.
// *db.Queries satisfies it; a nil one names every company by its slug.
type companyNamer interface {
	CompanyNamesBySlugs(ctx context.Context, slugs []string) ([]db.CompanyNamesBySlugsRow, error)
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

// companyFacetParam is the one facet outside the default set, counted only when
// `facets=` names it. It has thousands of values and the sidebar company filter
// is a server-backed typeahead (GET /api/v1/companies), so the default set does
// not pay for it; a caller that names it gets the exact distinct-company count
// and the top topCompaniesLimit companies by job count (see JobFacets).
const companyFacetParam = "company_slug"

// topCompaniesLimit bounds the named company distribution: the response carries
// the busiest companies, not the up-to-maxValuesPerFacet tail Meili returns.
const topCompaniesLimit = 50

// facetsNoDistribution are string facets we filter on but leave out of the
// DEFAULT distribution set; each is counted only when `facets=` names it.
var facetsNoDistribution = map[string]bool{companyFacetParam: true}

// facetAttributes is the list of index attributes to request facets for: every
// string facet (the same attributes search.StringFacets filters on), minus the
// ones in facetsNoDistribution, plus the extras. Sorted for a deterministic
// request. This is the single source shared with the search filter vocabulary —
// a new facet added to search.StringFacets is counted here automatically.
//
// `only` narrows the set to the named public params, and is the only way to
// reach a facet in facetsNoDistribution. A distribution is counted
// per attribute and the wide-valued ones (cities, skills) dominate — on prod the
// full set costs 284ms against ?work_mode=remote versus 10ms for one attribute —
// so a caller that reads a single key should not pay for the other twenty-six.
func facetAttributes(only ...string) []string {
	want := make(map[string]bool, len(only))
	for _, p := range only {
		want[p] = true
	}
	// No names given ⇒ every param is wanted.
	keep := func(param string) bool { return len(want) == 0 || want[param] }

	attrs := make([]string, 0, len(search.StringFacets)+len(facetExtraParams))
	for param, attr := range search.StringFacets {
		if !keep(param) || (facetsNoDistribution[param] && !want[param]) {
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

// requestedFacets reads the optional `facets=`: a comma-separated list of public
// facet params to count instead of all of them.
//
// An unknown name is an error, not a silent no-op. Callers read counts by key and
// already treat a missing one as "Meili's per-facet cap dropped this value", so a
// typo would hide indefinitely. company_slug is accepted here although it is
// absent from the default set: naming it is how a caller opts in.
func requestedFacets(c *fiber.Ctx) ([]string, error) {
	raw := c.Query("facets")
	if raw == "" {
		return nil, nil
	}
	names := strings.Split(raw, ",")
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		_, isString := search.StringFacets[name]
		_, isExtra := facetExtraParams[name]
		if !isString && !isExtra {
			return nil, fiber.NewError(fiber.StatusBadRequest, "unknown facet: "+name)
		}
		out = append(out, name)
	}
	return out, nil
}

// locationFacetParams are the geography facets that share ONE OR-group in
// FilterFromValues (their included values widen the results together, not
// intersect). Disjunctive counting of any one of them must drop the whole group's
// contribution, not just that param — otherwise selecting a country would zero
// every sibling region (the reverse of what disjunctive mode is for).
var locationFacetParams = []string{"regions", "countries", "cities", "metros"}

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

// facetsParams are this endpoint's own params: the query text, the facet-subset
// selector and the disjunctive toggle.
var facetsParams = []string{"q", "facets", "disjunctive"}

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
		// `facets=` is refused, not ignored: disjunctive derives one query per
		// facet from the SELECTION, so narrowing the output saves none of them —
		// full price for a partial answer.
		if len(only) > 0 {
			return fiber.NewError(fiber.StatusBadRequest, "facets= cannot be combined with disjunctive")
		}
		vals := queryValues(c)
		res, err = h.facets.DisjunctiveFacetCounts(c.Context(), q, facetReqs(vals), search.FilterFromValues(vals))
	} else {
		res, err = h.cachedFacetCounts(c.Context(), search.FacetParams{
			Query:  q,
			Filter: buildSearchFilter(c),
			Facets: facetAttributes(only...),
		})
	}
	if err != nil {
		return err
	}

	view := facetView(res)
	h.nameCompanies(c.Context(), view.TopCompanies)
	return dataResponseWithIgnored(c, view, ignoredParams(c, facetsParams), ignoredJobValues(c))
}

// nameCompanies fills each top company's display name from the companies table
// in one read. A slug with no row yet (the table trails ingest) keeps its slug as
// its name, so every entry carries one.
//
// The names only decorate the counts, so a failed read is logged and every company
// keeps its slug: the caller still gets the facets and the exact total.
func (h *searchHandlers) nameCompanies(ctx context.Context, top []companyFacetValue) {
	if len(top) == 0 {
		return
	}
	names := map[string]string{}
	if h.companyNames != nil {
		slugs := make([]string, len(top))
		for i, t := range top {
			slugs[i] = t.Slug
		}
		rows, err := h.companyNames.CompanyNamesBySlugs(ctx, slugs)
		if err != nil {
			log.Printf("facets: company names unavailable, using slugs: %v", err)
		}
		for _, r := range rows {
			names[r.Slug] = r.Name
		}
	}
	for i := range top {
		if n := names[top[i].Slug]; n != "" {
			top[i].Name = n
		} else {
			top[i].Name = top[i].Slug
		}
	}
}

// facetPayload is the public shape of a facet result: distributions and numeric
// stats keyed by the query-param names callers filter by, never by the internal
// Meilisearch attributes. CompaniesTotal and TopCompanies appear only when the
// company_slug facet was requested; the company distribution travels as the
// ordered, named TopCompanies rather than inside Facets.
type facetPayload struct {
	Total          int64                       `json:"total"`
	Facets         map[string]map[string]int64 `json:"facets"`
	Stats          map[string]search.FacetStat `json:"stats"`
	CompaniesTotal *int64                      `json:"companies_total,omitempty"`
	TopCompanies   []companyFacetValue         `json:"top_companies,omitempty"`
}

// companyFacetValue is one company of the trimmed company distribution.
type companyFacetValue struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// topCompanies orders a company distribution by job count (slug breaks ties, so
// the order is stable) and keeps the first topCompaniesLimit. Names are filled by
// the caller, which has the database.
func topCompanies(dist map[string]int64) []companyFacetValue {
	out := make([]companyFacetValue, 0, len(dist))
	for slug, n := range dist {
		out = append(out, companyFacetValue{Slug: slug, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Slug < out[j].Slug
	})
	if len(out) > topCompaniesLimit {
		out = out[:topCompaniesLimit]
	}
	return out
}

// facetView re-keys a raw facet result to the public param names. It is shared by
// the HTTP endpoint and the assistant's facets tool, so the vocabulary the agent
// filters by is exactly the vocabulary the API publishes.
func facetView(res search.FacetResult) facetPayload {
	param := facetParamByAttr()

	// Distributions, dropping the noisy per-value distribution of the continuous
	// numeric facets (kept only as stats below).
	facets := make(map[string]map[string]int64, len(res.Facets))
	var top []companyFacetValue
	for attr, dist := range res.Facets {
		p, ok := param[attr]
		if !ok || facetExtraParams[p].statOnly {
			continue
		}
		if p == companyFacetParam {
			top = topCompanies(dist)
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
	return facetPayload{Total: res.Total, Facets: facets, Stats: stats, CompaniesTotal: res.Companies, TopCompanies: top}
}
