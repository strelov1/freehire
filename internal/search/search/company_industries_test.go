package search

import (
	"net/url"
	"slices"
	"testing"
)

// The column is useless as a facet until Meilisearch is told it may be filtered.
// Until this attribute is in the index settings, a query naming it 500s.
func TestCompanySettingsMakesIndustriesFilterable(t *testing.T) {
	s := companySettings()
	if !slices.Contains(s.FilterableAttributes, "industries") {
		t.Errorf("industries must be filterable, got %v", s.FilterableAttributes)
	}
	if !slices.Contains(s.FilterableAttributes, "industries_derived") {
		t.Errorf("industries_derived must be filterable, got %v", s.FilterableAttributes)
	}
}

// The facet reaches a company through either attribute: the curated `industries` an
// importer wrote, and `industries_derived` — RefreshCompanyFacets' materialized
// translation of the company's own domains, already empty wherever precedence or the
// domain-count threshold says it should be. So a requested industry becomes two
// fragments in one OR group, with no runtime emptiness/count check at this layer.
func TestCompanyFilterFromValuesAcceptsIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"fintech"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "fintech"`, `industries_derived = "fintech"`) {
		t.Errorf("industries filter missing its derived arm, got %v", gs)
	}
}

// Several values of one facet are an OR within a single group, the same shape the
// other array facets use. Values arrive as a repeated parameter, not a
// comma-separated list — the company facets never split on commas.
func TestCompanyFilterFromValuesOrsSeveralIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"healthcare", "fintech"}})

	gs := groups(t, f)
	if !hasGroup(gs,
		`industries = "healthcare"`, `industries = "fintech"`,
		`industries_derived = "healthcare"`, `industries_derived = "fintech"`,
	) {
		t.Errorf("industries values should OR within one group, got %v", gs)
	}
}

// Widening industries must not leak into the facet it borrows from: `domains` still
// filters the raw job-derived value, including the ones no industry names.
func TestCompanyFilterFromValuesLeavesDomainsFacetAlone(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"domains": {"other"}})

	gs := groups(t, f)
	if !hasGroup(gs, `domains = "other"`) {
		t.Errorf("the domains facet should be untouched, got %v", gs)
	}
	if len(gs) != 1 {
		t.Errorf("no other group should appear, got %v", gs)
	}
}
