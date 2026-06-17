package sources

import (
	"context"
	"slices"
	"testing"
)

func TestJustJoinProvider(t *testing.T) {
	if got := NewJustJoin(nil).Provider(); got != "justjoin" {
		t.Errorf("Provider() = %q, want justjoin", got)
	}
}

func TestJustJoinIsBoardlessAggregator(t *testing.T) {
	s := NewJustJoin(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("justjoin should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("justjoin should implement the aggregator marker")
	}
}

func TestJustJoinRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["justjoin"]; !ok {
		t.Error("All() should register provider justjoin")
	}
	if !slices.Contains(FilterableProviders(), "justjoin") {
		t.Error("FilterableProviders() should include justjoin")
	}
}

func TestJustJoinBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/justjoin.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/justjoin.yml fails validation: %v", err)
	}
}

func TestJustJoinFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"guid":"135d4e26","slug":"coder-security-engineer--warszawa","title":"Security Engineer","companyName":"Coder","city":"Warszawa","workplaceType":"remote","publishedAt":"2026-05-19T00:41:27.040Z"},
{"guid":"noslug","slug":"","title":"drop","companyName":"Ghost"}
],"meta":{"next":{"cursor":20}}}`
	page2 := `{"data":[
{"guid":"77aa","slug":"acme-go-dev--krakow","title":"Go Developer","companyName":"Acme","city":"Kraków","workplaceType":"office","publishedAt":"2026-05-18T10:00:00.000Z"}
],"meta":{"next":null}}`
	fake := (&routedHTTP{}).route("from=20", page2).route("by-cursor", page1)
	jobs, err := NewJustJoin(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per cursor page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (empty-slug dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "135d4e26" || j.Company != "Coder" || j.Title != "Security Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://justjoin.it/job-offer/coder-security-engineer--warszawa" {
		t.Errorf("URL = %q, want synthesized job-offer URL", j.URL)
	}
	if j.Location != "Warszawa" {
		t.Errorf("Location = %q, want city", j.Location)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (from workplaceType)", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339 with millis")
	}
	// Second page's "office" workplaceType maps to onsite.
	if jobs[1].WorkMode != "onsite" {
		t.Errorf("jobs[1].WorkMode = %q, want onsite (office)", jobs[1].WorkMode)
	}
}
