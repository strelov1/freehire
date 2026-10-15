package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live builtin.com responses fetched on 2026-10-15: four listing
// cards verbatim (a hybrid, a six-location, a remote and a "Remote or Hybrid" posting), the
// "No job results" block a page past the end serves, and each posting page's ld+json block
// (jobBenefits cut to three entries, the employer logo dropped) and heading.

func builtinFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/builtin/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func builtinParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(builtinFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// builtinFake serves fixed pages by URL; an unknown URL is a 404.
type builtinFake struct {
	mu    sync.Mutex
	pages map[string]string
	errs  map[string]error
	got   []string
}

func (f *builtinFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	body, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *builtinFake) requested(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.got, url)
}

const (
	builtinXero   = "https://builtin.com/job/senior-product-marketing-manager-payments/11691620"
	builtinPwC    = "https://builtin.com/job/strategy-operations-strategy-consultant-senior-associate/11691513"
	builtinWasabi = "https://builtin.com/job/architect-object-storage-database-systems/11687409"
	builtinGM     = "https://builtin.com/job/staff-software-engineer-ai-enablement/11691412"
)

func TestBuiltInCardsReadIdentityEmployerAndWorkMode(t *testing.T) {
	cards := parseBuiltInCards(builtinParse(t, "list.html"))
	want := []builtinCard{
		{ID: "11691620", URL: builtinXero, Title: "Senior Product Marketing Manager - Payments", Company: "Xero", WorkMode: "hybrid"},
		{ID: "11691513", URL: builtinPwC, Title: "Strategy& - Operations Strategy Consultant, Senior Associate", Company: "PwC", WorkMode: "hybrid"},
		{ID: "11687409", URL: builtinWasabi, Title: "Architect – Object Storage & Database Systems", Company: "Wasabi Technologies", WorkMode: "remote"},
		// "Remote or Hybrid" states two arrangements, so none is taken.
		{ID: "11691412", URL: builtinGM, Title: "Staff Software Engineer- AI Enablement", Company: "General Motors", WorkMode: ""},
	}
	if !slices.Equal(cards, want) {
		t.Fatalf("cards =\n%+v\nwant\n%+v", cards, want)
	}
	if got := parseBuiltInCards(builtinParse(t, "list_past_end.html")); len(got) != 0 {
		t.Errorf("past-end page yielded cards: %+v", got)
	}
}

func TestBuiltInPostingRemote(t *testing.T) {
	base := builtinCard{ID: "11687409", URL: builtinWasabi, Title: "card title", Company: "Wasabi Technologies", WorkMode: "remote"}.job()
	job, ok := parseBuiltInPosting(builtinParse(t, "detail_remote.html"), base)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.ExternalID != "11687409" || job.URL != builtinWasabi {
		t.Errorf("identity = %q %q", job.ExternalID, job.URL)
	}
	if job.Title != "Architect – Object Storage & Database Systems" || job.Company != "Wasabi Technologies" {
		t.Errorf("title/company = %q / %q", job.Title, job.Company)
	}
	if job.WorkMode != "remote" || !job.Remote {
		t.Errorf("work mode = %q remote = %v", job.WorkMode, job.Remote)
	}
	if job.Location != "USA" || !slices.Equal(job.Countries, []string{"us"}) {
		t.Errorf("location = %q countries = %v", job.Location, job.Countries)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 200000 || job.SalaryMax == nil || *job.SalaryMax != 250000 ||
		job.SalaryCurrency != "USD" || job.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-10-14" {
		t.Errorf("posted = %v", job.PostedAt)
	}
	if job.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", job.EmploymentType)
	}
	if !strings.Contains(job.Description, "At Wasabi, we’re a proven collection of pioneers") {
		t.Errorf("description = %.200q", job.Description)
	}
}

func TestBuiltInPostingTelecommuteMarksRemoteWhenTheCardDidNot(t *testing.T) {
	base := builtinCard{ID: "11687409", URL: builtinWasabi, Title: "t", Company: "c"}.job()
	job, ok := parseBuiltInPosting(builtinParse(t, "detail_remote.html"), base)
	if !ok || job.WorkMode != "remote" || !job.Remote {
		t.Errorf("ok=%v work mode = %q remote = %v", ok, job.WorkMode, job.Remote)
	}
}

func TestBuiltInPostingSeveralLocations(t *testing.T) {
	base := builtinCard{ID: "11691513", URL: builtinPwC, Title: "t", Company: "PwC", WorkMode: "hybrid"}.job()
	job, ok := parseBuiltInPosting(builtinParse(t, "detail_multi_location.html"), base)
	if !ok {
		t.Fatal("posting not parsed")
	}
	want := "Dallas, Texas, USA; Chicago, Illinois, USA; New York, New York, USA; Boston, Massachusetts, USA; " +
		"San Francisco, California, USA; Washington, District of Columbia, USA"
	if job.Location != want || !slices.Equal(job.Countries, []string{"us"}) {
		t.Errorf("location = %q countries = %v", job.Location, job.Countries)
	}
	// The JobPosting's title carries a doubled space; it is collapsed.
	if job.Title != "Strategy& - Operations Strategy Consultant, Senior Associate" {
		t.Errorf("title = %q", job.Title)
	}
	if job.WorkMode != "hybrid" || job.Remote {
		t.Errorf("work mode = %q remote = %v", job.WorkMode, job.Remote)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 77000 || job.SalaryMax == nil || *job.SalaryMax != 202000 {
		t.Errorf("salary = %v-%v", job.SalaryMin, job.SalaryMax)
	}
}

func TestBuiltInPostingDropsAnAnnualRangeFiledAsHourly(t *testing.T) {
	base := builtinCard{ID: "10947487", URL: "https://builtin.com/job/x/10947487", Title: "t", Company: "PwC"}.job()
	job, ok := parseBuiltInPosting(builtinParse(t, "detail_hourly_error.html"), base)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.SalaryMin != nil || job.SalaryMax != nil || job.SalaryCurrency != "" || job.SalaryPeriod != "" {
		t.Errorf("salary = %v-%v %s/%s, want none for 29250-48000 USD/HOUR", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.EmploymentType != "internship" {
		t.Errorf("employment type = %q", job.EmploymentType)
	}
}

// builtinSite serves the fixture listing as the dev-engineering window and as the data-analytics
// window (so the same postings arrive under two categories), and every other window as empty.
func builtinSite(t *testing.T) *builtinFake {
	t.Helper()
	list, end := builtinFixture(t, "list.html"), builtinFixture(t, "list_past_end.html")
	pages := map[string]string{}
	for _, category := range builtinCategories {
		page := builtinPageURL(category)
		pages[page(1)] = end
		pages[page(2)] = end
	}
	pages[builtinPageURL("dev-engineering")(1)] = list
	pages[builtinPageURL("data-analytics")(1)] = list
	pages[builtinWasabi] = builtinFixture(t, "detail_remote.html")
	return &builtinFake{pages: pages}
}

func TestBuiltInPageURLKeepsTheFilterAheadOfThePage(t *testing.T) {
	page := builtinPageURL("engineering")
	if got := page(1); got != "https://builtin.com/jobs/engineering?allLocations=true&daysSinceUpdated=1" {
		t.Errorf("page 1 = %s", got)
	}
	if got := page(3); got != "https://builtin.com/jobs/engineering?allLocations=true&daysSinceUpdated=1&page=3" {
		t.Errorf("page 3 = %s", got)
	}
}

func TestBuiltInGatedCrawlBuysOnlyTheBodiesItKeeps(t *testing.T) {
	site := builtinSite(t)
	s := builtin{http: site, maxNewPerRun: 10}
	gate := DetailGate{
		Covered:  func(names []string) map[string]bool { return map[string]bool{"PwC": true} },
		Rejected: func(j Job) bool { return strings.Contains(j.Title, "Marketing") },
	}
	seen := func(id string) bool { return id == "11691412" }
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{}, seen, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("jobs = %d, want each of the 4 postings once although two categories list them: %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["11691412"].SeenRefresh {
		t.Error("a seen posting is not a liveness refresh")
	}
	for _, id := range []string{"11691513", "11691620"} { // covered employer, rejected title
		if j := byID[id]; j.Description != "" || j.SeenRefresh || j.Company == "" {
			t.Errorf("%s should be yielded body-less for the pipeline to discard: %+v", id, j)
		}
	}
	if byID["11687409"].Description == "" {
		t.Error("the kept posting was not hydrated")
	}
	for _, u := range []string{builtinPwC, builtinXero, builtinGM} {
		if site.requested(u) {
			t.Errorf("bought a body nobody keeps: %s", u)
		}
	}
}

func TestBuiltInHydrationBudgetDefersTheRest(t *testing.T) {
	site := builtinSite(t)
	s := builtin{http: site, maxNewPerRun: 1}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// Cards come in listing order, so the budget's one page goes to the first card (Xero),
	// whose page the fake does not serve: a gone posting, deferred without failing the run.
	if len(jobs) != 0 {
		t.Errorf("jobs = %+v, want none: one budgeted page (gone) and three deferred", jobs)
	}
	if !site.requested(builtinXero) || site.requested(builtinWasabi) {
		t.Errorf("requests = %v", site.got)
	}
}

func TestBuiltInRefusedPostingPagesFailTheRun(t *testing.T) {
	site := builtinSite(t)
	site.errs = map[string]error{builtinWasabi: &StatusError{Method: "GET", Code: 429, URL: builtinWasabi}}
	s := builtin{http: site, maxNewPerRun: 10}
	gate := DetailGate{
		Covered:  func([]string) map[string]bool { return nil },
		Rejected: func(j Job) bool { return j.ExternalID != "11687409" },
	}
	if _, err := s.FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate); err == nil {
		t.Error("a run that needed one posting page and was refused it reported success")
	}
}

func TestBuiltInFirstListingPageFailingFailsTheRun(t *testing.T) {
	site := builtinSite(t)
	first := builtinPageURL("cybersecurity-it")(1)
	site.errs = map[string]error{first: &StatusError{Method: "GET", Code: 429, URL: first}}
	if _, err := (builtin{http: site, maxNewPerRun: 10}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a category whose first page failed did not fail the run")
	}
}

func TestBuiltInEmptyListingFailsTheRun(t *testing.T) {
	end := builtinFixture(t, "list_past_end.html")
	pages := map[string]string{}
	for _, category := range builtinCategories {
		pages[builtinPageURL(category)(1)] = end
	}
	if _, err := (builtin{http: &builtinFake{pages: pages}}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a run that listed no card at all reported success")
	}
}

func TestBuiltInRegistration(t *testing.T) {
	s := NewBuiltIn(&builtinFake{})
	if s.Provider() != "builtin" {
		t.Errorf("provider = %q", s.Provider())
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("builtin is not CoverageGated")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("builtin is not an aggregator")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("builtin is not boardless")
	}
	if g, ok := s.(sweepGrace); !ok || g.sweepGrace() != 30*24*time.Hour {
		t.Error("builtin does not declare its 30-day sweep grace")
	}
	for _, marker := range []bool{isFullCatalog(s), isFullBoardListing(s)} {
		if marker {
			t.Error("a windowed crawl must not claim fullCatalog or fullBoardListing")
		}
	}
}

func isFullCatalog(s Source) bool      { _, ok := s.(fullCatalog); return ok }
func isFullBoardListing(s Source) bool { _, ok := s.(fullBoardListing); return ok }

// The catalogue is broader than IT, so every top-level family is walked, and the technical ones
// come first: the hydration budget is spent in this order.
func TestBuiltInWalksEveryFamilyTechnicalFirst(t *testing.T) {
	if builtinCategories[0] != "dev-engineering" || !slices.Contains(builtinCategories, "sales") ||
		!slices.Contains(builtinCategories, "marketing") {
		t.Errorf("categories = %v", builtinCategories)
	}
}
