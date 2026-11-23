package sources

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestJobsACUKRegistry(t *testing.T) {
	src := Taxonomy()["jobsacuk"]
	if src == nil {
		t.Fatal("jobs.ac.uk is absent from the source registry")
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Fatal("jobs.ac.uk detail and listing must be read separately")
	}
	if _, ok := src.(interface{ Boardless() }); !ok {
		t.Fatal("jobs.ac.uk must crawl one boardless catalogue")
	}
	if _, ok := src.(interface{ Aggregator() }); !ok {
		t.Fatal("jobs.ac.uk names employers per posting")
	}
}

//go:embed testdata/jobsacuk
var jobsACUKFixtures embed.FS

const jobsACUKFixtureURL = "https://www.jobs.ac.uk/job/DTS558/commercial-manager"

func jobsACUKFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := jobsACUKFixtures.ReadFile("testdata/jobsacuk/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newTestJobsACUK(f *routedHTTP) jobsACUK {
	s := NewJobsACUK(f).(jobsACUK)
	s.limiter = noWait{}
	s.now = func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) }
	return s
}

func jobsACUKTwoPageListing(t *testing.T, second string) *routedHTTP {
	t.Helper()
	f := &routedHTTP{}
	return f.route("startIndex=26", second).route("/search/", jobsACUKFixture(t, "search-first.html"))
}

func TestJobsACUKHydratesTheLastPageAndRefreshesKnownPostings(t *testing.T) {
	f := jobsACUKTwoPageListing(t, jobsACUKFixture(t, "search-last.html"))
	f.route(jobsACUKFixtureURL, jobsACUKFixture(t, "commercial-manager.html"))
	s := newTestJobsACUK(f)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != "DTS558" })
	if err != nil || len(jobs) != 26 || f.calls != 3 {
		t.Fatalf("got %d jobs, %d requests and %v, want 26 jobs and only two listings and one detail read", len(jobs), f.calls, err)
	}
	for _, j := range jobs[:25] {
		if !j.SeenRefresh || j.Unreadable {
			t.Errorf("posting %s should be refreshed by identity: %+v", j.ExternalID, j)
		}
	}
	j := jobs[25]
	if j.ExternalID != "DTS558" || j.URL != jobsACUKFixtureURL || j.Title != "Commercial Manager" ||
		j.Company != "University of Leicester" || j.SeenRefresh || j.Unreadable {
		t.Fatalf("wrong detail identity: %+v", j)
	}
	if j.Location != "Leicester, England" || len(j.Countries) != 1 || j.Countries[0] != "gb" ||
		j.IsTechHint || j.EmploymentType != "full_time" {
		t.Errorf("wrong location, work type or tech evidence: %+v", j)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 51258 || j.SalaryMax == nil || *j.SalaryMax != 57666 ||
		j.SalaryCurrency != "GBP" || j.SalaryPeriod != "year" {
		t.Errorf("wrong salary: %+v", j)
	}
	if !strings.Contains(j.Description, "commercialisation") || j.ApplicationDeadline == nil ||
		j.ApplicationDeadline.Format(time.DateOnly) != "2026-12-11" {
		t.Errorf("wrong body or deadline: %+v", j)
	}
}

func TestJobsACUKUsesSecondSortOnlyToFillListingGap(t *testing.T) {
	first := jobsACUKFixture(t, "search-first.html")
	otherOrder := strings.Replace(first, "/job/TST025/example-25", "/job/DTS558/commercial-manager", 1)
	f := (&routedHTTP{}).
		route("sortOrder=1&pageSize=25&startIndex=1", first).
		route("sortOrder=1&pageSize=25&startIndex=26", "<html><body>Unexpected page</body></html>").
		route("sortOrder=2&pageSize=25&startIndex=1", otherOrder).
		route("sortOrder=2&pageSize=25&startIndex=26", `<strong class="job-count">26</strong><div id="job-listings"><a href="/job/TST025/example-25">Example 25</a></div>`).
		route(jobsACUKFixtureURL, jobsACUKFixture(t, "commercial-manager.html"))
	jobs, err := newTestJobsACUK(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != "DTS558" })
	if err != nil || len(jobs) != 26 || f.calls != 5 {
		t.Fatalf("got %d jobs, %d requests and %v, want the gap recovered before hydration", len(jobs), f.calls, err)
	}
	if j := jobs[25]; j.ExternalID != "DTS558" || j.Unreadable || j.SeenRefresh {
		t.Errorf("wrong fallback detail: %+v", j)
	}
}

func TestJobsACUKFailsIncompleteOrInconsistentListings(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		requests   int
	}{
		{"interstitial across both sorts", "<html><body>Unexpected page</body></html>", 4},
		{"missing last result", `<strong class="job-count">26</strong><div id="job-listings"></div>`, 4},
		{"duplicate last result", `<strong class="job-count">26</strong><div id="job-listings"><a href="/job/TST001/example-1">Example</a></div>`, 2},
		{"changed total", `<strong class="job-count">27</strong><div id="job-listings"><a href="/job/DTS558/commercial-manager">Commercial Manager</a></div>`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := jobsACUKTwoPageListing(t, tc.line)
			jobs, err := newTestJobsACUK(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
			if err == nil || len(jobs) != 0 || f.calls != tc.requests {
				t.Fatalf("got %d jobs, %d requests and %v, want the broken listing to fail before hydration", len(jobs), f.calls, err)
			}
		})
	}
}

func TestJobsACUKAcceptsAConfirmedEmptyListing(t *testing.T) {
	f := (&routedHTTP{}).route("/search/", `<strong class="job-count">0</strong><div id="job-listings"></div>`)
	jobs, err := newTestJobsACUK(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 || f.calls != 1 {
		t.Fatalf("got %d jobs, %d requests and %v, want a confirmed empty listing", len(jobs), f.calls, err)
	}
}

func TestJobsACUKReportsUnreadableDetailsByIdentity(t *testing.T) {
	const listing = `<strong class="job-count">3</strong><div id="job-listings">
		<a href="/job/DTS558/commercial-manager">Commercial Manager</a>
		<a href="/job/DTS718/research-fellow-in-cell-biology">Research Fellow</a>
		<a href="/job/AAA000/gone">Gone</a></div>`
	f := (&routedHTTP{}).route("/search/", listing).
		route(jobsACUKFixtureURL, jobsACUKFixture(t, "commercial-manager.html")).
		route("/job/DTS718/", "<html><body>Unexpected page</body></html>").
		routeErr("/job/AAA000/", &StatusError{Code: http.StatusNotFound})
	s := newTestJobsACUK(f)
	jobs, err := s.Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("got %d jobs and %v, want live detail and unreadable marker", len(jobs), err)
	}
	if j := jobs[1]; !j.Unreadable || j.SeenRefresh || j.ExternalID != "DTS718" ||
		j.URL != "https://www.jobs.ac.uk/job/DTS718/research-fellow-in-cell-biology" {
		t.Errorf("wrong unreadable marker: %+v", j)
	}
	jobs, err = s.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != "DTS718" })
	if err == nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs and %v, want the only attempted detail failure to fail the crawl", len(jobs), err)
	}
}

func TestJobsACUKKeepsThePostingThroughItsClosingDate(t *testing.T) {
	// The source serializes an inclusive closing date as midnight.
	p := listedPosting{id: "DTS558", url: jobsACUKFixtureURL}
	page := (&routedHTTP{}).route(jobsACUKFixtureURL, jobsACUKFixture(t, "commercial-manager.html"))
	s := newTestJobsACUK(page)
	for _, day := range []int{11, 12} {
		t.Run(fmt.Sprint(day), func(t *testing.T) {
			s.now = func() time.Time { return time.Date(2026, 12, day, 12, 0, 0, 0, time.UTC) }
			_, ok, err := s.detail(context.Background(), p)
			if err != nil || ok != (day == 11) {
				t.Errorf("posting on December %d: open=%v, err=%v", day, ok, err)
			}
		})
	}
}
