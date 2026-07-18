package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestFunctionalWorksProvider(t *testing.T) {
	if got := NewFunctionalWorks(nil).Provider(); got != "functionalworks" {
		t.Errorf("Provider() = %q, want functionalworks", got)
	}
}

func TestFunctionalWorksIsBoardlessAggregator(t *testing.T) {
	s := NewFunctionalWorks(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("functionalworks should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("functionalworks should implement the aggregator marker")
	}
}

func TestFunctionalWorksRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["functionalworks"]; !ok {
		t.Error("All() should register provider functionalworks")
	}
	if !slices.Contains(FilterableProviders(), "functionalworks") {
		t.Error("FilterableProviders() should include functionalworks")
	}
}

func TestFunctionalWorksBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/functionalworks.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/functionalworks.yml fails validation: %v", err)
	}
}

func TestFunctionalWorksFetchMaps(t *testing.T) {
	resp := `{"data":{"jobs":[
{"title":"Senior Scala Engineer","slug":"remote-senior-scala-engineer-abc","company":{"name":"Acme"},"location":{"city":"Berlin","country":"Germany"},"remote":true,"firstPublished":"2026-07-15T21:18:56.516Z","descriptionHtml":"<p>Build &amp; ship with Scala.</p>","tags":[{"label":"Scala"},{"label":"Kafka"}]},
{"title":"On-site Clojure Dev","slug":"clojure-dev-2","company":{"name":"Globex"},"location":{"city":null,"country":"France"},"remote":false,"firstPublished":"2026-07-10T00:00:00Z","descriptionHtml":"<p>Onsite role.</p>","tags":[]},
{"title":"No slug","slug":"","company":{"name":"NoSlug"}},
{"title":"No company","slug":"has-slug-3","company":{"name":""}}
]}}`
	fake := (&routedHTTP{}).route("graphql", resp)

	jobs, err := NewFunctionalWorks(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (empty-slug and empty-company dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "remote-senior-scala-engineer-abc" || j.Company != "Acme" || j.Title != "Senior Scala Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://functional.works-hub.com/jobs/remote-senior-scala-engineer-abc" {
		t.Errorf("URL = %q, want the public job page from the slug", j.URL)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q, want \"Berlin, Germany\"", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}
	if len(j.Skills) == 0 {
		t.Errorf("Skills empty, want the tags canonicalized through skilltag")
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed firstPublished")
	}

	// The second job: city is null (country only) and a non-remote role leaves WorkMode empty.
	j2 := jobs[1]
	if j2.Location != "France" {
		t.Errorf("Location = %q, want \"France\" (city null)", j2.Location)
	}
	if j2.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty for a non-remote posting", j2.WorkMode)
	}
}
