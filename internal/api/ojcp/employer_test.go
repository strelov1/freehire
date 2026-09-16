package ojcp

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func acmeCompany() db.Company {
	return db.Company{
		Slug:       "acme",
		Name:       "Acme Corp",
		Tagline:    pgtype.Text{String: "Industrial innovation since 1946", Valid: true},
		Industries: []string{"manufacturing", "logistics"},
		HqCountry:  pgtype.Text{String: "us", Valid: true},
		JobCount:   42,
	}
}

func TestEmployerContextCarriesWhatOnePostingCannotSay(t *testing.T) {
	resp := EmployerContextFrom(acmeCompany())

	if resp.EmployerID != "acme" {
		t.Errorf("employer_id = %q, want the company slug an agent read off a posting", resp.EmployerID)
	}
	if resp.Name != "Acme Corp" {
		t.Errorf("name = %q", resp.Name)
	}
	if resp.OpenRolesCount != 42 {
		t.Errorf("open_roles_count = %d, want the open posting count", resp.OpenRolesCount)
	}
	if len(resp.Industries) != 2 {
		t.Errorf("industries = %v", resp.Industries)
	}
	if err := validateAgainstSchema(t, schemaEmployerContextResponse, resp); err != nil {
		t.Fatalf("employer context rejected: %v", err)
	}
}

func TestEmployerHQCountryIsAnISOCode(t *testing.T) {
	// Stored lowercase like every other geography facet; the standard's country fields are
	// ISO 3166-1 alpha-2, canonically uppercase — the same trap addressCountry fell into.
	resp := EmployerContextFrom(acmeCompany())

	if resp.HQLocation == nil {
		t.Fatal("hq_location is nil though the company states a country")
	}
	if resp.HQLocation.Country != "US" {
		t.Errorf("hq_location.country = %q, want US", resp.HQLocation.Country)
	}
}

func TestEmployerSaysNothingItDoesNotKnow(t *testing.T) {
	// A company with no tagline, no industries and no HQ is the ordinary case for most of
	// the catalogue. Empty strings and a hollow location block would each read as a fact.
	resp := EmployerContextFrom(db.Company{Slug: "unknown-co", Name: "Unknown Co"})

	if resp.Description != "" {
		t.Errorf("description = %q, want empty", resp.Description)
	}
	if resp.HQLocation != nil {
		t.Errorf("hq_location = %+v, want nil", resp.HQLocation)
	}
	if err := validateAgainstSchema(t, schemaEmployerContextResponse, resp); err != nil {
		t.Fatalf("bare employer context rejected: %v", err)
	}
}

func TestEmployerOpenRolesCountIsNeverNegative(t *testing.T) {
	// The schema types it as an integer with no lower bound, so a negative would validate
	// and then read to an agent as a real figure.
	company := acmeCompany()
	company.JobCount = -1

	resp := EmployerContextFrom(company)

	if resp.OpenRolesCount != 0 {
		t.Errorf("open_roles_count = %d, want it floored at zero", resp.OpenRolesCount)
	}
}
