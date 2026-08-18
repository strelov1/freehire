//go:build integration

// Integration tests for the Meilisearch-backed companies index: a full rebuild
// (CompanyRebuild swap) followed by ranked search — relevance-first ordering (an
// exact name match ahead of higher-volume prefix matches), typo tolerance, and facet
// filtering. These exercise behavior only a real engine exhibits. Run with:
//
//	go test -tags=integration ./internal/search/
//
// Requires Docker (reuses startMeili from search_integration_test.go).
package search

import (
	"context"
	"net/url"
	"testing"
)

// buildCompanyIndex runs a full CompanyRebuild over docs and returns the client.
func buildCompanyIndex(t *testing.T, c *Client, docs []CompanyDocument) {
	t.Helper()
	ctx := context.Background()
	r := c.NewCompanyRebuild()
	if err := r.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := r.Push(ctx, docs); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := r.Promote(ctx); err != nil {
		t.Fatalf("Promote: %v", err)
	}
}

func TestIntegration_CompanySearch_ExactNameRanksFirst(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	// "arb" (2 jobs) must outrank "arbor"/"arbitrage" (far more jobs) that only prefix-
	// match the query — the exactness ranking rule wins over the job_count tiebreaker.
	buildCompanyIndex(t, c, []CompanyDocument{
		{Slug: "arbor", Name: "Arbor", JobCount: 40},
		{Slug: "arbitrage-labs", Name: "Arbitrage Labs", JobCount: 99},
		{Slug: "arb", Name: "arb", JobCount: 2},
	})

	res, err := c.SearchCompanies(ctx, CompanySearchParams{Query: "arb", Limit: 10})
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("no hits for q=arb")
	}
	if res.Hits[0].Slug != "arb" {
		t.Errorf("first hit = %q, want arb (exact name must rank first despite low job_count); hits=%v", res.Hits[0].Slug, slugs(res.Hits))
	}
	if res.Total < 1 {
		t.Errorf("total = %d, want >= 1", res.Total)
	}
}

func TestIntegration_CompanySearch_ToleratesTypo(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	buildCompanyIndex(t, c, []CompanyDocument{
		{Slug: "airbnb", Name: "Airbnb", JobCount: 500},
		{Slug: "globex", Name: "Globex", JobCount: 10},
	})

	// "arbnb" is one edit from "airbnb" — typo tolerance should still find it.
	res, err := c.SearchCompanies(ctx, CompanySearchParams{Query: "arbnb", Limit: 10})
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	if !containsSlug(res.Hits, "airbnb") {
		t.Errorf("typo query 'arbnb' did not resolve to airbnb; hits=%v", slugs(res.Hits))
	}
}

func TestIntegration_CompanySearch_FacetFilter(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	buildCompanyIndex(t, c, []CompanyDocument{
		{Slug: "euro-lab", Name: "Euro Lab", JobCount: 5, Regions: []string{"europe"}, CompanyTypes: []string{"startup"}},
		{Slug: "asia-co", Name: "Asia Co", JobCount: 5, Regions: []string{"asia"}, CompanyTypes: []string{"product"}},
		{Slug: "euro-corp", Name: "Euro Corp", JobCount: 5, Regions: []string{"europe"}, CompanyTypes: []string{"enterprise"}},
	})

	// regions=europe AND company_type=startup → only euro-lab.
	filter := CompanyFilterFromValues(url.Values{"regions": {"europe"}, "company_type": {"startup"}})
	res, err := c.SearchCompanies(ctx, CompanySearchParams{Filter: filter, Limit: 10})
	if err != nil {
		t.Fatalf("SearchCompanies: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Slug != "euro-lab" {
		t.Errorf("facet filter → %v, want [euro-lab]", slugs(res.Hits))
	}
}

func slugs(hits []CompanyDocument) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Slug
	}
	return out
}

func containsSlug(hits []CompanyDocument, slug string) bool {
	for _, h := range hits {
		if h.Slug == slug {
			return true
		}
	}
	return false
}

// The industry facet's derived arm is a parenthesised conjunction inside an OR group
// — a filter shape nothing else in this codebase builds. A unit test can only assert
// the string; whether Meilisearch PARSES it is a property of the engine, and getting
// it wrong is a 500 on every industry-filtered request rather than a wrong result.
func TestIntegration_CompanySearch_IndustryPrecedence(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	buildCompanyIndex(t, c, []CompanyDocument{
		// Curated: matches through its own column.
		{Slug: "curated-fin", Name: "Curated Fin", JobCount: 5, Industries: []string{"fintech"}},
		// Uncurated: matches through the domain that means the same thing.
		// Two shapes of "no curated industry": a nil slice serializes to null, an
		// empty one to []. Meilisearch does not treat them as the same thing, so both
		// are here — testing only one would let a filter that handles only that one pass.
		{Slug: "derived-nil", Name: "Derived Nil", JobCount: 5, Domains: []string{"fintech"}},
		{Slug: "derived-empty", Name: "Derived Empty", JobCount: 5, Industries: []string{}, Domains: []string{"fintech"}},
		// Curated as something else, with a domains union that drifted — the
		// production shape that made ?industries=gaming return Uber. Its curated
		// column answers for it; the drift must not.
		{Slug: "big-classified", Name: "Big Classified", JobCount: 5,
			Industries: []string{"logistics"}, Domains: []string{"fintech", "gamedev"}},
	})

	filter := CompanyFilterFromValues(url.Values{"industries": {"fintech"}})
	res, err := c.SearchCompanies(ctx, CompanySearchParams{Filter: filter, Limit: 10})
	if err != nil {
		t.Fatalf("SearchCompanies: %v (does Meilisearch accept the conjunction?)", err)
	}

	got := slugs(res.Hits)
	for _, want := range []string{"curated-fin", "derived-nil", "derived-empty"} {
		if !contains(got, want) {
			t.Errorf("industries=fintech → %v, missing %s", got, want)
		}
	}
	if len(got) != 3 {
		t.Errorf("industries=fintech → %v, want exactly the three above", got)
	}
	if contains(got, "big-classified") {
		t.Error("a company curated as logistics was matched through its domains")
	}
}
