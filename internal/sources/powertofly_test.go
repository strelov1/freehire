package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestPowerToFlyProvider(t *testing.T) {
	if got := NewPowerToFly(nil).Provider(); got != "powertofly" {
		t.Errorf("Provider() = %q, want powertofly", got)
	}
}

func TestPowerToFlyIsBoardlessAggregator(t *testing.T) {
	s := NewPowerToFly(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("powertofly should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("powertofly should implement the aggregator marker")
	}
}

func TestPowerToFlyRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["powertofly"]; !ok {
		t.Error("All() should register provider powertofly")
	}
	if !slices.Contains(FilterableProviders(), "powertofly") {
		t.Error("FilterableProviders() should include powertofly")
	}
}

func TestPowerToFlyBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/powertofly.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/powertofly.yml fails validation: %v", err)
	}
}

func TestPowerToFlyFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"id":1691729,"title":"Senior Go Engineer","description":"<p>Build &amp; ship.</p>","location":"Remote","location_regions":["USA"],"employment_type":"Full Time","company":{"name":"NASCAR"},"country":{"title":"United States"},"state":{"title":"Alabama"},"city":{"title":"Talladega"}},
{"id":0,"title":"skip me","company":{"name":"NoID"}}
],"meta":{"next_page":2}}`
	page2 := `{"data":[
{"id":42,"title":"Ops Lead","description":"x","location":"Onsite","location_regions":["USA"],"employment_type":"Per Project","company":{"name":"Acme"},"country":{"title":"United States"}}
],"meta":{"next_page":null}}`
	// page=2 routed first so the more specific match wins over the base ?page=1 route.
	fake := (&routedHTTP{}).route("page=2", page2).route("page=1", page1)

	jobs, err := NewPowerToFly(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the id=0 posting dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "1691729" || j.Company != "NASCAR" || j.Title != "Senior Go Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://powertofly.com/jobs/detail/1691729" {
		t.Errorf("URL = %q, want the powertofly detail URL", j.URL)
	}
	// "location":"Remote" is the work arrangement, not geography — geography comes from city/state/country.
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.Location != "Talladega, Alabama, United States" {
		t.Errorf("Location = %q, want the built city/state/country", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}

	// page 2: onsite arrangement, "Per Project" → contract, geography falls back through the chain.
	j2 := jobs[1]
	if j2.WorkMode != "onsite" || j2.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want onsite/false", j2.WorkMode, j2.Remote)
	}
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j2.EmploymentType)
	}
}
