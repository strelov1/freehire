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

// The facet reaches a company through either vocabulary, so one requested industry
// becomes two fragments in one OR group: the curated column, and the job-derived
// domain that means the same thing. Here the two vocabularies spell it the same.
func TestCompanyFilterFromValuesAcceptsIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"fintech"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "fintech"`, `((industries IS EMPTY OR industries IS NULL) AND domains = "fintech")`) {
		t.Errorf("industries filter missing its domain arm, got %v", gs)
	}
}

// The spellings differ more often than not, which is the whole reason a mapping
// exists rather than a string comparison.
func TestCompanyFilterFromValuesTranslatesIndustryToItsDomain(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"developer-tools"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "developer-tools"`, `((industries IS EMPTY OR industries IS NULL) AND domains = "devtools")`) {
		t.Errorf("developer-tools should also match the devtools domain, got %v", gs)
	}
}

// Several values of one facet are an OR within a single group, the same shape the
// other array facets use. Values arrive as a repeated parameter, not a
// comma-separated list — the company facets never split on commas. The curated
// fragments keep request order; the domain fragments follow, sorted.
func TestCompanyFilterFromValuesOrsSeveralIndustries(t *testing.T) {
	f := CompanyFilterFromValues(url.Values{"industries": {"healthcare", "fintech"}})

	gs := groups(t, f)
	if !hasGroup(gs,
		`industries = "healthcare"`, `industries = "fintech"`,
		`((industries IS EMPTY OR industries IS NULL) AND domains = "fintech")`, `((industries IS EMPTY OR industries IS NULL) AND domains = "healthcare")`,
	) {
		t.Errorf("industries values should OR within one group, got %v", gs)
	}
}

// An industry the mapping deliberately does not cover must not widen the filter.
// Emitting a fragment for it would either match nothing (harmless but noisy) or,
// worse, match a domain that misdescribes the company.
func TestCompanyFilterFromValuesLeavesUnmappedIndustryAlone(t *testing.T) {
	// accounting is a curated industry the coarse domain vocabulary simply has no
	// value for — unlike entertainment or transportation, which do map.
	f := CompanyFilterFromValues(url.Values{"industries": {"accounting"}})

	gs := groups(t, f)
	if !hasGroup(gs, `industries = "accounting"`) {
		t.Errorf("an unmapped industry should filter on the curated column alone, got %v", gs)
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
