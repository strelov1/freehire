package sources

import (
	"context"
	"slices"
	"testing"
)

func TestHimalayasProvider(t *testing.T) {
	if got := NewHimalayas(nil).Provider(); got != "himalayas" {
		t.Errorf("Provider() = %q, want himalayas", got)
	}
}

func TestHimalayasIsBoardlessAggregator(t *testing.T) {
	s := NewHimalayas(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("himalayas should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("himalayas should implement the aggregator marker")
	}
}

func TestHimalayasRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["himalayas"]; !ok {
		t.Error("All() should register provider himalayas")
	}
	if !slices.Contains(FilterableProviders(), "himalayas") {
		t.Error("FilterableProviders() should include himalayas")
	}
}

func TestHimalayasBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/himalayas.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/himalayas.yml fails validation: %v", err)
	}
}

func TestHimalayasFetchPaginatesAndMaps(t *testing.T) {
	// totalCount (150) exceeds one page (limit 100), so the adapter must fetch a second
	// offset page and stop once offset passes totalCount.
	page1 := `{"totalCount":150,"jobs":[
{"title":"Web Engineer","companyName":"KraftPixel","applicationLink":"https://himalayas.app/companies/kraftpixel/jobs/web-engineer","guid":"https://himalayas.app/companies/kraftpixel/jobs/web-engineer","locationRestrictions":["United States","Canada"],"description":"<p>Build web.</p>","pubDate":1747699200},
{"title":"NoGUID drop","companyName":"Ghost","guid":""}
]}`
	page2 := `{"totalCount":150,"jobs":[
{"title":"Data Analyst","companyName":"Peroptyx","applicationLink":"https://himalayas.app/companies/peroptyx/jobs/data-analyst","guid":"https://himalayas.app/companies/peroptyx/jobs/data-analyst","locationRestrictions":["Ireland"],"description":"<p>Analyze.</p>","pubDate":1781725000}
]}`
	fake := (&routedHTTP{}).route("offset=100", page2).route("offset=0", page1)
	jobs, err := NewHimalayas(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per offset page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (empty-guid dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "https://himalayas.app/companies/kraftpixel/jobs/web-engineer" {
		t.Errorf("ExternalID = %q, want the guid", j.ExternalID)
	}
	if j.Company != "KraftPixel" || j.Title != "Web Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://himalayas.app/companies/kraftpixel/jobs/web-engineer" {
		t.Errorf("URL = %q, want applicationLink", j.URL)
	}
	if j.Location != "United States, Canada" {
		t.Errorf("Location = %q, want joined locationRestrictions", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote (himalayas is remote-only)", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch seconds")
	}
}
