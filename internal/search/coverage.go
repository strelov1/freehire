package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/meilisearch/meilisearch-go"
)

// coverageBatchSize bounds how many company slugs one NonAggregatorCompanies query asks
// about at once, keeping each request's filter payload small. Not tuned beyond "clearly
// small enough" — see the aggregator-ats-coverage-skip design doc.
const coverageBatchSize = 500

// NonAggregatorCompanies implements internal/pipeline's CoverageLookup: for a batch of
// company_slug values, returns the subset that already have an open posting from a source
// NOT in aggregators. The answer is keyed by the slugs the caller asked about, whichever
// field actually matched.
func (c *Client) NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error) {
	return nonAggregatorCompanies(ctx, companySlugs, aggregators, func(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		return c.facet.SearchWithContext(ctx, query, &meilisearch.SearchRequest{Filter: filter, Facets: facets, Limit: 0})
	})
}

// nonAggregatorCompanies is the batching/query-shape logic behind NonAggregatorCompanies,
// split out (and taking an injected searcher) so it is unit-testable without a live engine —
// the same seam disjunctiveFacetCounts uses.
func nonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string, search facetSearcher) (map[string]bool, error) {
	// The source clause is the same for every batch, so it is built once.
	sourceClause := NotInStrings("source", aggregators)
	covered := make(map[string]bool)
	for _, batch := range chunkStrings(companySlugs, coverageBatchSize) {
		folded, owners := foldedBatch(batch)
		// Both clauses sit in ONE group, so they are OR'd: an employer counts as covered
		// when either side's spelling matches. Keeping the exact clause is not redundant —
		// company_slug_folded reaches a document only once a rebuild has written it, and a
		// row that predates the backfill has none at all, so the exact match is what carries
		// the gate meanwhile.
		match := []string{InStrings("company_slug", batch)}
		if len(folded) > 0 {
			match = append(match, InStrings("company_slug_folded", folded))
		}
		groups := [][]string{match}
		if sourceClause != "" {
			groups = append(groups, []string{sourceClause})
		}
		resp, err := search(ctx, "", Filter(groups...), []string{"company_slug", "company_slug_folded"})
		if err != nil {
			return nil, fmt.Errorf("search: aggregator coverage lookup: %w", err)
		}
		fr, err := buildFacetResult(resp)
		if err != nil {
			return nil, err
		}
		// Walking the BATCH rather than the facet is what keeps the answer to the slugs the
		// caller asked about: Meili reports the whole facet distribution of the matched set,
		// and the folded clause pulls in documents whose exact slug was never in the batch.
		// Crediting those would put keys in the answer the caller never asked for, which its
		// contract forbids — and each would be a coverage claim about a company nobody
		// enquired about.
		exact := fr.Facets["company_slug"]
		for _, slug := range batch {
			if _, hit := exact[slug]; hit {
				covered[slug] = true
			}
		}
		// A folded hit answers for every requested slug that folds to it ("q-tech" and
		// "qtech" both fold to "qtech").
		for value := range fr.Facets["company_slug_folded"] {
			for _, slug := range owners[value] {
				covered[slug] = true
			}
		}
	}
	return covered, nil
}

// foldedBatch returns the distinct folded spellings to filter on for one batch of requested
// slugs, plus the reverse index that credits a folded hit back to every slug that folds to it.
// A slug folding to "" (nothing but hyphens) is left out: as a filter value it would match
// every document that has no folded slug at all.
func foldedBatch(companySlugs []string) (folded []string, owners map[string][]string) {
	owners = make(map[string][]string, len(companySlugs))
	for _, slug := range companySlugs {
		f := foldSlug(slug)
		if f == "" {
			continue
		}
		if _, seen := owners[f]; !seen {
			folded = append(folded, f)
		}
		owners[f] = append(owners[f], slug)
	}
	return folded, owners
}

// foldSlug is the fold jobs.company_slug_folded stores (migration 0109) and the
// aggregator-suppression pass compares on: the company slug with its hyphens removed. The two
// must agree exactly, so this is the only place the query side computes it.
func foldSlug(slug string) string { return strings.ReplaceAll(slug, "-", "") }

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
