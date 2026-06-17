package sources

import (
	"context"
	"slices"
	"testing"
)

func TestWorkingNomadsProvider(t *testing.T) {
	if got := NewWorkingNomads(nil).Provider(); got != "workingnomads" {
		t.Errorf("Provider() = %q, want workingnomads", got)
	}
}

func TestWorkingNomadsIsBoardlessAggregator(t *testing.T) {
	s := NewWorkingNomads(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("workingnomads should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("workingnomads should implement the aggregator marker")
	}
}

func TestWorkingNomadsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["workingnomads"]; !ok {
		t.Error("All() should register provider workingnomads")
	}
	if !slices.Contains(FilterableProviders(), "workingnomads") {
		t.Error("FilterableProviders() should include workingnomads")
	}
}

func TestWorkingNomadsBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/workingnomads.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/workingnomads.yml fails validation: %v", err)
	}
}

func TestWorkingNomadsFetchMaps(t *testing.T) {
	feed := `[
{"url":"https://www.workingnomads.com/job/go/1663269/","title":"Senior React Native Developer","description":"<p>Build apps.</p>","company_name":"Lemon.io","category_name":"Development","tags":"react native,react,nodejs","location":"Europe, North America","pub_date":"2026-06-12T11:32:31-04:00"},
{"url":"https://www.workingnomads.com/about/","title":"No numeric id","company_name":"Nope"}
]`
	fake := (&routedHTTP{}).route("exposed_jobs", feed)
	jobs, err := NewWorkingNomads(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (no-id posting dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1663269" {
		t.Errorf("ExternalID = %q, want 1663269 (parsed from URL)", j.ExternalID)
	}
	if j.Company != "Lemon.io" || j.Title != "Senior React Native Developer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.workingnomads.com/job/go/1663269/" || j.Location != "Europe, North America" {
		t.Errorf("URL=%q Location=%q", j.URL, j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote (workingnomads is remote-only)", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339")
	}
}
