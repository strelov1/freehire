package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestWantedKRProvider(t *testing.T) {
	if got := NewWantedKR(nil).Provider(); got != "wantedkr" {
		t.Errorf("Provider() = %q, want wantedkr", got)
	}
}

func TestWantedKRIsBoardlessAggregator(t *testing.T) {
	s := NewWantedKR(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("wantedkr should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("wantedkr should implement the aggregator marker")
	}
}

func TestWantedKRRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["wantedkr"]; !ok {
		t.Error("All() should register provider wantedkr")
	}
	if !slices.Contains(FilterableProviders(), "wantedkr") {
		t.Error("FilterableProviders() should include wantedkr")
	}
}

func TestWantedKRBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/wantedkr.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/wantedkr.yml fails validation: %v", err)
	}
}

func TestWantedKRFetchListThenDetailAndMaps(t *testing.T) {
	list := `{"data":[{"id":369269}]}` // single item (< page size) ends pagination
	detail := `{"job":{"id":369269,"position":"Global Performance Marketer",
"company":{"name":"McKinleyRice"},
"address":{"full_location":"Seoul, Gangnam-gu","country":"한국"},
"detail":{"intro":"About us\nGlobal startup.","main_tasks":"Run campaigns","requirements":"4+ years","preferred_points":"India market","benefits":"Flexible hours"}}}`
	fake := (&routedHTTP{}).
		route("/api/v4/jobs/369269", detail).
		route("country=kr", list)

	jobs, err := NewWantedKR(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "369269" || j.Company != "McKinleyRice" || j.Title != "Global Performance Marketer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.wanted.co.kr/wd/369269" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Seoul, Gangnam-gu" {
		t.Errorf("Location = %q", j.Location)
	}
	for _, want := range []string{"About us", "Run campaigns", "4+ years", "India market", "Flexible hours"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %q", want, j.Description)
		}
	}
	if !strings.Contains(j.Description, "<br>") {
		t.Errorf("Description should turn newlines into <br>: %q", j.Description)
	}
}
