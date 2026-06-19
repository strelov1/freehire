package sources

import (
	"context"
	"strings"
	"testing"
)

func TestOracleProvider(t *testing.T) {
	if got := NewOracle(nil).Provider(); got != "oracle" {
		t.Errorf("Provider() = %q, want %q", got, "oracle")
	}
}

// TestOracleFetchListsAndFetchesDetail covers the core path: page the requisition list,
// fetch each requisition's detail for the description, and map work-mode + posted date.
// Fixtures mirror the live Oracle Recruiting Cloud shapes (requisitions nest under
// items[0].requisitionList; the on-site code is ORA_ON_SITE; description is split across
// three external fields).
func TestOracleFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("findReqs", `{"hasMore": false, "items": [{
			"TotalJobsCount": 2,
			"requisitionList": [
				{"Id": "30607", "Title": "Backend Engineer", "PostedDate": "2026-06-16",
				 "PrimaryLocation": "Berlin, Germany", "PrimaryLocationCountry": "DE",
				 "WorkplaceTypeCode": "ORA_ON_SITE"},
				{"Id": "30610", "Title": "Data Engineer", "PostedDate": "2026-06-12",
				 "PrimaryLocation": "United States", "PrimaryLocationCountry": "US",
				 "WorkplaceTypeCode": "ORA_REMOTE"}
			]
		}]}`).
		route("30607", `{"items": [{
			"Id": "30607",
			"ExternalDescriptionStr": "<p>Build the backend.</p>",
			"ExternalResponsibilitiesStr": "<ul><li>Own services</li></ul>",
			"ExternalQualificationsStr": "<p>Go experience.</p>"
		}]}`).
		route("30610", `{"items": [{
			"Id": "30610",
			"ExternalDescriptionStr": "<p>Crunch data.</p>"
		}]}`)

	jobs, err := NewOracle(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "oracle",
		Board: "fa-test.fa.ocs.oraclecloud.com/CX_1",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["30607"]
	if !ok {
		t.Fatal("requisition 30607 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q", j.Location)
	}
	wantURL := "https://fa-test.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1/job/30607"
	if j.URL != wantURL {
		t.Errorf("URL = %q, want %q", j.URL, wantURL)
	}
	if j.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite for ORA_ON_SITE", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true, want false for an on-site role")
	}
	for _, want := range []string{"Build the backend.", "Own services", "Go experience."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %q", want, j.Description)
		}
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-06-16" {
		t.Errorf("PostedAt = %v, want 2026-06-16", j.PostedAt)
	}

	data := byID["30610"]
	if data.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote for ORA_REMOTE", data.WorkMode)
	}
	if !data.Remote {
		t.Error("Remote = false, want true for a remote role")
	}
}

// TestOracleOffsetIsInsideFinder guards the pagination fix: Oracle ignores a top-level
// &offset= query param (it only honors offset INSIDE the finder clause, alongside limit),
// so a top-level offset silently re-fetches the first page forever. The fake routes each
// page on ",offset=N" — the comma prefix matches only when offset sits inside the
// finder list — so a regression to a top-level "&offset=N" leaves page two unrouted and
// fails the run.
func TestOracleOffsetIsInsideFinder(t *testing.T) {
	page := func(ids ...string) string {
		var items []string
		for _, id := range ids {
			items = append(items, `{"Id": "`+id+`", "Title": "Role `+id+`", "PrimaryLocation": "Remote", "WorkplaceTypeCode": "ORA_REMOTE"}`)
		}
		return `{"items": [{"TotalJobsCount": 3, "requisitionList": [` + strings.Join(items, ",") + `]}]}`
	}
	fake := (&routedHTTP{}).
		route(",offset=0", page("1", "2")).
		route(",offset=2", page("3")).
		route("ById", `{"items": [{"ExternalDescriptionStr": "<p>desc</p>"}]}`)

	jobs, err := NewOracle(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "oracle", Board: "fa-test.fa.ocs.oraclecloud.com/CX_1",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (offset must be inside the finder so page two advances)", len(jobs))
	}
}

// TestOraclePaginatesByTotal verifies the lister keeps requesting pages until it has
// gathered TotalJobsCount requisitions, not just the first page.
func TestOraclePaginatesByTotal(t *testing.T) {
	page := func(ids ...string) string {
		var items []string
		for _, id := range ids {
			items = append(items, `{"Id": "`+id+`", "Title": "Role `+id+`", "PrimaryLocation": "Remote", "WorkplaceTypeCode": "ORA_REMOTE"}`)
		}
		return `{"TotalJobsCount": 3, "requisitionList": [` + strings.Join(items, ",") + `]}`
	}
	fake := &routedHTTP{}
	// Two list pages then detail stubs. offset=0 returns two, offset=2 returns one.
	fake.route("offset=0", `{"items": [`+page("1", "2")+`]}`).
		route("offset=2", `{"items": [`+page("3")+`]}`).
		route("ById", `{"items": [{"ExternalDescriptionStr": "<p>desc</p>"}]}`)

	jobs, err := NewOracle(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "oracle", Board: "fa-test.fa.ocs.oraclecloud.com/CX_1",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two pages", len(jobs))
	}
}
