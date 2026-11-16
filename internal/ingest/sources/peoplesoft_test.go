package sources

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live PeopleSoft Candidate Gateway responses fetched on
// 2026-10-15: careers.dc.gov's search page and its first "load more" postback (the form tag,
// the session inputs and the first rows verbatim; the stated count rewritten to the rows kept),
// and two posting pages (DC's single-section posting, Sandia's multi-section one) with their
// header fields and description sections verbatim.

const (
	psBoard     = "careers.dc.gov/erecruit/EMPLOYEE/HRMS"
	psComponent = "https://careers.dc.gov/psc/erecruit/EMPLOYEE/HRMS/c/HRS_HRAM_FL.HRS_CG_SEARCH_FL.GBL"
	psSearch    = psComponent + "?Page=HRS_APP_SCHJOB_FL&Action=U&FOCUS=Applicant&SiteId=1"
)

func psPosting(id string) string {
	return psComponent + "?Page=HRS_APP_JBPST_FL&Action=U&FOCUS=Applicant&SiteId=1&JobOpeningId=" + id + "&PostingSeq=1"
}

func psFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/peoplesoft/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func psParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(psFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// psFake serves GETs by URL and answers every POST with the next queued page.
type psFake struct {
	pages map[string]string
	posts []string
	sent  []url.Values
	gets  []string
}

func (f *psFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.gets = append(f.gets, u)
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 500, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *psFake) PostFormWithHeaders(_ context.Context, u string, _ map[string]string, v url.Values) (*html.Node, error) {
	f.sent = append(f.sent, v)
	if len(f.posts) == 0 {
		return nil, errors.New("no postback queued")
	}
	body := f.posts[0]
	f.posts = f.posts[1:]
	return html.Parse(strings.NewReader(body))
}

func TestPeopleSoftBoardParsing(t *testing.T) {
	b, err := parsePeopleSoftBoard("Careers.DC.gov/erecruit/EMPLOYEE/HRMS")
	if err != nil {
		t.Fatal(err)
	}
	if got := b.searchURL(); got != psSearch {
		t.Errorf("search url = %s", got)
	}
	for _, bad := range []string{"careers.dc.gov", "careers.dc.gov/erecruit/EMPLOYEE", "careers.dc.gov/erecruit/EMPLOYEE/HRMS/x", "careers.dc.gov//EMPLOYEE/HRMS", "evil.com:8080/a/b/c"} {
		if _, err := parsePeopleSoftBoard(bad); err == nil {
			t.Errorf("board %q accepted", bad)
		}
	}
}

func TestPeopleSoftPageReadsRowsCountAndForm(t *testing.T) {
	page, err := parsePeopleSoftPage(psParse(t, "dc_search.html"))
	if err != nil {
		t.Fatal(err)
	}
	if page.total != 3 || len(page.rows) != 2 {
		t.Fatalf("total=%d rows=%d", page.total, len(page.rows))
	}
	want := peoplesoftRow{ID: "33704", Title: "Fiscal Monitor (SOR)", Location: "64 New York Ave. N.E.", Opened: "10/14/2026"}
	if page.rows[0] != want {
		t.Errorf("row 0 = %+v", page.rows[0])
	}
	if page.fields.Get("ICSID") == "" || page.fields.Get("ICStateNum") != "1" {
		t.Errorf("form state = %v", page.fields)
	}
	if page.action != psComponent {
		t.Errorf("action = %s", page.action)
	}
}

func TestPeopleSoftNonSearchPageIsAnError(t *testing.T) {
	root, _ := html.Parse(strings.NewReader("Site name is not valid. Check your url syntax and try again."))
	if _, err := parsePeopleSoftPage(root); err == nil {
		t.Fatal("a page without the win0 form read as a result list")
	}
}

func TestPeopleSoftCount(t *testing.T) {
	for in, want := range map[string]int{"68 rows": 68, "1 of 96": 96, "Ligne 1 sur 96": 96, "1,234 rows": 1234} {
		if got, ok := peoplesoftCount(in); !ok || got != want {
			t.Errorf("%q = %d, %v", in, got, ok)
		}
	}
	if _, ok := peoplesoftCount("No jobs found."); ok {
		t.Error("a count with no number read as one")
	}
}

func TestPeopleSoftPostingReadsSectionsInOrder(t *testing.T) {
	job, ok := parsePeopleSoftPosting(psParse(t, "sandia_posting.html"), peoplesoftRow{ID: "698789", Location: "Albuquerque, NM", Opened: "10/14/2026"})
	if !ok {
		t.Fatal("posting not read")
	}
	about := strings.Index(job.Description, "About Sandia")
	duty := strings.Index(job.Description, "What Your Job Will Be Like")
	pay := strings.Index(job.Description, "Salary Range")
	if about < 0 || duty < about || pay < duty {
		t.Errorf("sections out of order or missing: %d %d %d", about, duty, pay)
	}
	if !strings.HasPrefix(job.Title, "Cleared Senior/Principal Mechanical Engineer") || job.EmploymentType != "full_time" {
		t.Errorf("job = %q %q", job.Title, job.EmploymentType)
	}
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("posted = %v", job.PostedAt)
	}
	// A different opening's page — or the search page an expired opening lands on — is not it.
	if _, ok := parsePeopleSoftPosting(psParse(t, "sandia_posting.html"), peoplesoftRow{ID: "1"}); ok {
		t.Error("a page for another opening was read")
	}
	if _, ok := parsePeopleSoftPosting(psParse(t, "dc_search.html"), peoplesoftRow{ID: "33704"}); ok {
		t.Error("the search page was read as a posting")
	}
}

func TestPeopleSoftFetchPostsBackToTheStatedCount(t *testing.T) {
	f := &psFake{
		pages: map[string]string{
			psSearch:           psFixture(t, "dc_search.html"),
			psPosting("33704"): psFixture(t, "dc_posting.html"),
		},
		posts: []string{psFixture(t, "dc_postback.html")},
	}
	seen := map[string]bool{"33680": true}
	jobs, err := NewPeopleSoft(f).(peoplesoft).FetchNew(context.Background(), CompanyEntry{Provider: "peoplesoft", Board: psBoard, Company: "DC Government"}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].Get("ICAction") != peoplesoftLoadMore || f.sent[0].Get("ICStateNum") != "1" {
		t.Fatalf("postbacks = %v", f.sent)
	}
	// 33697 has no fixture page: its read fails and it is deferred, not stored body-less.
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	if j := jobs[0]; j.ExternalID != "33704" || j.URL != psPosting("33704") || j.Company != "DC Government" || !strings.Contains(j.Description, "JOB SUMMARY") {
		t.Errorf("hydrated = %+v", j)
	}
	if j := jobs[1]; j.ExternalID != "33680" || !j.SeenRefresh || j.Description != "" {
		t.Errorf("seen = %+v", j)
	}
	for _, g := range f.gets {
		if strings.Contains(g, "JobOpeningId=33680") {
			t.Error("a seen posting's page was requested")
		}
	}
}

func TestPeopleSoftFetchFailsWhenAPostbackAddsNothing(t *testing.T) {
	f := &psFake{
		pages: map[string]string{psSearch: psFixture(t, "dc_search.html")},
		posts: []string{psFixture(t, "dc_search.html")},
	}
	if _, err := NewPeopleSoft(f).Fetch(context.Background(), CompanyEntry{Board: psBoard, Company: "DC"}); err == nil {
		t.Fatal("a walk short of the stated count succeeded")
	}
}

func TestPeopleSoftFetchFailsWhenNoNewPostingIsRead(t *testing.T) {
	f := &psFake{
		pages: map[string]string{psSearch: psFixture(t, "dc_search.html")},
		posts: []string{psFixture(t, "dc_postback.html")},
	}
	if _, err := NewPeopleSoft(f).Fetch(context.Background(), CompanyEntry{Board: psBoard, Company: "DC"}); err == nil {
		t.Fatal("a crawl that read no posting page succeeded")
	}
}

func TestPeopleSoftIsFullBoardListing(t *testing.T) {
	if _, ok := NewPeopleSoft(nil).(fullBoardListing); !ok {
		t.Fatal("peoplesoft must claim fullBoardListing")
	}
	if _, ok := NewPeopleSoft(nil).(HydratingSource); !ok {
		t.Fatal("peoplesoft must hydrate only new postings")
	}
}

func TestPeopleSoftRepostedOpeningUsesSecondSequence(t *testing.T) {
	b, err := parsePeopleSoftBoard(psBoard)
	if err != nil {
		t.Fatal(err)
	}
	first := psPosting("33966")
	second := strings.TrimSuffix(first, "PostingSeq=1") + "PostingSeq=2"
	detail := `<span id="HRS_SCH_WRK2_HRS_JOB_OPENING_ID">33966</span><span id="HRS_SCH_WRK2_POSTING_TITLE">Engineer</span><div id="HRS_SCH_PSTDSC_DESCRLONG$0"><p>Build systems</p></div>`
	f := &psFake{pages: map[string]string{first: psFixture(t, "dc_search.html"), second: detail}}
	jobs, err := (peoplesoft{http: f}).hydrate(context.Background(), b, CompanyEntry{Company: "DC"}, []peoplesoftRow{{ID: "33966", Title: "Engineer"}}, func(string) bool { return false })
	if err != nil || len(jobs) != 1 || jobs[0].URL != second || !strings.Contains(jobs[0].Description, "Build systems") {
		t.Fatalf("%+v %v", jobs, err)
	}
}
