package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// spainjobsFake serves the real fixtures (fetched 2026-10-27): the sitemap index trimmed to one
// job sitemap and a company one, that job sitemap trimmed to four postings and a company page, and
// three posting pages trimmed to what the adapter reads — one with a JobPosting, one without (an
// agency advert), one with a single-figure salary. A posting with no page fixture answers 404.
type spainjobsFake struct {
	pageErr error

	mu    sync.Mutex
	xmls  []string
	pages []string
}

var spainjobsPages = map[string]string{
	"ecbbc2e56c": "testdata/spainjobs/jsonld.html",
	"4a1a6e9e52": "testdata/spainjobs/nojsonld.html",
	"3714840599": "testdata/spainjobs/salary.html",
}

func (f *spainjobsFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.xmls = append(f.xmls, u)
	f.mu.Unlock()
	files := map[string]string{
		spainjobsSitemapIndex:                      "testdata/spainjobs/sitemap-index.xml",
		spainjobsBaseURL + "/sitemap/jobs-new.xml": "testdata/spainjobs/jobs-new.xml",
	}
	path, ok := files[u]
	if !ok {
		return fmt.Errorf("spainjobsFake: no sitemap %s", u)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *spainjobsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	for id, path := range spainjobsPages {
		if strings.HasSuffix(u, "--"+id) {
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return html.Parse(strings.NewReader(string(raw)))
		}
	}
	return nil, &StatusError{Method: "GET", Code: 404, URL: u}
}

func newTestSpainJobs(f *spainjobsFake) spainjobs {
	s := NewSpainJobs(f).(spainjobs)
	s.limiter = noWait{}
	return s
}

func TestSpainJobsHydratesNewPostings(t *testing.T) {
	f := &spainjobsFake{}
	jobs, err := newTestSpainJobs(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "3714840599" })
	if err != nil {
		t.Fatal(err)
	}
	// Only the job sitemap is read, never the company one.
	if len(f.xmls) != 2 || f.xmls[1] != spainjobsBaseURL+"/sitemap/jobs-new.xml" {
		t.Errorf("sitemaps read = %v", f.xmls)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The gone posting (404) is not yielded; the company page is not a posting.
	if len(jobs) != 3 {
		t.Fatalf("jobs = %+v, want three", jobs)
	}
	if r := byID["3714840599"]; !r.SeenRefresh || r.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less refresh", r)
	}
	j := byID["ecbbc2e56c"]
	if j.Title != "Business Development Manager (Architect)" || j.Company != "011h" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://apply.workable.com/j/6C44286182" {
		t.Errorf("url = %q, want the employer's apply link", j.URL)
	}
	if j.Location != "Barcelona, Catalonia" || len(j.Countries) != 1 || j.Countries[0] != "es" {
		t.Errorf("place = %q %v", j.Location, j.Countries)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 4 {
		t.Errorf("experience = %v", j.ExperienceYearsMin)
	}
	if !strings.Contains(j.Description, "011h designs and builds") {
		t.Errorf("description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-08" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	a := byID["4a1a6e9e52"]
	if a.Title != "Accounting Assistant" || a.Company != "IMAN Temporing" {
		t.Errorf("page-read advert title/company = %q / %q", a.Title, a.Company)
	}
	if !strings.Contains(a.Description, "Degree in Business Administration") {
		t.Errorf("page-read advert description = %q", a.Description)
	}
	if !strings.HasPrefix(a.URL, "https://empleo.imancorp.es/jobs/") {
		t.Errorf("page-read advert url = %q", a.URL)
	}
}

func TestSpainJobsReadsASingleFigureSalary(t *testing.T) {
	f := &spainjobsFake{}
	jobs, err := newTestSpainJobs(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID != "3714840599" {
			continue
		}
		if j.SalaryMin == nil || *j.SalaryMin != 32000 || *j.SalaryMax != 32000 || j.SalaryPeriod != "year" || j.SalaryCurrency != "EUR" {
			t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
		}
		if j.Company != "Marlex Recruiting" {
			t.Errorf("company = %q", j.Company)
		}
		return
	}
	t.Fatal("salary posting not yielded")
}

func TestSpainJobsGateSkipsCoveredEmployersBeforeTheirPage(t *testing.T) {
	f := &spainjobsFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c] = c == "iman temporing"
		}
		return out
	}}
	if _, err := newTestSpainJobs(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.pages {
		if strings.Contains(p, "iman-temporing") {
			t.Errorf("bought the page of a covered employer: %s", p)
		}
	}
	if len(f.pages) != 3 {
		t.Errorf("pages = %v, want the three uncovered postings", f.pages)
	}
}

func TestSpainJobsMostRecentlyModifiedFirstAndBounded(t *testing.T) {
	f := &spainjobsFake{}
	s := newTestSpainJobs(f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 1 || !strings.HasSuffix(f.pages[0], "--4a1a6e9e52") {
		t.Errorf("pages = %v, want only the most recently modified posting", f.pages)
	}
}

func TestSpainJobsNoPageAnsweringFailsTheCrawl(t *testing.T) {
	f := &spainjobsFake{pageErr: errors.New("boom")}
	if _, err := newTestSpainJobs(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error")
	}
}

func TestSpainJobsLeavesUnreadPostingsNearTheDeadline(t *testing.T) {
	f := &spainjobsFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestSpainJobs(f).FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("a run out of time is not a failed crawl: %v", err)
	}
	if len(jobs) != 0 || len(f.pages) != 0 {
		t.Errorf("jobs = %d, pages = %d; want nothing read inside the margin", len(jobs), len(f.pages))
	}
}
