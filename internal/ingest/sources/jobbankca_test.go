package sources

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

//go:embed testdata/jobbankca
var jobbankcaFixtures embed.FS

// jobbankcaFake serves the live fixtures (fetched 2026-11-22: the first listing page trimmed to
// three rows, and five posting pages trimmed to their JobPosting block). A page with no fixture
// answers 404, as the site does for a posting that is gone.
type jobbankcaFake struct {
	pages map[string]string // URL -> fixture name
	err   error             // answered for every request when set

	mu   sync.Mutex
	hits []string
}

func newJobbankcaFake() *jobbankcaFake {
	f := &jobbankcaFake{pages: map[string]string{
		fmt.Sprintf(jobbankcaListingURL, 1): "listing.html",
	}}
	for _, id := range []string{"50781876", "50781877", "50770473", "50781793", "50781785", "50770467", "50774863", "50770468"} {
		f.pages[jobbankcaPostingLink(id)] = "posting_" + id + ".html"
	}
	return f
}

func (f *jobbankcaFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.hits = append(f.hits, u)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	name, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := jobbankcaFixtures.ReadFile("testdata/jobbankca/" + name)
	if err != nil {
		panic(err)
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *jobbankcaFake) postingHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.hits {
		if strings.Contains(u, "/jobposting/") {
			n++
		}
	}
	return n
}

func newTestJobBankCA(g HTMLGetter) jobbankca {
	return jobbankca{
		http:          g,
		limiter:       rate.NewLimiter(rate.Inf, 1),
		listingPages:  3,
		freshPerRun:   10,
		backlogPerRun: 0,
		now:           time.Now,
	}
}

func jobbankcaTestDetail(t *testing.T, id string) (Job, bool) {
	t.Helper()
	job, ok, err := newTestJobBankCA(newJobbankcaFake()).detail(context.Background(), id)
	if err != nil {
		t.Fatalf("detail(%s): %v", id, err)
	}
	return job, ok
}

func TestJobBankCAIsABoardlessAggregatorThatHydrates(t *testing.T) {
	s := NewJobBankCA(nil)
	if _, ok := s.(HydratingSource); !ok {
		t.Error("jobbankca must be a HydratingSource: the body costs a page request at a five-second pace")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("jobbankca must be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobbankca must be an aggregator: every posting names its own employer")
	}
	if got := SweepGraceWindows(Taxonomy())["jobbankca"]; got != jobbankcaSweepGrace {
		t.Errorf("sweep grace = %v, want %v", got, jobbankcaSweepGrace)
	}
}

func TestJobBankCAListingPageReadsPostingIDsFromTheLinks(t *testing.T) {
	ids, err := newTestJobBankCA(newJobbankcaFake()).listingPage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, ","), "50781876,50781802,50781793"; got != want {
		t.Errorf("ids = %s, want %s", got, want)
	}
}

func TestJobBankCADirectPostingWithAFixedWage(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50781876")
	if !ok {
		t.Fatal("posting dropped")
	}
	if j.Title != "child caregiver - private home" || j.Company != "Andrew Schwartz" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://www.jobbank.gc.ca/jobsearch/jobposting/50781876" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Westmount, QC, Canada" || len(j.Countries) != 1 || j.Countries[0] != "ca" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-22" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-13" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 28 || j.SalaryMax != nil || j.SalaryCurrency != "CAD" || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %v-%v %s/%s", deref(j.SalaryMin), deref(j.SalaryMax), j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.WorkMode != "" || j.Remote {
		t.Errorf("an \"On site\" default must state nothing: WorkMode=%q Remote=%v", j.WorkMode, j.Remote)
	}
	if !strings.Contains(j.Description, "Perform light housekeeping") || !strings.HasPrefix(j.Description, "<p>") {
		t.Errorf("description = %.120q", j.Description)
	}
}

func TestJobBankCASalaryRangeAndPartTime(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50781877")
	if !ok {
		t.Fatal("posting dropped")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 18 || j.SalaryMax == nil || *j.SalaryMax != 20 || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %v-%v/%s", deref(j.SalaryMin), deref(j.SalaryMax), j.SalaryPeriod)
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time (\"Casual employment\" then \"Part time\")", j.EmploymentType)
	}
}

func TestJobBankCAHybridPostingWithAnAnnualSalaryHasThousandsSeparators(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50770473")
	if !ok {
		t.Fatal("posting dropped")
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 87658 || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v/%s, want 87658/year (content=\"87,658.18\")", deref(j.SalaryMin), j.SalaryPeriod)
	}
}

func TestJobBankCAPartnerDescriptionThatIsEscapedMarkupIsRenderedAsMarkup(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50781793")
	if !ok {
		t.Fatal("posting dropped")
	}
	if strings.Contains(j.Description, "&lt;") || strings.Contains(j.Description, "&amp;lt;") {
		t.Errorf("description still carries escaped markup: %.200q", j.Description)
	}
	if !strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "Hospitality Aide") {
		t.Errorf("description = %.200q", j.Description)
	}
	if j.Company != "Sienna Senior Living" {
		t.Errorf("company = %q", j.Company)
	}
}

func TestJobBankCAVariousLocationsListEveryAddressWithoutStrayCommas(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50770467")
	if !ok {
		t.Fatal("posting dropped")
	}
	if want := "Québec, QC, Canada; Rimouski, QC, Canada"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
}

func TestJobBankCAPostingWithNoAddressReadsTheLocationLabel(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50770468")
	if !ok {
		t.Fatal("posting dropped")
	}
	if want := "Montréal Island, Canada"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
}

func TestJobBankCAEmployerNameDropsTheScreenReaderNoteBesideItsLink(t *testing.T) {
	j, ok := jobbankcaTestDetail(t, "50774863")
	if !ok {
		t.Fatal("posting dropped")
	}
	if want := "City Roofing & Exteriors"; j.Company != want {
		t.Errorf("Company = %q, want %q", j.Company, want)
	}
}

func TestJobBankCAPostingWithAnEmptyDescriptionIsNotYielded(t *testing.T) {
	if _, ok := jobbankcaTestDetail(t, "50781785"); ok {
		t.Error("a posting its partner sent without a body must not be yielded")
	}
}

func TestJobBankCAGonePostingIsDroppedAndAFailureIsAnError(t *testing.T) {
	s := newTestJobBankCA(newJobbankcaFake())
	if _, ok, err := s.detail(context.Background(), "1"); ok || err != nil {
		t.Errorf("a 404 posting: ok=%v err=%v, want dropped without an error", ok, err)
	}
	fake := newJobbankcaFake()
	fake.err = &StatusError{Method: "GET", Code: 500}
	s = newTestJobBankCA(fake)
	s.retryBackoff = time.Nanosecond
	if _, _, err := s.detail(context.Background(), "50781876"); err == nil {
		t.Error("a 500 is the absence of an answer and must be an error")
	}
	if got := len(fake.hits); got != 1+jobbankcaRetries {
		t.Errorf("a refused page was asked %d times, want %d", got, 1+jobbankcaRetries)
	}
}

func TestJobBankCAFetchNewRefreshesSeenPostingsAndHydratesTheRest(t *testing.T) {
	fake := newJobbankcaFake()
	fake.pages[fmt.Sprintf(jobbankcaListingURL, 1)] = "listing.html"
	s := newTestJobBankCA(fake)
	// The listing names 50781876 (hydrates), 50781802 (stored: refreshed) and 50781793 (hydrates).
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "50781802" })
	if err != nil {
		t.Fatal(err)
	}
	var refreshed, hydrated []string
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed = append(refreshed, j.ExternalID)
			if j.Description != "" || j.URL == "" {
				t.Errorf("a refresh carries identity only: %+v", j)
			}
		} else {
			hydrated = append(hydrated, j.ExternalID)
		}
	}
	if strings.Join(refreshed, ",") != "50781802" || strings.Join(hydrated, ",") != "50781876,50781793" {
		t.Errorf("refreshed %v hydrated %v", refreshed, hydrated)
	}
	if got := fake.postingHits(); got != 2 {
		t.Errorf("%d posting pages requested, want 2: a stored posting costs no page", got)
	}
	// Page 2 has no fixture (404): the walk ends with the first page's postings, not a failure.
	if len(fake.hits) != 1+2+1 {
		t.Errorf("requests = %v", fake.hits)
	}
}

func TestJobBankCAFetchNewBoundsThePostingPages(t *testing.T) {
	fake := newJobbankcaFake()
	s := newTestJobBankCA(fake)
	s.freshPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "50781876" {
		t.Errorf("jobs = %+v, want only the newest posting (highest id)", jobs)
	}
}

func TestJobBankCAFetchNewFailsWhenItReadNoneOfItsNewPostings(t *testing.T) {
	fake := newJobbankcaFake()
	delete(fake.pages, jobbankcaPostingLink("50781876"))
	delete(fake.pages, jobbankcaPostingLink("50781793"))
	// Both new postings answer 404, which is an answer, so the crawl is not broken.
	if _, err := newTestJobBankCA(fake).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "50781802" }); err != nil {
		t.Errorf("postings the site reports gone are answers, got %v", err)
	}
	// A page that is not a posting at all (a challenge served with 200) is no answer.
	fake.pages[jobbankcaPostingLink("50781793")] = "listing.html"
	fake.pages[jobbankcaPostingLink("50781876")] = "listing.html"
	if _, err := newTestJobBankCA(fake).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "50781802" }); err == nil {
		t.Error("a crawl that listed new postings and read none must fail")
	}
}

func TestJobBankCAFirstListingPageFailingFailsTheCrawl(t *testing.T) {
	fake := newJobbankcaFake()
	fake.err = errors.New("timeout")
	s := newTestJobBankCA(fake)
	s.retryBackoff = time.Nanosecond
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Error("an unreachable site must fail the crawl, not read as an empty one")
	}
}

func TestJobBankCAStopsHydratingNearTheDeadline(t *testing.T) {
	fake := newJobbankcaFake()
	s := newTestJobBankCA(fake)
	s.deadlineMargin = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("running out of time is not a failure: %v", err)
	}
	if len(jobs) != 0 || fake.postingHits() != 0 {
		t.Errorf("jobs=%d posting requests=%d, want none past the margin", len(jobs), fake.postingHits())
	}
}
