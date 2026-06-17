package sources

import (
	"context"
	"slices"
	"testing"
)

func TestJobicyProvider(t *testing.T) {
	if got := NewJobicy(nil).Provider(); got != "jobicy" {
		t.Errorf("Provider() = %q, want jobicy", got)
	}
}

func TestJobicyIsBoardlessAggregator(t *testing.T) {
	s := NewJobicy(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("jobicy should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobicy should implement the aggregator marker")
	}
}

func TestJobicyRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["jobicy"]; !ok {
		t.Error("All() should register provider jobicy")
	}
	if !slices.Contains(FilterableProviders(), "jobicy") {
		t.Error("FilterableProviders() should include jobicy")
	}
}

func TestJobicyBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/jobicy.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/jobicy.yml fails validation: %v", err)
	}
}

func TestJobicyFetchMaps(t *testing.T) {
	feed := `{"jobs":[
{"id":147014,"url":"https://jobicy.com/jobs/147014-service-tech","jobTitle":"Service Technician","companyName":"Municipal Emergency Services","jobGeo":"USA","jobDescription":"<p>Fix gear.</p>","pubDate":"2026-06-17T11:05:11+00:00"},
{"id":0,"companyName":"NoID","jobTitle":"skip"}
]}`
	fake := (&routedHTTP{}).route("remote-jobs", feed)
	jobs, err := NewJobicy(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (zero-id dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "147014" || j.Company != "Municipal Emergency Services" || j.Title != "Service Technician" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.Location != "USA" || j.WorkMode != "remote" {
		t.Errorf("Location=%q WorkMode=%q", j.Location, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339")
	}
}
