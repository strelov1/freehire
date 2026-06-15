package search

import (
	"context"
	"encoding/json"
	"fmt"

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
// plus Meilisearch's estimated total for the filtered set.
type FacetResult struct {
	Total  int64
	Facets map[string]map[string]int64 // attr → value → count
	Stats  map[string]FacetStat        // attr → {min,max}
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
	resp, err := c.index.SearchWithContext(ctx, p.Query, &meilisearch.SearchRequest{
		Filter: p.Filter,
		Facets: p.Facets,
		Limit:  0,
	})
	if err != nil {
		return FacetResult{}, fmt.Errorf("search: facet query: %w", err)
	}
	return buildFacetResult(resp)
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
