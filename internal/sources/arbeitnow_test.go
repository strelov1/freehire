package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestArbeitnowProvider(t *testing.T) {
	if got := NewArbeitnow(nil).Provider(); got != "arbeitnow" {
		t.Errorf("Provider() = %q, want arbeitnow", got)
	}
}

func TestArbeitnowIsBoardlessAggregator(t *testing.T) {
	s := NewArbeitnow(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("arbeitnow should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("arbeitnow should implement the aggregator marker")
	}
}

func TestArbeitnowRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["arbeitnow"]; !ok {
		t.Error("All() should register provider arbeitnow")
	}
	if !slices.Contains(FilterableProviders(), "arbeitnow") {
		t.Error("FilterableProviders() should include arbeitnow")
	}
}

func TestArbeitnowBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/arbeitnow.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/arbeitnow.yml fails validation: %v", err)
	}
}

func TestArbeitnowFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"slug":"data-engineer-berlin-1","company_name":"Passerelle","title":"Data Engineer","description":"<p>Build &amp; ship.</p>","remote":true,"url":"https://www.arbeitnow.com/jobs/companies/passerelle/data-engineer-berlin-1","location":"Berlin","created_at":1781713837},
{"slug":"","company_name":"NoID","title":"skip me","url":"x","location":"Berlin","created_at":1}
],"links":{"next":"https://www.arbeitnow.com/api/job-board-api?page=2"}}`
	page2 := `{"data":[],"links":{"next":null}}`
	// page=2 routed first so the more specific match wins over the base job-board-api route.
	fake := (&routedHTTP{}).route("page=2", page2).route("job-board-api", page1)

	jobs, err := NewArbeitnow(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the empty-slug posting dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "data-engineer-berlin-1" || j.Company != "Passerelle" || j.Title != "Data Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	// The API description is already real HTML, so the valid &amp; entity is preserved.
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch")
	}
}
