package ojcp

import "testing"

func TestJobLocationNamesAnUnambiguousPlace(t *testing.T) {
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Cities = []string{"Berlin"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	if posting.JobLocation == nil {
		t.Fatal("jobLocation is nil for a posting with one city in one country")
	}
	if got := posting.JobLocation.Address.AddressCountry; got != "DE" {
		t.Errorf("addressCountry = %q, want DE", got)
	}
	if got := posting.JobLocation.Address.AddressLocality; got != "Berlin" {
		t.Errorf("addressLocality = %q, want Berlin", got)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with a location rejected: %v", err)
	}
}

func TestJobLocationStaysSilentWhereThePostingSpansSeveralPlaces(t *testing.T) {
	// The schema shapes jobLocation as ONE schema.org Place, and a posting open in three
	// countries does not have one. Picking the first element would state a restriction the
	// employer never made — an agent filtering on it would drop the posting everywhere else.
	row := openPostingRow()
	row.Countries = []string{"DE", "PL", "ES"}
	row.Cities = []string{"Berlin", "Warsaw"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	if posting.JobLocation != nil {
		t.Errorf("jobLocation = %+v, want nil for a multi-country posting", posting.JobLocation)
	}
}

func TestJobLocationKeepsTheHalfItDoesKnow(t *testing.T) {
	// One country, several cities within it: the country is still a fact, so it is stated
	// and the city is not. Withholding both because one is ambiguous serves less than the
	// posting says.
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Cities = []string{"Berlin", "Munich"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	if posting.JobLocation == nil {
		t.Fatal("jobLocation is nil though the country is unambiguous")
	}
	if posting.JobLocation.Address.AddressCountry != "DE" {
		t.Errorf("addressCountry = %q, want DE", posting.JobLocation.Address.AddressCountry)
	}
	if got := posting.JobLocation.Address.AddressLocality; got != "" {
		t.Errorf("addressLocality = %q, want empty with two cities", got)
	}
}

func TestJobLocationNeverBorrowsOurMacroRegionForAddressRegion(t *testing.T) {
	// Our Regions are macro-regions — "europe", "global" — while schema.org's addressRegion
	// is a state or province. They are different kinds of thing, and an agent reading
	// addressRegion="europe" would file the posting under a province by that name.
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Regions = []string{"europe"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	if posting.JobLocation == nil {
		t.Fatal("jobLocation is nil though the country is unambiguous")
	}
	if got := posting.JobLocation.Address.AddressRegion; got != "" {
		t.Errorf("addressRegion = %q, want empty — a macro-region is not a province", got)
	}
}

func TestJobLocationIsAbsentWhenNoPlaceIsKnown(t *testing.T) {
	// An empty Place block says "we know where this is" and then says nothing. Absence is
	// the honest rendering.
	posting := jobPostingFrom(openPosting(), testOrigin)

	if posting.JobLocation != nil {
		t.Errorf("jobLocation = %+v, want nil with no geography at all", posting.JobLocation)
	}
}

func TestJobPostingCarriesTheDescription(t *testing.T) {
	// get_job_detail exists to hand an agent the posting's own text; without this field it
	// would return everything about a job except what the job is.
	j := openPosting()
	j.Description = "We are looking for a Go engineer to work on our ingest pipeline."

	posting := jobPostingFrom(j, testOrigin)

	if posting.Description != j.Description {
		t.Errorf("description = %q, want the posting's own text", posting.Description)
	}
}
