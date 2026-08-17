package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// TestNonAggregatorCompanies_QueryShape proves the query internal/pipeline's CoverageLookup
// contract needs: company_slug IN [batch] AND NOT source IN [aggregators], asking only for
// the company_slug facet distribution (Limit 0, no ranked hits).
func TestNonAggregatorCompanies_QueryShape(t *testing.T) {
	var gotFilter any
	var gotFacets []string
	searcher := func(_ context.Context, _ string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		gotFilter = filter
		gotFacets = facets
		dist, _ := json.Marshal(map[string]map[string]int64{"company_slug": {"acme": 3}})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}

	covered, err := nonAggregatorCompanies(context.Background(), []string{"acme", "globex"}, []string{"himalayas", "echojobs"}, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if !covered["acme"] || len(covered) != 1 {
		t.Fatalf("covered = %v, want only acme", covered)
	}
	if !slices.Equal(gotFacets, []string{"company_slug", "company_slug_folded"}) {
		t.Errorf("facets = %v, want both the exact and folded fields", gotFacets)
	}
	filterStr := fmt.Sprintf("%v", gotFilter)
	if !strings.Contains(filterStr, `company_slug IN ["acme", "globex"]`) {
		t.Errorf("filter %v missing company_slug IN clause", gotFilter)
	}
	if !strings.Contains(filterStr, `source NOT IN ["himalayas", "echojobs"]`) {
		t.Errorf("filter %v missing source NOT IN clause", gotFilter)
	}
}

// The query asks BOTH ways in one OR'd group: the exact slug against company_slug and its
// fold against company_slug_folded. The exact clause is not redundant — company_slug_folded
// reaches a document only once a rebuild has written it.
func TestNonAggregatorCompanies_FiltersExactAndFoldedTogether(t *testing.T) {
	var gotFilter any
	var gotFacets []string
	searcher := func(_ context.Context, _ string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		gotFilter, gotFacets = filter, facets
		return &meilisearch.SearchResponse{}, nil
	}
	if _, err := nonAggregatorCompanies(context.Background(), []string{"reid-health"}, nil, searcher); err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	groups, ok := gotFilter.([][]string)
	if !ok || len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("filter = %#v, want one group of two OR'd clauses", gotFilter)
	}
	if !strings.Contains(groups[0][0], `company_slug IN ["reid-health"]`) {
		t.Errorf("first clause = %q, want the exact slug", groups[0][0])
	}
	if !strings.Contains(groups[0][1], `company_slug_folded IN ["reidhealth"]`) {
		t.Errorf("second clause = %q, want the folded slug", groups[0][1])
	}
	if !slices.Equal(gotFacets, []string{"company_slug", "company_slug_folded"}) {
		t.Errorf("facets = %v, want both fields back", gotFacets)
	}
}

// The direction the old spelling-only lookup could not reach: the AGGREGATOR writes the
// unhyphenated form and the employer's own ATS is the hyphenated side. Nothing can guess where
// to put hyphens, so this only works because the fold is stored on the document.
func TestNonAggregatorCompanies_MatchesWhenTheATSIsTheHyphenatedSide(t *testing.T) {
	searcher := func(_ context.Context, _ string, _ any, _ []string) (*meilisearch.SearchResponse, error) {
		// The stored document is "reid-health"; its folded field is what answers.
		dist, _ := json.Marshal(map[string]map[string]int64{
			"company_slug":        {"reid-health": 4},
			"company_slug_folded": {"reidhealth": 4},
		})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}
	covered, err := nonAggregatorCompanies(context.Background(), []string{"reidhealth"}, nil, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if !covered["reidhealth"] || len(covered) != 1 {
		t.Fatalf("covered = %v, want the caller's own slug reidhealth", covered)
	}
}

// A folded hit answers for every requested slug that folds to it, and the folded value is
// asked about ONCE however many slugs share it.
func TestNonAggregatorCompanies_FoldedHitAnswersEveryAsker(t *testing.T) {
	var gotFilter any
	searcher := func(_ context.Context, _ string, filter any, _ []string) (*meilisearch.SearchResponse, error) {
		gotFilter = filter
		dist, _ := json.Marshal(map[string]map[string]int64{"company_slug_folded": {"qtech": 2}})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}
	covered, err := nonAggregatorCompanies(context.Background(), []string{"q-tech", "qtech"}, nil, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if !covered["q-tech"] || !covered["qtech"] || len(covered) != 2 {
		t.Fatalf("covered = %v, want both askers credited", covered)
	}
	groups := gotFilter.([][]string)
	if n := strings.Count(groups[0][1], ",") + 1; n != 1 {
		t.Errorf("asked about %d folded values, want 1 (qtech, not twice)", n)
	}
}

// An exact hit on a company nobody asked about must not be credited to anyone. Meili returns
// the WHOLE facet distribution of the matched set, and the folded clause can match documents
// whose exact slug was never in the batch.
func TestNonAggregatorCompanies_IgnoresAnExactHitNobodyAskedAbout(t *testing.T) {
	searcher := func(_ context.Context, _ string, _ any, _ []string) (*meilisearch.SearchResponse, error) {
		dist, _ := json.Marshal(map[string]map[string]int64{
			"company_slug": {"reid-health": 4, "unrelated-co": 9},
		})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}
	covered, err := nonAggregatorCompanies(context.Background(), []string{"reid-health"}, nil, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if covered["unrelated-co"] {
		t.Errorf("covered = %v, want no credit for a company nobody asked about", covered)
	}
	if !covered["reid-health"] {
		t.Errorf("covered = %v, want reid-health", covered)
	}
}

// A slug that is nothing but hyphens folds to "", which as a filter value would match every
// document carrying no folded slug at all. It must be left out of the folded clause.
func TestFoldedBatchDropsASlugThatFoldsToNothing(t *testing.T) {
	folded, owners := foldedBatch([]string{"---", "acme"})
	if !slices.Equal(folded, []string{"acme"}) {
		t.Errorf("folded = %v, want only acme — an empty fold matches everything", folded)
	}
	if _, present := owners[""]; present {
		t.Errorf("owners = %v, want no empty-string key", owners)
	}
}

// TestNonAggregatorCompanies_EmptyAggregatorsOmitsNotInClause proves an empty aggregators
// list produces a filter with ONLY the company_slug IN clause — no stray `NOT IN [] `
// fragment (NotInStrings' empty-input guard is applied before the group is appended, not
// relied on Filter's own group-dropping, which would not catch a 1-element []string{""}).
func TestNonAggregatorCompanies_EmptyAggregatorsOmitsNotInClause(t *testing.T) {
	var gotFilter any
	searcher := func(_ context.Context, _ string, filter any, _ []string) (*meilisearch.SearchResponse, error) {
		gotFilter = filter
		return &meilisearch.SearchResponse{}, nil
	}
	if _, err := nonAggregatorCompanies(context.Background(), []string{"acme"}, nil, searcher); err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	filterStr := fmt.Sprintf("%v", gotFilter)
	if !strings.Contains(filterStr, `company_slug IN ["acme"]`) {
		t.Errorf("filter %v missing company_slug IN clause", gotFilter)
	}
	if strings.Contains(filterStr, "NOT IN") {
		t.Errorf("filter %v contains a stray NOT IN clause for empty aggregators", gotFilter)
	}
	groups, ok := gotFilter.([][]string)
	if !ok || len(groups) != 1 {
		t.Fatalf("filter groups = %#v, want exactly 1 group (company_slug only)", gotFilter)
	}
}

// TestNonAggregatorCompanies_BatchesLargeSlugLists proves a company-slug list larger than
// coverageBatchSize is split into multiple queries, and the results are unioned — the
// batching the buffered ingest path (a whole board's companies in one call) relies on.
func TestNonAggregatorCompanies_BatchesLargeSlugLists(t *testing.T) {
	n := coverageBatchSize + 3
	slugs := make([]string, n)
	for i := range slugs {
		slugs[i] = "company" + strconv.Itoa(i)
	}

	var mu sync.Mutex
	var batchSizes []int
	searcher := func(_ context.Context, _ string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
		mu.Lock()
		batchSizes = append(batchSizes, companySlugCount(t, filter))
		mu.Unlock()
		dist, _ := json.Marshal(map[string]map[string]int64{"company_slug": {"company0": 1}})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}

	if _, err := nonAggregatorCompanies(context.Background(), slugs, nil, searcher); err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if len(batchSizes) != 2 || batchSizes[0] != coverageBatchSize || batchSizes[1] != 3 {
		t.Fatalf("batch sizes = %v, want [%d, 3] (one full batch + one 3-element remainder)", batchSizes, coverageBatchSize)
	}
}

// companySlugCount extracts the number of comma-separated values in the company_slug IN [...]
// fragment of a Filter()-built filter (a [][]string under the any), so a test can assert on
// batch size without threading it through a side channel.
func companySlugCount(t *testing.T, filter any) int {
	t.Helper()
	groups, ok := filter.([][]string)
	if !ok || len(groups) == 0 || len(groups[0]) == 0 {
		t.Fatalf("filter = %#v, want a non-empty [][]string with a company_slug fragment first", filter)
	}
	return strings.Count(groups[0][0], ",") + 1
}

// TestNonAggregatorCompanies_UnionsAcrossBatches proves the covered set returned is the
// union of every batch's result, not just the last one.
func TestNonAggregatorCompanies_UnionsAcrossBatches(t *testing.T) {
	n := coverageBatchSize + 1
	slugs := make([]string, n)
	for i := range slugs {
		slugs[i] = "c" + strconv.Itoa(i)
	}

	call := 0
	searcher := func(_ context.Context, _ string, _ any, _ []string) (*meilisearch.SearchResponse, error) {
		call++
		key := "c0"
		if call == 2 {
			key = "c" + strconv.Itoa(coverageBatchSize)
		}
		dist, _ := json.Marshal(map[string]map[string]int64{"company_slug": {key: 1}})
		return &meilisearch.SearchResponse{FacetDistribution: dist}, nil
	}

	covered, err := nonAggregatorCompanies(context.Background(), slugs, nil, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if !covered["c0"] || !covered["c"+strconv.Itoa(coverageBatchSize)] || len(covered) != 2 {
		t.Fatalf("covered = %v, want both batches' hits unioned", covered)
	}
}

// TestNonAggregatorCompanies_EmptySlugsNoQuery proves an empty companySlugs list issues no
// query at all — there is nothing to ask.
func TestNonAggregatorCompanies_EmptySlugsNoQuery(t *testing.T) {
	called := false
	searcher := func(context.Context, string, any, []string) (*meilisearch.SearchResponse, error) {
		called = true
		return nil, nil
	}
	covered, err := nonAggregatorCompanies(context.Background(), nil, []string{"himalayas"}, searcher)
	if err != nil {
		t.Fatalf("nonAggregatorCompanies: %v", err)
	}
	if called {
		t.Error("searcher called for an empty companySlugs list")
	}
	if len(covered) != 0 {
		t.Errorf("covered = %v, want empty", covered)
	}
}

// TestNonAggregatorCompanies_ErrorPropagates proves a search failure on any batch aborts
// with an error rather than returning a partial/misleading covered set.
func TestNonAggregatorCompanies_ErrorPropagates(t *testing.T) {
	searcher := func(context.Context, string, any, []string) (*meilisearch.SearchResponse, error) {
		return nil, errors.New("meili down")
	}
	if _, err := nonAggregatorCompanies(context.Background(), []string{"acme"}, nil, searcher); err == nil {
		t.Fatal("nonAggregatorCompanies: want error, got nil")
	}
}
