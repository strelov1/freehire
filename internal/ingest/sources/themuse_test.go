package sources

import (
	"context"
	"slices"
	"testing"
)

func TestTheMuseProvider(t *testing.T) {
	if got := NewTheMuse(nil).Provider(); got != "themuse" {
		t.Errorf("Provider() = %q, want themuse", got)
	}
}

func TestTheMuseIsBoardlessAggregator(t *testing.T) {
	s := NewTheMuse(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("themuse should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("themuse should implement the aggregator marker")
	}
}

func TestTheMuseRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["themuse"]; !ok {
		t.Error("All() should register provider themuse")
	}
	if !slices.Contains(FilterableProviders(), "themuse") {
		t.Error("FilterableProviders() should include themuse")
	}
}

func TestTheMuseFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"page_count":2,"results":[
{"id":123,"name":"Senior Specialist Electrical Engineer","contents":"<p>Build.</p>","publication_date":"2026-03-17T13:37:20Z","locations":[{"name":"Anaheim, CA"}],"levels":[{"name":"Senior Level","short_name":"senior"}],"refs":{"landing_page":"https://www.themuse.com/jobs/acme/senior-ee"},"company":{"name":"L3Harris Technologies"}},
{"id":0,"name":"NoID","company":{"name":"Ghost"}}
]}`
	page2 := `{"page_count":2,"results":[
{"id":124,"name":"Junior Analyst","contents":"<p>Analyze.</p>","publication_date":"2026-01-01T00:00:00Z","locations":[{"name":"Remote"}],"levels":[{"name":"Entry Level","short_name":"entry"}],"refs":{"landing_page":"https://www.themuse.com/jobs/acme/jr-analyst"},"company":{"name":"Acme"}}
]}`
	fake := (&routedHTTP{}).route("page=1", page1).route("page=2", page2)
	jobs, err := NewTheMuse(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (one per page)", fake.calls)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (zero-id dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "123" || j.Company != "L3Harris Technologies" || j.Title != "Senior Specialist Electrical Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.themuse.com/jobs/acme/senior-ee" {
		t.Errorf("URL = %q, want refs.landing_page", j.URL)
	}
	if j.Location != "Anaheim, CA" {
		t.Errorf("Location = %q, want locations[0].name", j.Location)
	}
	if j.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior", j.Seniority)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339 timestamp")
	}
	if jobs[1].Seniority != "junior" {
		t.Errorf("Seniority = %q, want junior (entry mapped)", jobs[1].Seniority)
	}
}

// The live API's own page_count/total wildly overstates what is actually walkable — a page
// far short of that count 400s with "Value `page` is too high" (verified live: page 100 fails
// while page 99 succeeds, despite page_count reporting in the tens of thousands). This
// reproduces that shape: page 2 fails even though page_count claims more pages exist, and the
// walk must end with the partial result rather than erroring the whole crawl.
func TestTheMuseFetchReturnsPartialWhenRealCeilingIsBelowReportedPageCount(t *testing.T) {
	page1 := `{"page_count":20356,"results":[
{"id":123,"name":"Role","contents":"<p>Build.</p>","publication_date":"2026-03-17T13:37:20Z","locations":[{"name":"Remote"}],"refs":{"landing_page":"https://www.themuse.com/jobs/acme/role"},"company":{"name":"Acme"}}
]}`
	fake := (&routedHTTP{}).route("page=1", page1)
	jobs, err := NewTheMuse(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (partial result kept from the successful first page)", len(jobs))
	}
}

func TestTheMuseFetchStopsOnFirstPageFailure(t *testing.T) {
	fake := &routedHTTP{}
	_, err := NewTheMuse(fake).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("Fetch: want error on first-page failure, got nil")
	}
}

func TestMuseSeniority(t *testing.T) {
	cases := map[string]string{
		"entry":   "junior",
		"mid":     "middle",
		"senior":  "senior",
		"unknown": "",
		"":        "",
	}
	for in, want := range cases {
		if got := museSeniority(in); got != want {
			t.Errorf("museSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}
