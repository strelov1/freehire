package sources

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/unstop
var unstopFixtures embed.FS

// The fixtures are trimmed from live responses of the open-postings search feed fetched on
// 2026-11-22: three jobs (an office role with a salary, a work-from-home role with no place and
// no salary shown, a second office role) and three internships (a paid office one, an unpaid
// work-from-home one, a stipend one).
func unstopFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := unstopFixtures.ReadFile("testdata/unstop/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// unstopFake answers the search endpoint per feed and page, and records what was asked.
type unstopFake struct {
	mu    sync.Mutex
	pages map[string]string
	errs  map[string]error
	asked []string
}

func (f *unstopFake) GetJSON(_ context.Context, url string, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, url)
	if err, ok := f.errs[url]; ok {
		return err
	}
	body, ok := f.pages[url]
	if !ok {
		return &StatusError{Code: 500}
	}
	return json.Unmarshal([]byte(body), v)
}

func unstopURL(feed string, page int) string {
	return fmt.Sprintf("%s?opportunity=%s&oppstatus=open&per_page=%d&page=%d", unstopListURL, feed, unstopPerPage, page)
}

func newUnstopFake(t *testing.T) *unstopFake {
	t.Helper()
	return &unstopFake{
		pages: map[string]string{
			unstopURL("jobs", 1):        unstopFixture(t, "unstop_jobs.json"),
			unstopURL("internships", 1): unstopFixture(t, "unstop_internships.json"),
		},
		errs: map[string]error{},
	}
}

func unstopByID(t *testing.T, jobs []Job) map[string]Job {
	t.Helper()
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestUnstopFetchMapsJobsAndInternships(t *testing.T) {
	fake := newUnstopFake(t)
	jobs, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	by := unstopByID(t, jobs)

	office := by["1822505"]
	if office.URL != "https://unstop.com/jobs/sales-executive-naaz-ai-labs-1822505" || office.Company != "Naaz AI Labs" ||
		office.Title != "Sales Executive" {
		t.Errorf("identity wrong: %+v", office)
	}
	if office.Location != "Bangalore Urban, Karnataka, India" || office.WorkMode != "onsite" || office.Remote {
		t.Errorf("place wrong: location %q mode %q remote %v", office.Location, office.WorkMode, office.Remote)
	}
	if office.EmploymentType != "full_time" {
		t.Errorf("employment type %q, want full_time", office.EmploymentType)
	}
	if office.SalaryMin == nil || *office.SalaryMin != 300000 || office.SalaryCurrency != "INR" || office.SalaryPeriod != "year" {
		t.Errorf("salary wrong: %v %v %q %q", office.SalaryMin, office.SalaryMax, office.SalaryCurrency, office.SalaryPeriod)
	}
	if office.PostedAt == nil || office.PostedAt.UTC().Format("2006-01-02T15:04") != "2026-11-21T17:53" {
		t.Errorf("posted_at %v", office.PostedAt)
	}
	if office.ApplicationDeadline == nil || office.ApplicationDeadline.Format("2006-01-02") != "2026-12-05" {
		t.Errorf("deadline %v", office.ApplicationDeadline)
	}
	if office.IsTechHint {
		t.Error("unstop files nothing as IT; IsTechHint must stay unset")
	}
	if !strings.Contains(office.Description, "<") {
		t.Errorf("description should keep its markup: %q", office.Description)
	}

	remote := by["1822509"]
	if remote.WorkMode != "remote" || !remote.Remote || remote.Location != "" {
		t.Errorf("wfh posting: mode %q remote %v location %q", remote.WorkMode, remote.Remote, remote.Location)
	}
	if remote.SalaryMin != nil || remote.SalaryMax != nil || remote.SalaryCurrency != "" {
		t.Errorf("hidden salary must stay unset: %+v", remote)
	}
	if len(remote.FallbackCountries) != 1 || remote.FallbackCountries[0] != "in" {
		t.Errorf("fallback countries %v", remote.FallbackCountries)
	}

	paid := by["1822665"]
	if paid.EmploymentType != "internship" || paid.SalaryPeriod != "month" || paid.SalaryMin == nil || *paid.SalaryMin != 6000 ||
		paid.SalaryMax == nil || *paid.SalaryMax != 8000 {
		t.Errorf("internship stipend wrong: %+v", paid)
	}
	unpaid := by["1822504"]
	if unpaid.EmploymentType != "internship" || unpaid.SalaryMin != nil || unpaid.WorkMode != "remote" {
		t.Errorf("unpaid internship wrong: %+v", unpaid)
	}
}

func TestUnstopFetchPagesToLastPage(t *testing.T) {
	fake := newUnstopFake(t)
	page := func(id, last int) string {
		return fmt.Sprintf(`{"data":{"last_page":%d,"total":2,"data":[{"id":%d,"title":"T%d","details":"<p>Body</p>",
			"seo_url":"https://unstop.com/jobs/t-%d","status":"LIVE","visibility":"public","regn_open":1,"subtype":"jobs",
			"organisation":{"name":"Acme"},"locations":[]}]}}`, last, id, id, id)
	}
	fake.pages[unstopURL("jobs", 1)] = page(1, 2)
	fake.pages[unstopURL("jobs", 2)] = page(2, 2)
	jobs, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(unstopByID(t, jobs)); got != 2+3 {
		t.Errorf("got %d distinct postings, want 5 (two paged jobs + three internships)", got)
	}
}

func TestUnstopFetchFailsOnPageError(t *testing.T) {
	fake := newUnstopFake(t)
	fake.errs[unstopURL("internships", 1)] = errors.New("boom")
	if _, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a feed that cannot be read must fail the run, or the sweep closes it whole")
	}
}

func TestUnstopFetchAcceptsMeasuredListingCountGap(t *testing.T) {
	fake := newUnstopFake(t)
	rows := make([]unstopPosting, 640)
	for i := range rows {
		rows[i].ID = i + 1
	}
	var page unstopPage
	page.Data.Total = 662
	page.Data.LastPage = 1
	page.Data.Data = rows
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	fake.pages[unstopURL("internships", 1)] = string(body)
	if _, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
}

func TestUnstopFetchFailsOnShortCrawl(t *testing.T) {
	fake := newUnstopFake(t)
	// The feed reports 100 postings and its only page holds three.
	fake.pages[unstopURL("jobs", 1)] = strings.Replace(fake.pages[unstopURL("jobs", 1)], `"total": 3`, `"total": 100`, 1)
	if _, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl that read 3 of 100 reported postings must fail")
	}
}

func TestUnstopFetchFailsOnEmptyFeed(t *testing.T) {
	fake := newUnstopFake(t)
	fake.pages[unstopURL("jobs", 1)] = `{"data":{"last_page":0,"total":0,"data":[]}}`
	if _, err := NewUnstop(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an empty feed is a broken crawl, not a quiet source")
	}
}

func TestUnstopSkipsPostingsItCannotStandBehind(t *testing.T) {
	good := unstopPosting{
		ID: 1, Title: "T", Details: "<p>Body</p>", SEOURL: "https://unstop.com/jobs/t-1",
		Status: "LIVE", Visibility: "public", RegnOpen: 1,
	}
	good.Organisation.Name = "Acme"
	if _, ok := good.toJob(); !ok {
		t.Fatal("baseline posting must map")
	}
	for name, mutate := range map[string]func(*unstopPosting){
		"no body":         func(p *unstopPosting) { p.Details = "" },
		"no employer":     func(p *unstopPosting) { p.Organisation.Name = " " },
		"not live":        func(p *unstopPosting) { p.Status = "DRAFT" },
		"registration":    func(p *unstopPosting) { p.RegnOpen = 0 },
		"not public":      func(p *unstopPosting) { p.Visibility = "private" },
		"no posting link": func(p *unstopPosting) { p.SEOURL = "" },
	} {
		p := good
		mutate(&p)
		if _, ok := p.toJob(); ok {
			t.Errorf("%s: posting should be skipped", name)
		}
	}
}

func TestUnstopIsRegisteredAsFullCatalogAggregator(t *testing.T) {
	s := NewUnstop(nil)
	if _, ok := s.(fullCatalog); !ok {
		t.Error("unstop must be a fullCatalog source")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("unstop must be an aggregator")
	}
}
