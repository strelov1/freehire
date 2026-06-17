package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMyCareersFutureProvider(t *testing.T) {
	if got := NewMyCareersFuture(nil).Provider(); got != "mycareersfuture" {
		t.Errorf("Provider() = %q, want mycareersfuture", got)
	}
}

func TestMyCareersFutureIsBoardlessAggregator(t *testing.T) {
	s := NewMyCareersFuture(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("mycareersfuture should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("mycareersfuture should implement the aggregator marker")
	}
}

func TestMyCareersFutureRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["mycareersfuture"]; !ok {
		t.Error("All() should register provider mycareersfuture")
	}
	if !slices.Contains(FilterableProviders(), "mycareersfuture") {
		t.Error("FilterableProviders() should include mycareersfuture")
	}
}

func TestMyCareersFutureBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/mycareersfuture.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/mycareersfuture.yml fails validation: %v", err)
	}
}

func TestMyCareersFutureFetchMapsAndPaginationStops(t *testing.T) {
	// One result (< page size) ends pagination after the first page.
	page := `{"results":[
{"uuid":"7386f4f23f6bab00d906bb9e0f33e4b4","title":"Software Engineer","description":"<p>Build &amp; ship.</p>","postedCompany":{"name":"Royal Org Pte Ltd"},"address":{"isOverseas":false},"metadata":{"newPostingDate":"2026-06-18"}},
{"uuid":"","title":"NoID","postedCompany":{"name":"x"}}
],"total":2}`
	fake := (&routedHTTP{}).route("/v2/jobs", page)
	jobs, err := NewMyCareersFuture(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (empty-uuid dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "7386f4f23f6bab00d906bb9e0f33e4b4" || j.Company != "Royal Org Pte Ltd" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.mycareersfuture.gov.sg/job/7386f4f23f6bab00d906bb9e0f33e4b4" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Singapore" {
		t.Errorf("Location = %q, want Singapore", j.Location)
	}
	if !strings.Contains(j.Description, "Build") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-18", j.PostedAt)
	}
}

func TestMyCareersFutureOverseasLocation(t *testing.T) {
	page := `{"results":[
{"uuid":"abc","title":"Role","description":"<p>x</p>","postedCompany":{"name":"Acme"},"address":{"isOverseas":true,"overseasCountry":"Malaysia"},"metadata":{"newPostingDate":"2026-06-18"}}
],"total":1}`
	fake := (&routedHTTP{}).route("/v2/jobs", page)
	jobs, _ := NewMyCareersFuture(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 || jobs[0].Location != "Malaysia" {
		t.Fatalf("Location = %q, want Malaysia (overseas)", jobs[0].Location)
	}
}
