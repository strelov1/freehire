package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// shineFake serves the real fixtures (fetched 2026-10-25): the sitemap index trimmed to one job
// sitemap and a blog sitemap, that job sitemap trimmed to five postings, and one posting page
// trimmed to its JobPosting. A posting with no page fixture answers 404, as a gone posting does.
type shineFake struct {
	pageErr error

	mu    sync.Mutex
	xmls  []string
	pages []string
}

const shinePageID = "19513074"

func (f *shineFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.xmls = append(f.xmls, u)
	f.mu.Unlock()
	files := map[string]string{
		shineSitemapIndex: "testdata/shine/sitemap-index.xml",
		shineBaseURL + "/sitemaps/all_jd_22_sitemap.xml": "testdata/shine/all_jd_22.xml",
	}
	path, ok := files[u]
	if !ok {
		return fmt.Errorf("shineFake: no sitemap %s", u)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *shineFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if !strings.HasSuffix(u, "/"+shinePageID) {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/shine/page.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestShine(f *shineFake) shine {
	s := NewShine(f).(shine)
	s.limiter = noWait{}
	return s
}

func TestShineHydratesNewPostingsNewestFirst(t *testing.T) {
	f := &shineFake{}
	jobs, err := newTestShine(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "19825107" })
	if err != nil {
		t.Fatal(err)
	}
	// Only the job sitemap is read, never the blog one.
	if !slices.Equal(f.xmls, []string{shineSitemapIndex, shineBaseURL + "/sitemaps/all_jd_22_sitemap.xml"}) {
		t.Errorf("sitemaps read = %v", f.xmls)
	}
	// Four unseen postings are read; three answer 404 (gone) and are not stored.
	if len(f.pages) != 4 {
		t.Errorf("read %d pages, want 4", len(f.pages))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 2 || !byID["19825107"].SeenRefresh {
		t.Fatalf("jobs = %+v, want the stored posting refreshed and one read", jobs)
	}
	j := byID[shinePageID]
	if j.Title != "Java Developer - Solution Design & Implementation" || j.Company != "CrossRoad Solution" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Bangalore, Karnataka" || !slices.Equal(j.FallbackCountries, []string{"in"}) {
		t.Errorf("location %q, fallback %v", j.Location, j.FallbackCountries)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 4 || j.EmploymentType != "full_time" {
		t.Errorf("experience %v, employment %q", j.ExperienceYearsMin, j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02") != "2026-09-02" {
		t.Errorf("posted %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Java Developer") {
		t.Errorf("description = %.120q", j.Description)
	}
	if j.SalaryMin != nil {
		t.Errorf("a posting with no baseSalary got one: %v", *j.SalaryMin)
	}
}

func TestShineGateJudgesThePostingByItsURL(t *testing.T) {
	f := &shineFake{}
	var asked []string
	gate := DetailGate{
		Covered: func(cs []string) map[string]bool {
			asked = cs
			return map[string]bool{"md shakeel ahamed": true}
		},
		Rejected: func(j Job) bool { return j.Title == "insurance sales manager" },
	}
	jobs, err := newTestShine(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(asked, "crossroad solution") {
		t.Errorf("gate was asked about %v, want the employers the URLs spell", asked)
	}
	for _, u := range f.pages {
		if strings.HasSuffix(u, "/19825107") || strings.HasSuffix(u, "/19825122") {
			t.Errorf("a condemned posting's page was read: %s", u)
		}
	}
	if len(f.pages) != 3 || len(jobs) != 3 {
		t.Errorf("read %d pages and yielded %d jobs, want 3 and 3 (2 condemned + 1 read)", len(f.pages), len(jobs))
	}
}

func TestShineBoundDefersTheOldest(t *testing.T) {
	f := &shineFake{}
	s := newTestShine(f)
	s.maxNewPerRun = 1
	if _, err := s.Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 1 || !strings.HasSuffix(f.pages[0], "/19825133") {
		t.Errorf("pages = %v, want only the newest id", f.pages)
	}
}

func TestShineFailsWhenNoPageAnswers(t *testing.T) {
	f := &shineFake{pageErr: errors.New("connection reset")}
	if _, err := newTestShine(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl whose every page failed reported success")
	}
}

func TestShineParsePosting(t *testing.T) {
	p, ok := shineParsePosting("https://www.shine.com/jobs/senior-data-engineer-1/mastercard/19679658")
	if !ok || p.id != 19679658 || p.title != "senior data engineer 1" || p.company != "mastercard" {
		t.Errorf("parsed %+v, %v", p, ok)
	}
	for _, bad := range []string{"https://www.shine.com/jobs/x/19679658", "https://www.shine.com/job-search/x/y/1", "https://www.shine.com/jobs/x/y/z"} {
		if _, ok := shineParsePosting(bad); ok {
			t.Errorf("%s parsed as a posting", bad)
		}
	}
}

func TestShineJobMapsSalaryAndAllIndia(t *testing.T) {
	var ld shineLD
	ld.Title, ld.Description = "Financial Analyst", "<p>Analyse.</p>"
	ld.HiringOrganization.Name = "Acme"
	ld.BaseSalary.Currency = "INR"
	ld.BaseSalary.Value.UnitText, ld.BaseSalary.Value.MinValue, ld.BaseSalary.Value.MaxValue = "YEAR", 1200000, 1600000
	ld.JobLocation = append(ld.JobLocation, struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
		} `json:"address"`
	}{})
	ld.JobLocation[0].Address.Locality = "All India"
	j, ok := shineJob(ld, shinePosting{id: 1, url: "https://www.shine.com/jobs/a/b/1"})
	if !ok {
		t.Fatal("rejected")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 1200000 || *j.SalaryMax != 1600000 || j.SalaryCurrency != "INR" || j.SalaryPeriod != "year" {
		t.Errorf("salary %v-%v %s %s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.Location != "India" {
		t.Errorf("location = %q", j.Location)
	}
}
