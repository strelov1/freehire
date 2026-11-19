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
	"golang.org/x/sync/singleflight"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// facetCacheTTL bounds how stale a cached facet distribution may be. Counts are
// identical for every caller and change only when ingest lands a wave, so the win
// is absorbing bursts of identical requests, not holding a count for long. Server
// renders keep their own ten-minute copy (web/src/lib/server/facetCache.ts).
const facetCacheTTL = 5 * time.Minute

// cachedFacetCounts wraps facetCounter.FacetCounts with a best-effort read-through
// cache: a nil cache, a miss or any cache error falls through to a live count.
// The key hashes the exact FacetParams. The disjunctive path is not routed here:
// it is the interactive live-modal path and its per-facet fan-out keys poorly.
func (h *searchHandlers) cachedFacetCounts(ctx context.Context, p search.FacetParams) (search.FacetResult, error) {
	return cachedFacetCounts(ctx, h.facets, h.cache, p)
}

// cachedFacetCounts is the cache-through facet read. It is a function so the ChatGPT
// app's job_market_stats shares the website's cache slots. A nil cache reads through.
func cachedFacetCounts(ctx context.Context, f facetCounter, c cache.Cache, p search.FacetParams) (search.FacetResult, error) {
	key := facetCacheKey(p)
	if res, ok := cachedFacets(ctx, c, key); ok {
		return res, nil
	}

	ch := facetFlight.DoChan(key, func() (any, error) {
		// A flight that finished between our miss and this call has already stored it.
		if res, ok := cachedFacets(ctx, c, key); ok {
			return res, nil
		}
		// The count is shared, so one caller going away must not cancel it for the others;
		// the engine client sets no timeout of its own, so the count gets one here.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), facetCountTimeout)
		defer cancel()
		res, err := countFacets(ctx, f, p)
		if err != nil {
			return nil, err
		}
		if c != nil {
			// Store is best-effort: a failed write just means the next request recomputes.
			_ = cache.SetJSON(ctx, c, key, res, facetCacheTTL)
		}
		return res, nil
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return search.FacetResult{}, r.Err
		}
		return r.Val.(search.FacetResult), nil
	case <-ctx.Done():
		return search.FacetResult{}, ctx.Err()
	}
}

// cachedFacets reads one facet result from the cache. A nil cache, a miss and a cache
// error all report not found.
func cachedFacets(ctx context.Context, c cache.Cache, key string) (search.FacetResult, bool) {
	if c == nil {
		return search.FacetResult{}, false
	}
	res, found, err := cache.GetJSON[search.FacetResult](ctx, c, key)
	return res, err == nil && found
}

// facetFlight collapses concurrent misses on one cache key into a single count, so
// an expired key does not send every request to Meilisearch at once. Callers only
// read the shared result.
var facetFlight singleflight.Group

// facetCountTimeout bounds a shared count, which no single caller's deadline does.
const facetCountTimeout = 30 * time.Second

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

// facetCacheKey derives a bounded, deterministic cache key by hashing the JSON of
// the params. The marshal error branch is a guard, not a real path: it degrades to
// one shared slot instead of panicking.
//
// The filter is canonicalized first: search.FilterFromValues ranges over a map, so
// group and value order is random per request and the same request would miss its
// own cache entry. AND across groups and OR within one are order-independent, so
// sorting is safe and never touches the filter sent to Meili.
func facetCacheKey(p search.FacetParams) string {
	p.Filter = canonicalFilter(p.Filter)
	raw, err := json.Marshal(p)
	if err != nil {
		return "jobfacets:v1:unkeyable"
	}
	sum := sha256.Sum256(raw)
	return "jobfacets:v1:" + hex.EncodeToString(sum[:])
}

// canonicalFilter sorts a [][]string filter within and across groups so order alone
// cannot change the cache key. Any other shape is returned unchanged.
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
	slices.SortFunc(out, slices.Compare)
	return out
}

// facetCounter is the analytics backend the facets handler depends on, separate
// from searcher so the handler needs only the method it uses. *search.Client
// satisfies both; a nil counter means search is unconfigured and the endpoint 503s.
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

// facetExtra describes a facetable attribute that is not a string-equality facet in
// search.StringFacets. statOnly marks a continuous numeric facet exposed only as
// min/max stats: its per-value distribution (a bucket per salary) is dropped.
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
// `facets=` names it: it has thousands of values and the sidebar filter is a
// server-backed typeahead. A caller that names it gets the exact distinct-company
// count and the top topCompaniesLimit companies (see JobFacets).
const companyFacetParam = "company_slug"

// topCompaniesLimit bounds the named company distribution: the response carries
// the busiest companies, not the up-to-maxValuesPerFacet tail Meili returns.
const topCompaniesLimit = 50

// facetsNoDistribution are string facets we filter on but leave out of the
// DEFAULT distribution set; each is counted only when `facets=` names it.
var facetsNoDistribution = map[string]bool{companyFacetParam: true}

// facetAttributes lists the index attributes to request facets for: every string
// facet (the same attributes search.StringFacets filters on) minus
// facetsNoDistribution, plus the extras. Sorted for a deterministic request.
//
// `only` narrows the set to the named public params and is the only way to reach a
// facet in facetsNoDistribution. Wide-valued attributes (cities, skills) dominate
// the cost, so a caller that reads one key should not pay for the rest.
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
// An unknown name is an error, not a no-op: callers read counts by key and treat a
// missing one as "dropped by Meili's per-facet cap", so a typo would hide. company_slug
// is accepted although absent from the default set: naming it is the opt-in.
func requestedFacets(vals url.Values) ([]string, error) {
	raw := vals.Get("facets")
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
// FilterFromValues. Disjunctive counting of one must drop the whole group, or
// selecting a country would zero every sibling region.
var locationFacetParams = []string{"regions", "countries", "cities", "metros"}

// facetReqs builds one disjunctive request per distribution attribute, counted
// under the filter with its own facet's params removed (the whole location
// OR-group for a location facet), so a selection doesn't hide its alternatives.
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
func (h *searchHandlers) JobFacets(ctx context.Context, _ apigen.JobFacetsParams) (*apigen.JobFacetsOK, error) {
	if h.facets == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	vals := contractQuery(ctx)
	q := vals.Get("q")
	only, err := requestedFacets(vals)
	if err != nil {
		return nil, err
	}

	var res search.FacetResult
	if wantsParam(vals, "disjunctive") {
		// Each facet is counted under the full filter minus its own selection. `facets=`
		// is refused, not ignored: disjunctive derives one query per facet from the
		// selection, so narrowing the output saves nothing.
		if len(only) > 0 {
			return nil, fiber.NewError(fiber.StatusBadRequest, "facets= cannot be combined with disjunctive")
		}
		res, err = h.facets.DisjunctiveFacetCounts(ctx, q, facetReqs(vals), search.FilterFromValues(vals))
	} else {
		res, err = h.cachedFacetCounts(ctx, search.FacetParams{
			Query:  q,
			Filter: buildSearchFilter(vals),
			Facets: facetAttributes(only...),
		})
	}
	if err != nil {
		return nil, err
	}

	view := facetView(res)
	h.nameCompanies(ctx, view.TopCompanies)
	data, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	out := &apigen.JobFacetsOK{Data: data}
	ignored, values := ignoredParams(vals, facetsParams), ignoredJobValues(vals)
	if len(ignored) > 0 || len(values) > 0 {
		out.Meta = apigen.NewOptIgnoredMeta(apigen.IgnoredMeta{
			IgnoredParams: contractIgnoredParams(ignored), IgnoredValues: contractIgnoredValues(values),
		})
	}
	return out, nil
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

	// The continuous numeric facets keep only their stats.
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
