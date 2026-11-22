package sources

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/clearancejobs
var clearancejobsFixtures embed.FS

const clearancejobsXMLBase = clearancejobsBase + "/data/sitemaps/"

// clearancejobsFake serves trimmed sitemaps and one posting page whose title, industry and
// validThrough are filled per id. Ids 100 and 200 are expired (100 answers 404 as the site does
// for a removed page, 200 a validThrough in the past), 300 to 500 are live.
type clearancejobsFake struct {
	files   map[string]string
	failMap map[string]error
	pageErr error
	gone    map[int64]bool

	mu    sync.Mutex
	pages []string
}

func newClearancejobsFake() *clearancejobsFake {
	return &clearancejobsFake{files: map[string]string{
		clearancejobsSitemapIndex:                    "index.xml",
		clearancejobsXMLBase + "job_postings.xml":    "postings1.xml",
		clearancejobsXMLBase + "job_postings_2.xml":  "postings2.xml",
		clearancejobsXMLBase + "clearance_state.xml": "postings2.xml", // a landing file, never read
	}}
}

func clearancejobsFixture(name string) string {
	raw, err := clearancejobsFixtures.ReadFile("testdata/clearancejobs/" + name)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func (f *clearancejobsFake) GetStream(_ context.Context, u, _ string, fn func(io.Reader) error) error {
	if err := f.failMap[u]; err != nil {
		return err
	}
	name, ok := f.files[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return fn(strings.NewReader(clearancejobsFixture(name)))
}

func (f *clearancejobsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	p, ok := clearancejobsParsePosting(u)
	if !ok || p.id == 100 || f.gone[p.id] {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	valid, industry := "2027-01-19T16:00:00+00:00Z", "IT - Software"
	if p.id == 200 {
		valid = "2026-09-01T16:00:00+00:00Z"
	}
	if p.id == 500 {
		industry = "Management"
	}
	page := strings.NewReplacer("TITLE", "Posting "+strconv.FormatInt(p.id, 10), "VALID", valid, "INDUSTRY", industry).
		Replace(clearancejobsFixture("page.html"))
	return html.Parse(bytes.NewReader([]byte(page)))
}

func fastClearancejobs(f *clearancejobsFake) clearancejobs {
	s := NewClearanceJobs(f).(clearancejobs)
	s.limiter = nopWaiter{}
	s.retryBackoff = time.Millisecond
	s.now = func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) }
	return s
}

func TestClearancejobsBoundarySkipsRemovedProbe(t *testing.T) {
	f := newClearancejobsFake()
	f.gone = map[int64]bool{400: true}
	var postings []clearancejobsPosting
	for id := int64(100); id <= 600; id += 100 {
		postings = append(postings, clearancejobsPosting{id: id, url: fmt.Sprintf("%s/jobs/%d/posting", clearancejobsBase, id)})
	}
	got, err := fastClearancejobs(f).livePostings(context.Background(), postings)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[len(got)-1].id != 300 {
		t.Fatalf("live range = %+v, want to retain live ID 300 before the removed probe", got)
	}
}

func TestClearancejobsListsPostingSitemapsOldestFirstOnce(t *testing.T) {
	postings, err := fastClearancejobs(newClearancejobsFake()).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, p := range postings {
		ids = append(ids, p.id)
	}
	// 300 is in both files and the category landing page is no posting.
	if want := []int64{100, 200, 300, 400, 500}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestClearancejobsReadsOnlyLivePostingsAboveTheBoundary(t *testing.T) {
	f := newClearancejobsFake()
	jobs, err := fastClearancejobs(f).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "300" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 3 || !byID["300"].SeenRefresh || byID["100"].ExternalID != "" || byID["200"].ExternalID != "" {
		t.Fatalf("jobs = %+v, want 300 refreshed and 400/500 read, 100/200 untouched", jobs)
	}
	j := byID["400"]
	if j.Title != "Posting 400" || j.Company != "Guidehouse" || !j.IsTechHint {
		t.Errorf("identity = %+v", j)
	}
	if byID["500"].IsTechHint {
		t.Errorf("a non-IT category must not hint technical: %+v", byID["500"])
	}
	if j.Location != "Washington, DC; Berlin, Germany" || j.EmploymentType != "full_time" {
		t.Errorf("Location/EmploymentType = %q/%q", j.Location, j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Capital Projects &amp; Infrastructure") || !strings.Contains(j.Description, "Security Clearance: Top Secret/SCI") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 98000 || *j.SalaryMax != 163000 || j.SalaryPeriod != "year" || j.SalaryCurrency != "USD" {
		t.Errorf("salary = %+v", j)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Day() != 20 {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "us" {
		t.Errorf("FallbackCountries = %v", j.FallbackCountries)
	}
}

func TestClearancejobsReadsAtMostTheRunBoundNewestFirst(t *testing.T) {
	s := fastClearancejobs(newClearancejobsFake())
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "500" {
		t.Errorf("jobs = %+v, want only the newest", jobs)
	}
}

func TestClearancejobsFailsWhenASitemapFails(t *testing.T) {
	f := newClearancejobsFake()
	f.failMap = map[string]error{clearancejobsXMLBase + "job_postings_2.xml": errors.New("boom")}
	if _, err := fastClearancejobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true }); err == nil {
		t.Fatal("a short sitemap list must fail the crawl")
	}
}

func TestClearancejobsFailsWhenPagesAreRefused(t *testing.T) {
	f := newClearancejobsFake()
	f.pageErr = &StatusError{Method: "GET", Code: 403, URL: "x"}
	if _, err := fastClearancejobs(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that cannot read any page must fail, not read as quiet")
	}
}

func TestClearancejobsTimeDropsTheStrayZ(t *testing.T) {
	got, ok := clearancejobsTime("2026-12-28T16:07:03+00:00Z")
	if !ok || !got.Equal(time.Date(2026, 12, 28, 16, 7, 3, 0, time.UTC)) {
		t.Errorf("got %v %v", got, ok)
	}
	if _, ok := clearancejobsTime("soon"); ok {
		t.Error("garbage must not parse")
	}
}
