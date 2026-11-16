package sources

import (
	"context"
	"strings"
	"testing"
)

const hrsmartTestHost = "gistworld.mua.hrdepartment.com"

func hrsmartTestList(id, next, total string) string {
	control := `<li class="paginateNext_div disabled"></li>`
	if next != "" {
		control = `<li class="paginateNext_div"><a href="` + next + `">Next</a></li>`
	}
	return `<table id="jobSearchResultsGrid_table"><tr><td><a href="/hrsmart/ats/Posting/view/` + id + `">Java Engineer</a></td></tr></table><div>Displaying 1 - 1 of ` + total + `</div>` + control
}

const hrsmartTestDetail = `<div id="job_details_ats_requisition_title">Senior Java Software Engineer</div><div id="job_details_hua_location_id">Basingstoke, GB</div><div id="job_details_ats_requisition_description"><p>Build Java services.</p></div>`

func TestHRsmartFetchesEveryPageUnderNativePostingIdentity(t *testing.T) {
	next := "/hrsmart/ats/JobSearch/viewAll/jobSearchPaginationExternal_page:2"
	fake := (&routedHTTP{}).
		route(hrsmartTestHost+next, hrsmartTestList("2", "", "2")).
		route(hrsmartTestHost+"/hrsmart/ats/JobSearch/viewAll", hrsmartTestList("3735", next, "2")).
		route(hrsmartTestHost+"/hrsmart/ats/Posting/view/", hrsmartTestDetail)
	jobs, err := NewHRsmart(fake).Fetch(context.Background(), CompanyEntry{Board: hrsmartTestHost, Company: "Gist Limited"})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("Fetch = %d jobs, %v", len(jobs), err)
	}
	j := jobs[0]
	if j.ExternalID != "3735" || j.URL != "https://"+hrsmartTestHost+"/hrsmart/ats/Posting/view/3735" || j.Company != "Gist Limited" || j.Title != "Senior Java Software Engineer" || j.Location != "Basingstoke, GB" || !strings.Contains(j.Description, "Java services") || j.Unreadable {
		t.Fatalf("native posting = %+v", j)
	}
}

func TestHRsmartRejectsAnIncompleteListing(t *testing.T) {
	fake := (&routedHTTP{}).route("/JobSearch/viewAll", hrsmartTestList("3735", "", "2"))
	if jobs, err := NewHRsmart(fake).Fetch(context.Background(), CompanyEntry{Board: hrsmartTestHost}); err == nil || len(jobs) != 0 {
		t.Fatalf("truncated listing = %v, %v", jobs, err)
	}
}

func TestHRsmartKeepsUnreadableIdentityWithoutClaimingContent(t *testing.T) {
	listing := strings.Replace(hrsmartTestList("3735", "", "2"), "</table>", `<tr><td><a href="/hrsmart/ats/Posting/view/2">Engineer</a></td></tr></table>`, 1)
	fake := (&routedHTTP{}).route("/JobSearch/viewAll", listing).route("/Posting/view/3735", "<h1>Sign in</h1>").route("/Posting/view/2", hrsmartTestDetail)
	jobs, err := NewHRsmart(fake).Fetch(context.Background(), CompanyEntry{Board: hrsmartTestHost, Company: "Gist Limited"})
	if err != nil || len(jobs) != 2 || !jobs[0].Unreadable || jobs[0].ExternalID != "3735" || jobs[0].Description != "" {
		t.Fatalf("unreadable posting = %+v, %v", jobs, err)
	}
}

func TestHRsmartFailsWhenNoListedPostingCanBeRead(t *testing.T) {
	fake := (&routedHTTP{}).route("/JobSearch/viewAll", hrsmartTestList("3735", "", "1")).route("/Posting/view/3735", "<h1>Sign in</h1>")
	if _, err := NewHRsmart(fake).Fetch(context.Background(), CompanyEntry{Board: hrsmartTestHost}); err == nil {
		t.Fatal("a blocked ATS must fail instead of reporting an empty successful crawl")
	}
}

func TestHRsmartCanHydrateOnlyTheLinkedPosting(t *testing.T) {
	listing := strings.Replace(hrsmartTestList("3735", "", "2"), "</table>", `<tr><td><a href="/hrsmart/ats/Posting/view/2">Engineer</a></td></tr></table>`, 1)
	fake := (&routedHTTP{}).route("/JobSearch/viewAll", listing).route("/Posting/view/3735", hrsmartTestDetail)
	s, ok := NewHRsmart(fake).(HydratingSource)
	if !ok {
		t.Fatal("link resolution must not hydrate every unrelated posting on this board")
	}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: hrsmartTestHost, Company: "Gist Limited"}, func(id string) bool { return id != "3735" })
	if err != nil || len(jobs) != 2 || jobs[0].Description == "" || !jobs[1].SeenRefresh || fake.calls != 2 {
		t.Fatalf("target hydration = %+v, requests=%d, %v", jobs, fake.calls, err)
	}
}

func TestHRsmartIsARegisteredNativeATS(t *testing.T) {
	s, ok := All(nil)["hrsmart"]
	if !ok || s.Provider() != "hrsmart" {
		t.Fatal("HRsmart must be a native source so its boards use normal ingest")
	}
	if ProviderKind(All(nil), "hrsmart") == "aggregator" {
		t.Fatal("HRsmart is the employer ATS, not a discovery aggregator")
	}
}
