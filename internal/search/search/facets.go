package search

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/meilisearch/meilisearch-go"
)

// FacetParams is a facet-distribution request: it reports the count of matching
// jobs per facet value (and numeric min/max) under a filter, with none of the
// ranking/pagination concerns of a SearchParams. Filter is the value built by
// Filter (nil for none); Facets lists the attributes to compute distributions for.
type FacetParams struct {
	Query  string
	Filter any
	Facets []string
}

// FacetResult holds the facet distribution and stats for a FacetCounts request,
// plus Meilisearch's estimated total for the filtered set. Companies is the exact
// number of distinct company_slug values under the same filter — set only when a
// caller asked for it (see DistinctCount), nil otherwise. It lives here so a
// cached facet result carries it.
type FacetResult struct {
	Total     int64
	Facets    map[string]map[string]int64 // attr → value → count
	Stats     map[string]FacetStat        // attr → {min,max}
	Companies *int64
}

// FacetStat is the numeric range of a facet over the matched set, as reported by
// Meilisearch's facetStats (e.g. salary min/max).
type FacetStat struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// FacetCounts returns the facet distribution for the matched set under the given
// filter. It is deliberately separate from Search: counting facets is a distinct
// responsibility from returning ranked hits, so it runs with limit 0 (no
// documents), no sort, and no hybrid embedder — just the distribution.
func (c *Client) FacetCounts(ctx context.Context, p FacetParams) (FacetResult, error) {
	resp, err := c.facetSearch(ctx, p.Query, p.Filter, p.Facets)
	if err != nil {
		return FacetResult{}, queryErr(ctx, "search: facet query", err)
	}
	return buildFacetResult(resp)
}

// facetSearch runs one facet-count query, reading q's operators the same way
// Search does: a single alternative is today's single-index request, several are
// one federated request whose merged counts are per document.
func (c *Client) facetSearch(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
	alts, err := ParseQuery(query)
	if err != nil {
		return nil, err
	}
	if len(alts) == 1 {
		return c.facet.SearchWithContext(ctx, alts[0].Text, &meilisearch.SearchRequest{
			Filter:               filter,
			Facets:               facets,
			Limit:                0,
			AttributesToSearchOn: alts[0].Fields,
		})
	}
	resp, err := c.manager.MultiSearchWithContext(ctx, buildFederatedFacets(alts, filter, facets))
	if err != nil {
		return nil, err
	}
	return federatedAsSearchResponse(resp)
}

// DistinctCount returns the exact number of distinct values of attr among the
// documents matching query and filter — e.g. how many companies a search spans,
// which the facet distribution cannot answer once it passes the
// maxValuesPerFacet cap.
//
// Two engine behaviours decide the request's shape. It pages (page 1,
// hitsPerPage 0) and reads totalHits, because with `limit: 0` Meilisearch's
// estimatedTotalHits ignores `distinct` and reports the document count. And it
// asks for no facets, because `distinct` beside `facets` collapses every facet
// count to 1 — the distribution is a separate request.
//
// An OR query is refused as ErrBadQuery: it runs as a federated search, where a
// per-alternative distinct counts a company matched by two alternatives twice,
// and a count that is not exact is what this exists to avoid.
func (c *Client) DistinctCount(ctx context.Context, query string, filter any, attr string) (int64, error) {
	alts, err := ParseQuery(query)
	if err != nil {
		return 0, err
	}
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

// FacetReq is one facet's disjunctive request: the attribute to distribute and
// the filter to count it under — the full filter with that facet's own selection
// removed, so its own selection does not zero out its sibling values.
type FacetReq struct {
	Attr   string
	Filter any
}

// facetSearcher runs one Limit:0 facet query. It is injected so
// disjunctiveFacetCounts is unit-testable without a live engine.
type facetSearcher func(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error)

// DisjunctiveFacetCounts computes each facet's distribution under its own reduced
// filter (so a selected facet still shows its siblings' counts) plus the grand
// total under the full filter. All queries run concurrently, so the latency is
// that of the slowest single facet query, not their sum.
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

	// The grand total under the full filter — the number "Show N results" reflects.
	run(func() error {
		resp, err := search(ctx, query, totalFilter, nil)
		if err != nil {
			return queryErr(ctx, "search: disjunctive total", err)
		}
		mu.Lock()
		res.Total = resp.EstimatedTotalHits
		mu.Unlock()
		return nil
	})
	// Each facet under its own reduced filter, keeping only its own distribution.
	for _, r := range reqs {
		r := r
		run(func() error {
			resp, err := search(ctx, query, r.Filter, []string{r.Attr})
			if err != nil {
				return queryErr(ctx, "search: disjunctive facet "+r.Attr, err)
			}
			fr, err := buildFacetResult(resp)
			if err != nil {
				return err
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

// buildFacetResult assembles a FacetResult from a Meilisearch response, decoding
// its raw facet payloads. Split from FacetCounts so the assembly is unit-testable
// without a live engine.
func buildFacetResult(resp *meilisearch.SearchResponse) (FacetResult, error) {
	facets, err := decodeFacetDistribution(resp.FacetDistribution)
	if err != nil {
		return FacetResult{}, err
	}
	stats, err := decodeFacetStats(resp.FacetStats)
	if err != nil {
		return FacetResult{}, err
	}
	return FacetResult{Total: resp.EstimatedTotalHits, Facets: facets, Stats: stats}, nil
}

// decodeFacetDistribution parses Meilisearch's raw facetDistribution JSON into
// attribute → value → count. A nil/empty payload yields a nil map (no facets
// requested or none matched), never an error.
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

// decodeFacetStats parses Meilisearch's raw facetStats JSON into attribute →
// {min,max}. A nil/empty payload yields a nil map, never an error.
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
