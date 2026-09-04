package sources

import (
	"context"
	"slices"
	"testing"
)

func TestRemotliProvider(t *testing.T) {
	if got := NewRemotli(nil).Provider(); got != "remotli" {
		t.Errorf("Provider() = %q, want remotli", got)
	}
}

func TestRemotliIsBoardlessAggregator(t *testing.T) {
	s := NewRemotli(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("remotli should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("remotli should implement the aggregator marker")
	}
}

func TestRemotliRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["remotli"]; !ok {
		t.Error("All() should register provider remotli")
	}
	if !slices.Contains(FilterableProviders(), "remotli") {
		t.Error("FilterableProviders() should include remotli")
	}
}

func TestRemotliFetchPaginatesFiltersAndMaps(t *testing.T) {
	page1 := `{"jobs":[
{"jobs":{"id":6381,"title":"Senior QA Engineer","company":"Storyblok","location":"Zürich, Switzerland","type":"full-time","description":"<p>Test.</p>","applyUrl":"https://www.storyblok.com/job?gh_jid=1","status":"active","publishedAt":"2026-08-10T08:47:26.000Z"}},
{"jobs":{"id":6300,"title":"Closed Role","company":"Ghost","location":"Remote","type":"full-time","status":"closed"}},
{"jobs":{"id":0,"title":"NoID","company":"NoID","status":"active"}}
],"pagination":{"page":1,"limit":20,"total":21,"totalPages":2}}`
	page2 := `{"jobs":[
{"jobs":{"id":6200,"title":"Remote Engineer","company":"Acme","location":"Remote-EMEA","type":"part-time","description":"<p>Second page.</p>","applyUrl":"https://acme.example/jobs/1","status":"active","publishedAt":"2026-08-05T00:00:00.000Z"}}
],"pagination":{"page":2,"limit":20,"total":21,"totalPages":2}}`
	fake := (&routedHTTP{}).route("page=1", page1).route("page=2", page2)
	jobs, err := NewRemotli(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (closed status and zero-id dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "6381" || j.Company != "Storyblok" || j.Title != "Senior QA Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.storyblok.com/job?gh_jid=1" {
		t.Errorf("URL = %q, want applyUrl", j.URL)
	}
	if j.Remote {
		t.Error("Remote should be false for an onsite Swiss city location")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339 timestamp")
	}
	j2 := jobs[1]
	if !j2.Remote {
		t.Error("Remote should be true for a Remote-EMEA location")
	}
	if j2.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time", j2.EmploymentType)
	}
}

func TestRemotliFetchStopsOnFirstPageFailure(t *testing.T) {
	fake := &routedHTTP{}
	_, err := NewRemotli(fake).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("Fetch: want error on first-page failure, got nil")
	}
}

func TestRemotliFetchReturnsPartialOnLaterPageFailure(t *testing.T) {
	page1 := `{"jobs":[
{"jobs":{"id":1,"title":"Role","company":"Acme","location":"Remote","type":"full-time","status":"active","applyUrl":"https://acme.example/1","publishedAt":"2026-08-10T08:47:26.000Z"}}
],"pagination":{"page":1,"limit":20,"total":21,"totalPages":2}}`
	fake := (&routedHTTP{}).route("page=1", page1)
	jobs, err := NewRemotli(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (partial result kept from the successful first page)", len(jobs))
	}
}
