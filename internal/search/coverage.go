package search

import (
	"context"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

// coverageBatchSize bounds how many company slugs one NonAggregatorCompanies query asks
// about at once, keeping each request's filter payload small. Not tuned beyond "clearly
// small enough" — see the aggregator-ats-coverage-skip design doc.
const coverageBatchSize = 500

// NonAggregatorCompanies implements internal/pipeline's CoverageLookup: for a batch of
// EXACT (unfolded) company_slug values, returns the subset that already have an open
// posting from a source NOT in aggregators. See CoverageLookup's doc comment for why this
// is exact-match only, never folded — a live Meili filter cannot compute the reindex
// suppression pass's hyphen-stripping fold at query time.
func (c *Client) NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error) {
	return nonAggregatorCompanies(ctx, companySlugs, aggregators, func(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		return c.facet.SearchWithContext(ctx, query, &meilisearch.SearchRequest{Filter: filter, Facets: facets, Limit: 0})
	})
}

// nonAggregatorCompanies is the batching/query-shape logic behind NonAggregatorCompanies,
// split out (and taking an injected searcher) so it is unit-testable without a live engine —
// the same seam disjunctiveFacetCounts uses.
func nonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string, search facetSearcher) (map[string]bool, error) {
	covered := make(map[string]bool)
	for _, batch := range chunkStrings(companySlugs, coverageBatchSize) {
		groups := [][]string{{InStrings("company_slug", batch)}}
		if notIn := NotInStrings("source", aggregators); notIn != "" {
			groups = append(groups, []string{notIn})
		}
		resp, err := search(ctx, "", Filter(groups...), []string{"company_slug"})
		if err != nil {
			return nil, fmt.Errorf("search: aggregator coverage lookup: %w", err)
		}
		fr, err := buildFacetResult(resp)
		if err != nil {
			return nil, err
		}
		for slug := range fr.Facets["company_slug"] {
			covered[slug] = true
		}
	}
	return covered, nil
}

// chunkStrings splits values into chunks of at most size elements each (the last chunk may
// be smaller). size <= 0 is treated as "one chunk" — chunkStrings is only ever called with
// the package's own positive constants, so this is a defensive fallback, not a documented
// caller-facing behavior.
func chunkStrings(values []string, size int) [][]string {
	if len(values) == 0 {
		return nil
	}
	if size <= 0 {
		return [][]string{values}
	}
	var chunks [][]string
	for i := 0; i < len(values); i += size {
		end := i + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[i:end])
	}
	return chunks
}
