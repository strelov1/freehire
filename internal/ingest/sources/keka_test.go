package sources

import (
	"context"
	"strings"
	"testing"
)

func TestKekaProvider(t *testing.T) {
	if got := NewKeka(nil).Provider(); got != "keka" {
		t.Errorf("Provider() = %q, want %q", got, "keka")
	}
}

// Keka earns fullBoardListing because the jobs-active API returns the board's whole
// open-postings array in one request.
func TestKekaMarkers(t *testing.T) {
	s := NewKeka(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("keka should implement the fullBoardListing marker")
	}
}

func TestKekaRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["keka"] {
		t.Error("FullBoardListingProviders(All(nil)) should include keka")
	}
}

func TestKekaRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["keka"]
	if !ok {
		t.Fatal("All() missing provider keka")
	}
	if s.Provider() != "keka" {
		t.Errorf("All()[keka].Provider() = %q", s.Provider())
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestKekaMarkers' fullBoardListing claim rests on.
func TestKekaFetchPropagatesAListingError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewKeka(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestKekaFetchListsPostings(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/careers/api/organization/default/careerportalinfo", `{"name": "Acme Technologies"}`).
		route("/careers/api/jobs/default/active", `[
			{"id": 160039, "title": "Sr. Solution Architect",
			 "description": "<div>Do the work.</div>",
			 "jobLocations": [{"city": "Hyderabad", "state": "TG", "countryName": "India"}],
			 "jobType": 2, "publishedOn": "2026-09-07T08:18:34.693Z"},
			{"id": 160038, "title": "HR Intern",
			 "description": "<div>Assist the team.</div>",
			 "jobLocations": [],
			 "jobType": 1, "publishedOn": "2026-05-01T00:00:00Z"}
		]`)

	jobs, err := NewKeka(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "keka", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["160039"]
	if !ok {
		t.Fatal("job 160039 missing")
	}
	if j.Title != "Sr. Solution Architect" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme Technologies" {
		t.Errorf("Company = %q, want the careerportalinfo name", j.Company)
	}
	if j.URL != "https://acme.keka.com/careers/jobdetails/160039" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Hyderabad, TG, India" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (jobType 2)", j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Do the work.") {
		t.Errorf("Description = %q, want the sanitized HTML", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed publishedOn (2026)", j.PostedAt)
	}

	intern := byID["160038"]
	if intern.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time (jobType 1)", intern.EmploymentType)
	}
	if intern.Location != "" {
		t.Errorf("Location = %q, want empty (no jobLocations)", intern.Location)
	}
}

// The org-name lookup is best-effort: its failure must not abort the crawl, and the board's
// configured Company name stands in.
func TestKekaFetchFallsBackToConfiguredCompanyNameOnInfoFailure(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/careers/api/jobs/default/active", `[
			{"id": 1, "title": "Engineer", "description": "<div>Work.</div>",
			 "jobLocations": [], "jobType": 2, "publishedOn": "2026-01-01T00:00:00Z"}
		]`)
		// no careerportalinfo route: routedHTTP fails any unrouted URL

	jobs, err := NewKeka(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "keka", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort on a failed org-name lookup: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme" {
		t.Fatalf("jobs = %+v, want one job with Company falling back to the configured name", jobs)
	}
}
