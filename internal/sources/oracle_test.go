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
			"ExternalQualificationsStr": "<p>Go experience.</p>",
			"JobSchedule": "Full time"
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
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (from JobSchedule)", j.EmploymentType)
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

// TestOracleFallsBackToCorporateDescriptionWhenExternalIsBlank covers the Campus/Summer-
// Analyst template seen live on Goldman Sachs' board: the External* fields carry only a
// lone "<br>" with nothing else, so detail must fall back to Corporate/OrganizationDescriptionStr
// rather than yielding a description with no usable text for downstream skill extraction.
func TestOracleFallsBackToCorporateDescriptionWhenExternalIsBlank(t *testing.T) {
	fake := (&routedHTTP{}).
		route("findReqs", `{"items": [{
			"TotalJobsCount": 1,
			"requisitionList": [
				{"Id": "170159", "Title": "Summer Analyst", "PostedDate": "2026-08-15",
				 "PrimaryLocation": "Warsaw, Poland", "WorkplaceTypeCode": "ORA_ON_SITE"}
			]
		}]}`).
		route("170159", `{"items": [{
			"Id": "170159",
			"ExternalDescriptionStr": "<br>",
			"ExternalResponsibilitiesStr": "",
			"ExternalQualificationsStr": "",
			"CorporateDescriptionStr": "<p>About the program.</p>",
			"OrganizationDescriptionStr": "<p>About the division.</p>"
		}]}`)

	jobs, err := NewOracle(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Goldman Sachs", Provider: "oracle",
		Board: "hdpc.fa.us2.oraclecloud.com/LateralHiring",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	for _, want := range []string{"About the program.", "About the division."} {
		if !strings.Contains(jobs[0].Description, want) {
			t.Errorf("Description missing %q: %q", want, jobs[0].Description)
		}
	}
	if strings.Contains(jobs[0].Description, "<br>") {
		t.Errorf("Description = %q, should not fall back to the blank External* markup", jobs[0].Description)
	}
}

// TestOracleHasVisibleTextIgnoresEscapedTagLookalikes guards against stripping tags after
// unescaping: a field whose only content is an HTML-escaped tag-shaped string (e.g. someone
// wrote "<tag>" as literal text) must still count as visible text, not be swallowed by the
// tag stripper once unescaping turns it into something that looks like a real tag.
func TestOracleHasVisibleTextIgnoresEscapedTagLookalikes(t *testing.T) {
	if !oracleHasVisibleText("<p>&lt;tag&gt;</p>") {
		t.Error("oracleHasVisibleText(`<p>&lt;tag&gt;</p>`) = false, want true")
	}
}

// TestOracleDetailKeepsEscapedTagLookalikeOverFallback exercises the same case through the
// full detail path: an ExternalDescriptionStr whose only content is an HTML-escaped
// tag-shaped string must count as visible text, so detail keeps it and does not fall back
// to Corporate/OrganizationDescriptionStr.
func TestOracleDetailKeepsEscapedTagLookalikeOverFallback(t *testing.T) {
	fake := (&routedHTTP{}).
		route("findReqs", `{"items": [{
			"TotalJobsCount": 1,
			"requisitionList": [
				{"Id": "170200", "Title": "Support Engineer", "PostedDate": "2026-08-15",
				 "PrimaryLocation": "Warsaw, Poland", "WorkplaceTypeCode": "ORA_ON_SITE"}
			]
		}]}`).
		route("170200", `{"items": [{
			"Id": "170200",
			"ExternalDescriptionStr": "<p>&lt;tag&gt;</p>",
			"ExternalResponsibilitiesStr": "",
			"ExternalQualificationsStr": "",
			"CorporateDescriptionStr": "<p>Fallback text that should not be used.</p>",
			"OrganizationDescriptionStr": ""
		}]}`)

	jobs, err := NewOracle(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "oracle",
		Board: "fa-test.fa.ocs.oraclecloud.com/CX_1",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "&lt;tag&gt;") {
		t.Errorf("Description = %q, want it to keep the escaped literal <tag> text", jobs[0].Description)
	}
	if strings.Contains(jobs[0].Description, "Fallback text") {
		t.Errorf("Description = %q, should not have fallen back to CorporateDescriptionStr", jobs[0].Description)
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
