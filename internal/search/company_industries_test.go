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
}

// The attribute being filterable is not enough: a query parameter has to be mapped
// onto it, or the filter is unreachable from the API.
func TestCompanyFilterFromValuesAcceptsIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"fintech"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "fintech"`) {
		t.Errorf("industries filter missing, got %v", gs)
	}
}

// Several values of one facet are an OR within a single group, the same shape the
// other array facets use. Values arrive as a repeated parameter, not a
// comma-separated list — the company facets never split on commas.
func TestCompanyFilterFromValuesOrsSeveralIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"fintech", "healthcare"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "fintech"`, `industries = "healthcare"`) {
		t.Errorf("industries values should OR within one group, got %v", gs)
	}
}
