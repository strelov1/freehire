package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestGetonbrdProvider(t *testing.T) {
	if got := NewGetonbrd(nil).Provider(); got != "getonbrd" {
		t.Errorf("Provider() = %q, want getonbrd", got)
	}
}

func TestGetonbrdIsBoardlessAggregator(t *testing.T) {
	s := NewGetonbrd(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("getonbrd should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("getonbrd should implement the aggregator marker")
	}
}

func TestGetonbrdRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["getonbrd"]; !ok {
		t.Error("All() should register provider getonbrd")
	}
	if !slices.Contains(FilterableProviders(), "getonbrd") {
		t.Error("FilterableProviders() should include getonbrd")
	}
}

func TestGetonbrdBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/getonbrd.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/getonbrd.yml fails validation: %v", err)
	}
}

func TestGetonbrdWorkMode(t *testing.T) {
	cases := map[string]string{"fully_remote": "remote", "remote": "remote", "hybrid": "hybrid", "no_remote": "onsite", "weird": ""}
	for in, want := range cases {
		if got := getonbrdWorkMode(in); got != want {
			t.Errorf("getonbrdWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetonbrdFetchEnumeratesResolvesCompanyAndMaps(t *testing.T) {
	categories := `{"data":[{"id":"programming"}]}`
	jobsPage := `{"data":[
{"id":"engineering-manager-rankmi-santiago","attributes":{"title":"Engineering Manager","description":"<ul><li>Reqs</li></ul>","functions":"<ul><li>Lead</li></ul>","benefits":"<div>Perks</div>","remote_modality":"hybrid","countries":["Chile"],"published_at":1781711852,"company":{"data":{"id":1339}}}}
],"meta":{"total_pages":1}}`
	company := `{"data":{"attributes":{"name":"Rankmi"}}}`
	// Order matters: most specific substrings first (routedHTTP matches first hit).
	fake := (&routedHTTP{}).
		route("/companies/1339", company).
		route("/categories/programming/jobs", jobsPage).
		route("/api/v0/categories", categories)

	jobs, err := NewGetonbrd(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "engineering-manager-rankmi-santiago" || j.Company != "Rankmi" {
		t.Errorf("id/company wrong: %s / %s", j.ExternalID, j.Company)
	}
	if j.URL != "https://www.getonbrd.com/jobs/engineering-manager-rankmi-santiago" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.WorkMode != "hybrid" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want hybrid/true", j.WorkMode, j.Remote)
	}
	if j.Location != "Chile" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "Lead") || !strings.Contains(j.Description, "Reqs") || !strings.Contains(j.Description, "Perks") {
		t.Errorf("Description should assemble functions+description+benefits: %q", j.Description)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch")
	}
}
