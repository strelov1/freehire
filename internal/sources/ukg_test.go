package sources

import (
	"context"
	"strings"
	"testing"
)

func TestSplitUKGBoard(t *testing.T) {
	cases := []struct {
		board               string
		wantOK              bool
		host, tenant, guid_ string
	}{
		{"recruiting.ultipro.com/van5000vcscu/a46cbdaa-ca2c", true, "recruiting.ultipro.com", "van5000vcscu", "a46cbdaa-ca2c"},
		{"recruiting.ultipro.ca/bur5000burn/3fa0ebc6", true, "recruiting.ultipro.ca", "bur5000burn", "3fa0ebc6"},
		{"tenant/guid", false, "", "", ""},            // missing host
		{"host/tenant/guid/extra", false, "", "", ""}, // too many parts
		{"host//guid", false, "", "", ""},             // empty tenant
		{"", false, "", "", ""},                       // empty
	}
	for _, tc := range cases {
		host, tenant, guid, ok := splitUKGBoard(tc.board)
		if ok != tc.wantOK || host != tc.host || tenant != tc.tenant || guid != tc.guid_ {
			t.Errorf("splitUKGBoard(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				tc.board, host, tenant, guid, ok, tc.host, tc.tenant, tc.guid_, tc.wantOK)
		}
	}
}

func TestUKGLocation(t *testing.T) {
	structured := ukgOpportunity{Locations: []ukgLocation{{
		LocalizedName: "Vancity Centre",
		Address: &struct {
			City  string `json:"City"`
			State *struct {
				Name string `json:"Name"`
			} `json:"State"`
			Country *struct {
				Name string `json:"Name"`
			} `json:"Country"`
		}{
			City: "Vancouver",
			State: &struct {
				Name string `json:"Name"`
			}{Name: "British Columbia"},
			Country: &struct {
				Name string `json:"Name"`
			}{Name: "Canada"},
		},
	}}}
	if got := structured.location(); got != "Vancouver, British Columbia, Canada" {
		t.Errorf("structured location = %q", got)
	}

	// No structured address → fall back to the localized label.
	labelOnly := ukgOpportunity{Locations: []ukgLocation{{LocalizedName: "Remote - US"}}}
	if got := labelOnly.location(); got != "Remote - US" {
		t.Errorf("label-only location = %q", got)
	}

	// No locations → empty.
	if got := (ukgOpportunity{}).location(); got != "" {
		t.Errorf("no-location = %q, want empty", got)
	}
}

// ukgDetailHTML builds an OpportunityDetail page that bootstraps the opportunity model as
// the JSON argument of a CandidateOpportunityDetail(...) call, the shape the adapter parses.
func ukgDetailHTML(description string) string {
	return `<html><head></head><body>` +
		`<script>var opportunity = new US.Opportunity.CandidateOpportunityDetail(` +
		`{"Id":"x","Title":"t","FullTime":true,"Description":"` + description + `"});</script>` +
		`</body></html>`
}

func TestUKGFetch(t *testing.T) {
	listing := `{"totalCount":2,"opportunities":[
		{"Id":"id-1","Title":"Backend Engineer","PostedDate":"2026-06-19T22:12:06.752Z","BriefDescription":"brief one",
		 "Locations":[{"LocalizedName":"Vancity Centre","Address":{"City":"Vancouver","State":{"Name":"British Columbia"},"Country":{"Name":"Canada"}}}]},
		{"Id":"id-2","Title":"Data Analyst","PostedDate":"2026-06-18T00:00:00Z","BriefDescription":"brief two",
		 "Locations":[{"LocalizedName":"Remote - US"}]}
	]}`
	http := (&routedHTTP{}).
		route("/JobBoardView/LoadSearchResults", listing).
		route("/OpportunityDetail", ukgDetailHTML("<p>Full body</p>"))

	jobs, err := ukg{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "recruiting.ultipro.com/van5000vcscu/the-guid"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "id-1" || j.Title != "Backend Engineer" {
		t.Errorf("job0 id/title = %q/%q", j.ExternalID, j.Title)
	}
	if want := "recruiting.ultipro.com/van5000vcscu/JobBoard/the-guid/OpportunityDetail"; !strings.Contains(j.URL, want) || !strings.Contains(j.URL, "opportunityId=id-1") {
		t.Errorf("job0 URL = %q", j.URL)
	}
	if j.Location != "Vancouver, British Columbia, Canada" {
		t.Errorf("job0 location = %q", j.Location)
	}
	// The detail page upgrades the brief body to the full description.
	if !strings.Contains(j.Description, "Full body") {
		t.Errorf("job0 description = %q, want full body", j.Description)
	}
	// The detail page's FullTime flag maps to the structured employment type.
	if j.EmploymentType != "full_time" {
		t.Errorf("job0 EmploymentType = %q, want full_time (from FullTime:true)", j.EmploymentType)
	}
	if j.PostedAt == nil {
		t.Errorf("job0 PostedAt is nil")
	}
	// Second job's label-only location and remote inference.
	if jobs[1].Location != "Remote - US" || !jobs[1].Remote {
		t.Errorf("job1 location/remote = %q/%v", jobs[1].Location, jobs[1].Remote)
	}
}

// When the detail page is unreachable, the listing's brief description is retained rather
// than dropping the job.
func TestUKGFetchBriefFallback(t *testing.T) {
	listing := `{"totalCount":1,"opportunities":[
		{"Id":"id-1","Title":"Backend Engineer","BriefDescription":"the brief body","Locations":[]}
	]}`
	http := (&routedHTTP{}).route("/JobBoardView/LoadSearchResults", listing) // no detail route → GetHTML errors

	jobs, err := ukg{http: http}.Fetch(context.Background(),
		CompanyEntry{Board: "recruiting.ultipro.com/t/g"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !strings.Contains(jobs[0].Description, "the brief body") {
		t.Fatalf("want 1 job with brief body, got %+v", jobs)
	}
}

func TestUKGFetchBadBoard(t *testing.T) {
	if _, err := (ukg{http: &routedHTTP{}}).Fetch(context.Background(), CompanyEntry{Board: "bad"}); err == nil {
		t.Errorf("want error for malformed board")
	}
}
