package sources

import (
	"context"
	"slices"
	"testing"
)

func TestRemotiveProvider(t *testing.T) {
	if got := NewRemotive(nil).Provider(); got != "remotive" {
		t.Errorf("Provider() = %q, want remotive", got)
	}
}

func TestRemotiveIsBoardlessAggregator(t *testing.T) {
	s := NewRemotive(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("remotive should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("remotive should implement the aggregator marker")
	}
}

func TestRemotiveRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["remotive"]; !ok {
		t.Error("All() should register provider remotive")
	}
	if !slices.Contains(FilterableProviders(), "remotive") {
		t.Error("FilterableProviders() should include remotive")
	}
}

func TestRemotiveBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/remotive.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/remotive.yml fails validation: %v", err)
	}
}

func TestRemotiveFetchSingleRequestAndMaps(t *testing.T) {
	feed := `{"jobs":[
{"id":2090991,"url":"https://remotive.com/remote-jobs/software-development/frontend-developer-2090991","title":"Frontend Developer","company_name":"Quinncia Inc","category":"Software Development","job_type":"full_time","publication_date":"2026-06-16T06:59:30","candidate_required_location":"Worldwide","description":"<p>Build UIs.</p>"},
{"id":0,"company_name":"NoID","title":"skip"}
]}`
	fake := (&routedHTTP{}).route("remote-jobs", feed)
	jobs, err := NewRemotive(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("made %d requests, want exactly 1 (rate-limited API, no pagination)", fake.calls)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (zero-id dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "2090991" || j.Company != "Quinncia Inc" || j.Title != "Frontend Developer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.Location != "Worldwide" || !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Location=%q Remote=%v WorkMode=%q", j.Location, j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed zoneless ISO timestamp")
	}
}
