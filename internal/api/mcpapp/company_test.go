package mcpapp

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestACompanySearchResultCarriesItsPageAndItsOpenCount(t *testing.T) {
	// The two things a person asking "who is hiring Go engineers in Berlin" acts on: where
	// to read more, and whether there is anything open at all.
	got := NewProjector(testOrigin).CompanySummary(search.CompanyDocument{
		Slug: "acme", Name: "Acme", Tagline: "We make things", JobCount: 12,
	})

	if got.URL != "https://freehire.me/companies/acme" {
		t.Errorf("url = %q, want the employer's page on freehire", got.URL)
	}
	if got.OpenJobs != 12 {
		t.Errorf("open_jobs = %d, want 12", got.OpenJobs)
	}
	if got.Slug != "acme" || got.Name != "Acme" || got.Tagline != "We make things" {
		t.Errorf("got %+v, want the stored identity", got)
	}
}

func TestACompanysHomeCountryIsPublishedUppercased(t *testing.T) {
	// Every geography facet is stored lowercase here while the published codes are ISO
	// 3166-1 alpha-2 — the same trap the OJCP surface documents for a posting's country.
	got := NewProjector(testOrigin).CompanyDetail(db.Company{
		Slug: "acme", Name: "Acme",
		HqCountry:  pgtype.Text{String: "de", Valid: true},
		Industries: []string{"fintech"},
		JobCount:   3,
	})

	if got.HQCountry != "DE" {
		t.Errorf("hq_country = %q, want DE", got.HQCountry)
	}
	if len(got.Industries) != 1 || got.Industries[0] != "fintech" {
		t.Errorf("industries = %v, want [fintech]", got.Industries)
	}
}

func TestANegativeOpenCountIsNeverPublished(t *testing.T) {
	// job_count is materialised by a rollup, so a floor here costs nothing and removes a
	// class of answer no reader should ever see.
	got := NewProjector(testOrigin).CompanyDetail(db.Company{Slug: "acme", JobCount: -4})

	if got.OpenJobs != 0 {
		t.Errorf("open_jobs = %d, want 0 rather than a negative figure", got.OpenJobs)
	}
}
