package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/foundit
var founditFixtures embed.FS

// founditFake serves the live fixtures (fetched 2026-11-17: trimmed sitemaps and the
// Infosys posting's JobPosting block). The active files are served gzip-compressed, as the site
// serves them; a page with no fixture answers 403 as Akamai does.
type founditFake struct {
	files   map[string]string // sitemap URL -> fixture name
	failMap map[string]error  // sitemap URL -> error
	pageErr error

	mu    sync.Mutex
	pages []string
}

const founditXMLBase = founditBase + "/xmlsitemap/"

func newFounditFake() *founditFake {
	return &founditFake{files: map[string]string{
		founditSitemapIndex:                            "index.xml",
		founditXMLBase + "active-jobs-sitemap0.xml.gz": "active0.xml",
		founditXMLBase + "active-jobs-sitemap1.xml.gz": "active1.xml",
		founditXMLBase + "todays-jobs-sitemap.xml.gz":  "today.xml",
	}}
}

func founditFixture(name string) []byte {
	raw, err := founditFixtures.ReadFile("testdata/foundit/" + name)
	if err != nil {
		panic(err)
	}
	return raw
}

func (f *founditFake) GetStream(_ context.Context, u, _ string, fn func(io.Reader) error) error {
	if err := f.failMap[u]; err != nil {
		return err
	}
	name, ok := f.files[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	body := founditFixture(name)
	if strings.HasSuffix(u, ".gz") && strings.HasPrefix(name, "active") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(body)
		_ = zw.Close()
		body = buf.Bytes()
	}
	return fn(bytes.NewReader(body))
}

func (f *founditFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if !strings.HasSuffix(u, "-69824181") {
		return nil, &StatusError{Method: "GET", Code: 403, URL: u}
	}
	return html.Parse(bytes.NewReader(founditFixture("job_69824181.html")))
}

func fastFoundit(f *founditFake) foundit {
	s := NewFoundit(f).(foundit)
	s.limiter = nopWaiter{}
	return s
}

type nopWaiter struct{}

func (nopWaiter) Wait(context.Context) error { return nil }

func TestFounditListsEverySitemapPostingNewestFirstOnce(t *testing.T) {
	postings, err := fastFoundit(newFounditFake()).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, p := range postings {
		ids = append(ids, p.id)
	}
	// 5 distinct postings across the three files; 69802546 is in both active files and the
	// search landing page is no posting. Newest id first.
	want := []int64{69831149, 69824181, 69818763, 69802547, 69802546, 44138175}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestFounditFetchReadsNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := newFounditFake()
	jobs, err := fastFoundit(f).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id != "69824181" })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 1 || !strings.HasSuffix(f.pages[0], "-69824181") {
		t.Fatalf("page reads = %v, want only the unseen posting", f.pages)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6 (5 refreshed + 1 read)", len(jobs))
	}
	var read Job
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.Description != "" {
				t.Errorf("a refreshed posting must carry no body: %+v", j)
			}
			continue
		}
		read = j
	}
	if refreshed != 5 {
		t.Errorf("refreshed %d, want 5", refreshed)
	}
	if read.ExternalID != "69824181" || read.Title != "Consultant - Advanced Engineering" || read.Company != "Infosys Limited" {
		t.Errorf("identity = %+v", read)
	}
	if !strings.Contains(read.Description, "collaborative engineering team") {
		t.Errorf("Description = %q", read.Description)
	}
	if read.Location != "Bengaluru / Bangalore, Karnataka, India" || read.EmploymentType != "full_time" {
		t.Errorf("Location/EmploymentType = %q/%q", read.Location, read.EmploymentType)
	}
	if read.PostedAt == nil || !read.PostedAt.Equal(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-11-16 (day first)", read.PostedAt)
	}
	if read.SalaryMin != nil {
		t.Errorf("a hidden salary must not be published: %+v", read)
	}
	if len(read.FallbackCountries) != 1 || read.FallbackCountries[0] != "in" {
		t.Errorf("FallbackCountries = %v", read.FallbackCountries)
	}
}

func TestFounditReadsAtMostTheRunBoundNewestFirst(t *testing.T) {
	f := newFounditFake()
	s := fastFoundit(f)
	s.maxNewPerRun = 2
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// The two newest ids are asked about (69831149 has no page, 69824181 has); nothing past the
	// bound is read, and the deferred postings are not yielded body-less.
	if len(f.pages) != 2 || !strings.HasSuffix(f.pages[0], "-69831149") && !strings.HasSuffix(f.pages[1], "-69831149") {
		t.Errorf("page reads = %v, want the two newest", f.pages)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "69824181" {
		t.Errorf("jobs = %+v, want only the readable one of the two", jobs)
	}
}

func TestFounditFailsWhenAnyNamedSitemapFails(t *testing.T) {
	f := newFounditFake()
	f.failMap = map[string]error{founditXMLBase + "active-jobs-sitemap1.xml.gz": errors.New("boom")}
	if _, err := fastFoundit(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true }); err == nil {
		t.Fatal("a short sitemap list must fail the crawl, not read as a shrunken catalogue")
	}
}

func TestFounditFailsWhenTheIndexNamesNoActiveFile(t *testing.T) {
	f := newFounditFake()
	f.files = map[string]string{founditSitemapIndex: "today.xml"} // a urlset where the index should be
	if _, err := fastFoundit(f).list(context.Background()); err == nil {
		t.Fatal("an index naming no active-jobs file must fail")
	}
}

func TestFounditFailsACrawlWhosePagesAllFail(t *testing.T) {
	f := newFounditFake()
	f.pageErr = &StatusError{Method: "GET", Code: 403, URL: "x"}
	_, err := fastFoundit(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "none answered") {
		t.Fatalf("err = %v, want a none-answered failure", err)
	}
}

func TestFounditDropsAPostingTheSiteSaysIsGone(t *testing.T) {
	f := newFounditFake()
	f.pageErr = &StatusError{Method: "GET", Code: 404, URL: "x"}
	jobs, err := fastFoundit(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("a 404 is an answer, not an outage: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs = %+v, want none", jobs)
	}
}

func TestFounditJobRefusesWhatIsNotAPosting(t *testing.T) {
	p := founditPosting{id: 1, url: founditBase + "/job/x-1"}
	good := founditLD{Title: "Dev", Description: "<p>body</p>"}
	good.HiringOrganization.Name = "Acme"
	if _, ok := founditJob(good, p); !ok {
		t.Fatal("a complete posting was refused")
	}
	for name, mut := range map[string]func(*founditLD){
		"no title":   func(l *founditLD) { l.Title = "" },
		"no company": func(l *founditLD) { l.HiringOrganization.Name = "" },
		"no body":    func(l *founditLD) { l.Description = "  " },
	} {
		l := good
		mut(&l)
		if _, ok := founditJob(l, p); ok {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestFounditPlacesReadsAPlaceOrAList(t *testing.T) {
	one := `{"address":{"addressLocality":"Pune, Maharashtra, India"}}`
	two := `[{"address":{"addressLocality":"Pune, Maharashtra, India"}},{"address":{"addressLocality":"Noida, Uttar Pradesh, India"}},{"address":{"addressLocality":"Pune, Maharashtra, India"}}]`
	if got := founditPlaces([]byte(one)); got != "Pune, Maharashtra, India" {
		t.Errorf("one = %q", got)
	}
	if got := founditPlaces([]byte(two)); got != "Pune, Maharashtra, India; Noida, Uttar Pradesh, India" {
		t.Errorf("two = %q", got)
	}
	if got := founditPlaces(nil); got != "" {
		t.Errorf("none = %q", got)
	}
}

func TestFounditJobMapsAYearlySalaryAndRemote(t *testing.T) {
	p := founditPosting{id: 1, url: founditBase + "/job/x-1"}
	ld := founditLD{Title: "Dev", Description: "<p>body</p>", JobLocationType: "TELECOMMUTE", EmploymentType: "Contract"}
	ld.HiringOrganization.Name = "Acme"
	ld.BaseSalary.Currency = "inr"
	ld.BaseSalary.Value.UnitText = "YEAR"
	ld.BaseSalary.Value.MinValue, ld.BaseSalary.Value.MaxValue = 1200000, 1800000
	j, ok := founditJob(ld, p)
	if !ok || j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "contract" {
		t.Errorf("job = %+v", j)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 1200000 || *j.SalaryMax != 1800000 || j.SalaryCurrency != "INR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %+v", j)
	}
}

func TestFounditParsePostingIgnoresLandingPages(t *testing.T) {
	for _, u := range []string{founditBase + "/search/jobs-in-pune", founditBase + "/job/", founditBase + "/job/no-id-here", founditBase + "/srp/results?query=x"} {
		if _, ok := founditParsePosting(u); ok {
			t.Errorf("%s was read as a posting", u)
		}
	}
	if p, ok := founditParsePosting(founditBase + "/job/billing-engineer-g-r-infraprojects-limited-india-69802546"); !ok || p.id != 69802546 {
		t.Errorf("posting = %+v, %v", p, ok)
	}
}

func TestFounditIsARegisteredBoardlessAggregator(t *testing.T) {
	s, ok := All(NewClient())["foundit"]
	if !ok {
		t.Fatal("foundit is not in sources.All")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("foundit should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("foundit should be an aggregator")
	}
	if g, ok := s.(interface{ SweepGrace() time.Duration }); !ok || g.SweepGrace() != founditSweepGrace {
		t.Error("foundit should declare its sweep grace")
	}
}
