package sources

import (
	"context"
	"strings"
	"testing"
)

func TestAshbyGraphQLProvider(t *testing.T) {
	if got := NewAshbyGraphQL(nil).Provider(); got != "ashbygraphql" {
		t.Errorf("Provider() = %q, want %q", got, "ashbygraphql")
	}
}

func TestAshbyGraphQLFetch(t *testing.T) {
	// The embed GraphQL endpoint lists brief postings (no description), then a per-posting
	// detail query carries descriptionHtml. Routes are keyed by the ?op= operation.
	fake := (&routedHTTP{}).
		route("op=ApiJobBoardWithTeams", `{"data":{"jobBoard":{"jobPostings":[
			{"id":"961b9946","title":"Blockchain Security Analyst","locationName":"Remote - US","employmentType":"FullTime"}
		]}}}`).
		route("op=ApiJobPosting", `{"data":{"jobPosting":{
			"id":"961b9946","title":"Blockchain Security Analyst","locationName":"Remote - US",
			"descriptionHtml":"<p>About Chainlink</p>  "
		}}}`)

	jobs, err := NewAshbyGraphQL(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Chainlink Labs", Provider: "ashbygraphql", Board: "chainlink-labs",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.routes[0].match, "ApiJobBoardWithTeams") {
		t.Fatal("test wiring: list route missing")
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "961b9946" {
		t.Errorf("ExternalID = %q, want the posting id", j.ExternalID)
	}
	if j.URL != "https://jobs.ashbyhq.com/chainlink-labs/961b9946" {
		t.Errorf("URL = %q, want jobs.ashbyhq.com/<board>/<id>", j.URL)
	}
	if j.Title != "Blockchain Security Analyst" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Chainlink Labs" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Remote - US" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "About Chainlink") {
		t.Errorf("Description = %q, want detail descriptionHtml", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from a remote location")
	}
}

func TestAshbyGraphQLFetchSkipsFailedDetail(t *testing.T) {
	// Two briefs but only one has a working detail route; the other's detail errors and
	// that posting is dropped, never aborting the board.
	fake := (&routedHTTP{}).
		route("op=ApiJobBoardWithTeams", `{"data":{"jobBoard":{"jobPostings":[
			{"id":"ok-1","title":"Engineer","locationName":"Remote"},
			{"id":"missing-2","title":"Designer","locationName":"Remote"}
		]}}}`)
	// No op=ApiJobPosting route → every detail call errors → both dropped.
	jobs, err := NewAshbyGraphQL(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Chainlink Labs", Provider: "ashbygraphql", Board: "chainlink-labs",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort on a failed detail: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 (all details failed, skipped not fatal)", len(jobs))
	}
}
