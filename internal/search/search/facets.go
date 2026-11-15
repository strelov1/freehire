package search

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// FacetParams is a facet-distribution request: matching-job counts per facet value (and
// numeric min/max) under a filter. Filter is the value built by Filter (nil for none).
type FacetParams struct {
	Query  string
	Filter any
	Facets []string
}

// FacetResult holds the facet distribution and stats for a FacetCounts request plus
// Meilisearch's estimated total. Companies is the exact distinct company_slug count under
// the same filter, set only when asked for (see DistinctCount) so a cached result carries it.
type FacetResult struct {
	Total     int64
	Facets    map[string]map[string]int64 // attr → value → count
	Stats     map[string]FacetStat        // attr → {min,max}
	Companies *int64
}

// FacetStat is the numeric range of a facet over the matched set (Meilisearch facetStats).
type FacetStat struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// FacetCounts returns the facet distribution under the given filter. It runs with limit 0,
// no sort and no hybrid embedder.
func (c *Client) FacetCounts(ctx context.Context, p FacetParams) (FacetResult, error) {
	raw, err := c.facetSearch(ctx, p.Query, p.Filter, p.Facets)
	if err != nil {
		return FacetResult{}, queryErr(ctx, "search: facet query", err)
	}
	res := buildFacetResult(raw)
	if err := widenImpliedRegionCounts(ctx, p.Query, p.Filter, res.Facets["regions"], c.facetSearch); err != nil {
		return FacetResult{}, err
	}
	return res, nil
}

// andFragment ANDs a fragment onto a filter as FilterFromValues builds it (nil or
// [][]string). Any other shape is refused: dropping it would count the whole catalogue.
func andFragment(filter any, frag string) (any, error) {
	switch f := filter.(type) {
	case nil:
		return [][]string{{frag}}, nil
	case [][]string:
		return append(slices.Clone(f), []string{frag}), nil
	default:
		return nil, fmt.Errorf("search: cannot narrow a %T filter", filter)
	}
}

// widenImpliedRegionCounts replaces each implying region's bucket with what choosing it
// returns (see regionImplies): documents carrying ANY of its regions, counted once. Summing
// buckets would double-count an EMEA posting carrying both "eu" and "uk", so it asks the
// engine. A nil distribution costs nothing.
func widenImpliedRegionCounts(ctx context.Context, query string, filter any, dist map[string]int64, search facetSearcher) error {
	if dist == nil {
		return nil
	}
	for val := range regionImplies {
		set := impliedRegions(val)
		if !slices.ContainsFunc(set, func(r string) bool { return dist[r] > 0 }) {
			continue
		}
		widened, err := andFragment(filter, InStrings("regions", set))
		if err != nil {
			return err
		}
		resp, err := search(ctx, query, widened, nil)
		if err != nil {
			return queryErr(ctx, "search: region count "+val, err)
		}
		dist[val] = resp.Total
	}
	return nil
}

// rawFacets is an engine-neutral facet answer, filled by both Meilisearch and the
// FacetEngine.
type rawFacets struct {
	Total        int64
	Distribution map[string]map[string]int64
	Stats        map[string]FacetStat
}

// rawFromResponse decodes a Meilisearch response into a rawFacets.
func rawFromResponse(resp *meilisearch.SearchResponse) (rawFacets, error) {
	dist, err := decodeFacetDistribution(resp.FacetDistribution)
	if err != nil {
		return rawFacets{}, err
	}
	stats, err := decodeFacetStats(resp.FacetStats)
	if err != nil {
		return rawFacets{}, err
	}
	return rawFacets{Total: resp.EstimatedTotalHits, Distribution: dist, Stats: stats}, nil
}

// facetSearch runs one facet-count query. Without query text the router may send it to
// the second engine (see route.go); otherwise, and as fallback, it goes to Meilisearch.
func (c *Client) facetSearch(ctx context.Context, query string, filter any, facets []string) (rawFacets, error) {
	meili := func() (rawFacets, error) { return c.meiliFacets(ctx, query, filter, facets) }
	mode := c.routeMode(query)
	if mode == FacetModeMeili {
		c.countMeiliOnly(query, "facets")
		return meili()
	}
	return routeRequest(ctx, c.router, mode, "facets", filter,
		func(ctx context.Context) (rawFacets, error) {
			a, err := c.router.engine.Facets(ctx, filter, facets)
			return rawFacets(a), err
		},
		meili, cloneRaw, diffRaw)
}

// meiliFacets is the Meilisearch half of facetSearch. One alternative is a single-index
// request; several are one federated request whose merged counts are per document.
func (c *Client) meiliFacets(ctx context.Context, query string, filter any, facets []string) (rawFacets, error) {
	filter = withOpenJobs(filter)
	alts, err := ParseQuery(query)
	if err != nil {
		return rawFacets{}, err
	}
	for i := range alts {
		alts[i].Text = wholeWords(alts[i].Text)
	}
	if len(alts) == 1 {
		resp, err := c.facet.SearchWithContext(ctx, alts[0].Text, &meilisearch.SearchRequest{
			Filter:               filter,
			Facets:               facets,
			Limit:                0,
			AttributesToSearchOn: alts[0].Fields,
		})
		if err != nil {
			return rawFacets{}, err
		}
		return rawFromResponse(resp)
	}
	merged, err := c.manager.MultiSearchWithContext(ctx, buildFederatedFacets(alts, filter, facets))
	if err != nil {
		return rawFacets{}, err
	}
	resp, err := federatedAsSearchResponse(merged)
	if err != nil {
		return rawFacets{}, err
	}
	return rawFromResponse(resp)
}

// DistinctCount returns the exact number of distinct values of attr among documents
// matching query and filter, which the facet distribution cannot give past the
// maxValuesPerFacet cap.
//
// It pages (page 1, hitsPerPage 0) and reads totalHits because with `limit: 0`
// estimatedTotalHits ignores `distinct`. It requests no facets because `distinct` beside
// `facets` collapses every facet count to 1.
//
// An OR query is refused as ErrBadQuery: federation would count a company matched by two
// alternatives twice.
func (c *Client) DistinctCount(ctx context.Context, query string, filter any, attr string) (int64, error) {
	meili := func() (int64, error) { return c.meiliDistinctCount(ctx, query, filter, attr) }
	mode := c.routeMode(query)
	if mode == FacetModeMeili {
		c.countMeiliOnly(query, "distinct")
		return meili()
	}
	return routeRequest(ctx, c.router, mode, "distinct", filter,
		func(ctx context.Context) (int64, error) { return c.router.engine.DistinctCount(ctx, filter, attr) },
		meili, func(n int64) int64 { return n }, diffCount)
}

func (c *Client) meiliDistinctCount(ctx context.Context, query string, filter any, attr string) (int64, error) {
	filter = withOpenJobs(filter)
	alts, err := ParseQuery(query)
	if err != nil {
		return 0, err
	}
	alts[0].Text = wholeWords(alts[0].Text)
	if len(alts) != 1 {
		return 0, fmt.Errorf("search: distinct count: %w: not available for an OR query", ErrBadQuery)
	}
	none := int64(0)
	resp, err := c.facet.SearchWithContext(ctx, alts[0].Text, &meilisearch.SearchRequest{
		Filter:               filter,
		Distinct:             attr,
		Page:                 1,
		HitsPerPage:          &none,
		AttributesToSearchOn: alts[0].Fields,
	})
	if err != nil {
		return 0, queryErr(ctx, "search: distinct count", err)
	}
	return resp.TotalHits, nil
}

// FacetReq is one facet's disjunctive request: the attribute and the filter to count it
// under, the full filter minus that facet's own selection so siblings keep their counts.
type FacetReq struct {
	Attr   string
	Filter any
}

// facetSearcher runs one Limit:0 facet query; injected so tests need no live engine.
type facetSearcher func(ctx context.Context, query string, filter any, facets []string) (rawFacets, error)

// DisjunctiveFacetCounts computes each facet's distribution under its own reduced filter
// plus the grand total under the full filter, all queries concurrently.
func (c *Client) DisjunctiveFacetCounts(ctx context.Context, query string, reqs []FacetReq, totalFilter any) (FacetResult, error) {
	return disjunctiveFacetCounts(ctx, query, reqs, totalFilter, c.facetSearch)
}

func disjunctiveFacetCounts(ctx context.Context, query string, reqs []FacetReq, totalFilter any, search facetSearcher) (FacetResult, error) {
	res := FacetResult{Facets: map[string]map[string]int64{}, Stats: map[string]FacetStat{}}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}

	// The grand total under the full filter.
	run(func() error {
		resp, err := search(ctx, query, totalFilter, nil)
		if err != nil {
			return queryErr(ctx, "search: disjunctive total", err)
		}
		mu.Lock()
		res.Total = resp.Total
		mu.Unlock()
		return nil
	})
	for _, r := range reqs {
		r := r
		run(func() error {
			resp, err := search(ctx, query, r.Filter, []string{r.Attr})
			if err != nil {
				return queryErr(ctx, "search: disjunctive facet "+r.Attr, err)
			}
			fr := buildFacetResult(resp)
			if r.Attr == "regions" {
				if err := widenImpliedRegionCounts(ctx, query, r.Filter, fr.Facets[r.Attr], search); err != nil {
					return err
				}
			}
			mu.Lock()
			if d, ok := fr.Facets[r.Attr]; ok {
				res.Facets[r.Attr] = d
			}
			for k, v := range fr.Stats {
				res.Stats[k] = v
			}
			mu.Unlock()
			return nil
		})
	}

	wg.Wait()
	if firstErr != nil {
		return FacetResult{}, firstErr
	}
	return res, nil
}

// buildFacetResult assembles a FacetResult from a raw answer, folding source families
// into their own bucket.
func buildFacetResult(raw rawFacets) FacetResult {
	foldSourceFamilies(raw.Distribution["source"])
	return FacetResult{Total: raw.Total, Facets: raw.Distribution, Stats: raw.Stats}
}

// foldSourceFamilies adds each family member's count into the family's bucket
// (vocab.SourceFamilies). A posting has one source, so summing counts nothing twice.
func foldSourceFamilies(dist map[string]int64) {
	for val, n := range dist {
		if family := vocab.SourceFamilyOf(val); family != val {
			dist[family] += n
			delete(dist, val)
		}
	}
}

// decodeFacetDistribution parses facetDistribution JSON into attribute → value → count.
// An empty payload yields a nil map, not an error.
func decodeFacetDistribution(raw json.RawMessage) (map[string]map[string]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out map[string]map[string]int64
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("search: decode facet distribution: %w", err)
	}
	return out, nil
}

// decodeFacetStats parses facetStats JSON into attribute → {min,max}. An empty payload
// yields a nil map, not an error.
func decodeFacetStats(raw json.RawMessage) (map[string]FacetStat, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out map[string]FacetStat
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("search: decode facet stats: %w", err)
	}
	return out, nil
}
