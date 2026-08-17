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
// spelling actually matched (see coverageSpellings).
func (c *Client) NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error) {
	return nonAggregatorCompanies(ctx, companySlugs, aggregators, func(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		return c.facet.SearchWithContext(ctx, query, &meilisearch.SearchRequest{Filter: filter, Facets: facets, Limit: 0})
	})
}

// nonAggregatorCompanies is the batching/query-shape logic behind NonAggregatorCompanies,
// split out (and taking an injected searcher) so it is unit-testable without a live engine —
// the same seam disjunctiveFacetCounts uses.
func nonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string, search facetSearcher) (map[string]bool, error) {
	queries, owners := coverageQuery(companySlugs)
	covered := make(map[string]bool)
	for _, batch := range chunkStrings(queries, coverageBatchSize) {
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
		// A spelling may answer for more than one requested slug ("q-tech" and "qtech" both
		// ask about "qtech"), so the hit is credited to every slug that asked for it.
		for spelling := range fr.Facets["company_slug"] {
			for _, slug := range owners[spelling] {
				covered[slug] = true
			}
		}
	}
	return covered, nil
}

// coverageQuery expands the requested slugs into the spellings to filter on, deduped and in
// first-asked order, plus the reverse index that credits a hit back to every slug that asked
// for that spelling. The caller's own slugs stay the keys of the answer, so a spelling is an
// implementation detail the pipeline never sees.
func coverageQuery(companySlugs []string) (queries []string, owners map[string][]string) {
	owners = make(map[string][]string, len(companySlugs))
	for _, slug := range companySlugs {
		for _, spelling := range coverageSpellings(slug) {
			if _, asked := owners[spelling]; !asked {
				queries = append(queries, spelling)
			}
			owners[spelling] = append(owners[spelling], slug)
		}
	}
	return queries, owners
}

// coverageSpellings returns the company_slug spellings one requested slug should be looked up
// under: the slug itself, plus its hyphen-stripped form when that differs.
//
// A Meili filter matches a stored value literally — it cannot compute the hyphen-stripping
// fold the reindex suppression pass compares on. Asking about the folded SPELLING instead of
// folding the comparison gets most of the way there for free: the query stays an exact match
// against a stored value, no new filterable attribute is needed (adding one 500s search until
// the rebuild lands), and nothing about the index changes.
//
// Measured on prod: of the postings the exact-only gate let through and the weekly dedup pass
// later marked, 77% were an aggregator spelling the employer with hyphens where its own ATS
// spelled it without ("reid-health" vs "reidhealth"). The remaining directions are not
// reachable this way — where the ATS is the one using hyphens, there is no way to guess where
// they go — and stay the dedup pass's job.
//
// The risk this accepts is that stripping hyphens merges two genuinely different employers.
// It was measured rather than assumed: across the whole open non-aggregator catalogue, 1,173
// of 263,669 folded groups hold more than one spelling (0.44%), and a sample of them was
// entirely one company written several ways — "JP Morgan Chase"/"JPMorgan Chase", "Q-Tech"/
// "Qtech", "Spin Master"/"spinmaster". That is the same bet the reindex pass already makes;
// this only makes it a week earlier.
func coverageSpellings(slug string) []string {
	folded := strings.ReplaceAll(slug, "-", "")
	if folded == "" || folded == slug {
		return []string{slug}
	}
	return []string{slug, folded}
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
