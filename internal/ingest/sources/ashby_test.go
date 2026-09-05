package sources

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAshbyProvider(t *testing.T) {
	if got := NewAshby(nil).Provider(); got != "ashby" {
		t.Errorf("Provider() = %q, want %q", got, "ashby")
	}
}

// Ashby earns fullBoardListing because Fetch is a single unpaginated request that returns the
// board's whole jobs array in one response — there is no loop that could stop early, so any
// listing failure aborts the whole Fetch rather than returning a partial result.
func TestAshbyMarkers(t *testing.T) {
	s := NewAshby(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("ashby should implement the fullBoardListing marker")
	}
}

func TestAshbyRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["ashby"] {
		t.Error("FullBoardListingProviders(All(nil)) should include ashby")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestAshbyMarkers' fullBoardListing claim rests on.
func TestAshbyFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestAshbyFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": "job-uuid",
				"title": "Platform Engineer",
				"location": "San Francisco",
				"jobUrl": "https://jobs.ashbyhq.com/ashby/job-uuid",
				"publishedAt": "2024-01-15T10:00:00.000Z",
				"descriptionPlain": "Run the platform.",
				"descriptionHtml": "<p>Run the <strong>platform</strong>.</p><script>x()</script>",
				"isRemote": true
			}
		]
	}`}

	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Ashby", Provider: "ashby", Board: "ashby",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(fake.gotURL, "ashby") {
		t.Errorf("requested URL %q should target the board", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "job-uuid" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "job-uuid")
	}
	if j.Title != "Platform Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.ashbyhq.com/ashby/job-uuid" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Ashby" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "San Francisco" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "<strong>platform</strong>") {
		t.Errorf("Description should be the sanitized descriptionHtml, got %q", j.Description)
	}
	if strings.Contains(j.Description, "<script") {
		t.Errorf("Description retained a script tag, got %q", j.Description)
	}
	// This posting carries no workplaceType, so isRemote is the fallback that resolves
	// the mode — and Remote follows from that resolved mode (see MapAshbyPosting).
	if !j.Remote {
		t.Error("Remote = false, want true from the isRemote fallback")
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote from the isRemote fallback", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want parsed publishedAt with milliseconds")
	}
}

// Ashby sets isRemote on every posting that is not strictly onsite, so a hybrid role
// carries isRemote=true alongside workplaceType="Hybrid". workplaceType is the field the
// Ashby board itself renders as "Location Type", so it decides the work mode.
func TestAshbyFetchWorkplaceTypeBeatsIsRemote(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": "hybrid-uuid",
				"title": "Senior Web Designer",
				"location": "Vilnius",
				"jobUrl": "https://jobs.ashbyhq.com/surfshark/hybrid-uuid",
				"publishedAt": "2026-07-28T09:35:42.943+00:00",
				"descriptionHtml": "<p>Design it.</p>",
				"isRemote": true,
				"workplaceType": "Hybrid"
			}
		]
	}`}

	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Surfshark", Provider: "ashby", Board: "surfshark",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from workplaceType", j.WorkMode)
	}
	// A hybrid role requires office presence, so it is not remote — and its location
	// text carries no "remote" to trigger the heuristic either.
	if j.Remote {
		t.Error("Remote = true, want false: a hybrid role is not remote")
	}
}

// Ashby states the country as an alpha-3 code under address.postalAddress.addressCountry
// — a structured field the location string ("New York, NY (HQ)") does not spell out.
func TestAshbyFetchDecodesStructuredCountry(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": "ny-uuid",
				"title": "Backend Engineer",
				"location": "New York, NY (HQ)",
				"jobUrl": "https://jobs.ashbyhq.com/ramp/ny-uuid",
				"address": {"postalAddress": {"addressRegion": "NY", "addressCountry": "USA"}}
			}
		]
	}`}

	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Ramp", Provider: "ashby", Board: "ramp",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if got, want := jobs[0].Countries, []string{"us"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Countries = %v, want %v (normalized from the alpha-3 addressCountry)", got, want)
	}
}

// A posting whose address carries no addressCountry (most non-US boards) leaves
// Countries nil rather than guessing from the free-text location.
func TestAshbyFetchNoAddressLeavesCountriesEmpty(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{"id": "no-addr", "title": "Support Engineer", "location": "Remote"}
		]
	}`}

	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "ashby", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := jobs[0].Countries; got != nil {
		t.Errorf("Countries = %v, want nil", got)
	}
}

func TestAshbyFetchRequestsCompensation(t *testing.T) {
	fake := &fakeHTTP{body: `{"jobs": []}`}
	if _, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "ashby", Board: "acme"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "includeCompensation=true") {
		t.Errorf("requested URL %q should include includeCompensation=true — omitted, the API drops the field entirely", fake.gotURL)
	}
}

// Live-verified (2026-08-14): a real OpenAI posting carries exactly this shape — a
// Salary component alongside an EquityCashValue one, which must be ignored.
func TestAshbyFetchReadsSalaryComponent(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [{
			"id": "1", "title": "Research Engineer",
			"compensation": {
				"compensationTiers": [{
					"components": [
						{"compensationType": "Salary", "interval": "1 YEAR", "currencyCode": "USD", "minValue": 257000, "maxValue": 335000},
						{"compensationType": "EquityCashValue", "interval": "1 YEAR", "currencyCode": "USD", "minValue": null, "maxValue": null}
					]
				}]
			}
		}]
	}`}
	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{Company: "OpenAI", Provider: "ashby", Board: "openai"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := jobs[0]
	if j.SalaryMin == nil || *j.SalaryMin != 257000 {
		t.Errorf("SalaryMin = %v, want 257000", j.SalaryMin)
	}
	if j.SalaryMax == nil || *j.SalaryMax != 335000 {
		t.Errorf("SalaryMax = %v, want 335000", j.SalaryMax)
	}
	if j.SalaryCurrency != "USD" || j.SalaryPeriod != "year" {
		t.Errorf("SalaryCurrency/SalaryPeriod = %q/%q, want USD/year", j.SalaryCurrency, j.SalaryPeriod)
	}
}

func TestAshbyFetchIgnoresNonSalaryComponents(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [{
			"id": "1", "title": "Sales Rep",
			"compensation": {
				"compensationTiers": [{
					"components": [
						{"compensationType": "Commission", "interval": "1 YEAR", "currencyCode": "USD", "minValue": null, "maxValue": null},
						{"compensationType": "Bonus", "interval": "1 YEAR", "currencyCode": "USD", "minValue": null, "maxValue": null}
					]
				}]
			}
		}]
	}`}
	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "ashby", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if j := jobs[0]; j.SalaryMin != nil || j.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want both nil (no Salary-typed component present)", j.SalaryMin, j.SalaryMax)
	}
}
