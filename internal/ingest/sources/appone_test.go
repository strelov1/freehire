package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// apponeFixture reads a response captured verbatim from AppOne's live API (2026-10-20).
func apponeFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/appone/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// newTestAppOne builds the adapter without the 403 retry's wait.
func newTestAppOne(c JSONGetter) appone { return appone{http: c} }

func TestAppOneProvider(t *testing.T) {
	if got := NewAppOne(nil).Provider(); got != "appone" {
		t.Errorf("Provider() = %q, want %q", got, "appone")
	}
}

func TestAppOneIsNotAFullBoardListing(t *testing.T) {
	// The portal listing stops at 100 postings and takes no paging parameter, so a crawl that
	// returns may not be the whole board.
	if _, ok := NewAppOne(nil).(fullBoardListing); ok {
		t.Error("appone must not claim fullBoardListing: its listing is capped at 100")
	}
}

func TestAppOneFetchMapsAPortal(t *testing.T) {
	fake := (&routedHTTP{}).
		route("companyjobposts/streamlineworkcomp", apponeFixture(t, "list_streamlineworkcomp.json")).
		route("jobposting/6ac795efd03e41f35800e54b", apponeFixture(t, "detail_6ac795efd03e41f35800e54b.json"))
	// The portal's other postings answer the platform's own "closed or removed" 404, which drops them.
	for _, id := range []string{"6aa83a9fa90d13310695e743", "6ad2432cf81766f2f3d5d23b", "6a97171bfb6d8e7b38e8d27e", "69cd37198d092b12d04c794d", "6aad89a0b67bb35e61c6d70b"} {
		fake.routeErr("jobposting/"+id, &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	}

	jobs, err := newTestAppOne(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Streamline Imaging", Provider: "appone", Board: "streamlineworkcomp",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want the one posting whose detail answered", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "6ac795efd03e41f35800e54b" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://apply.appone.com/job/6ac795efd03e41f35800e54b" {
		t.Errorf("URL = %q, want the posting's apply page", j.URL)
	}
	if j.Title != "Mid-Level QA Analyst" || j.Company != "Streamline Imaging" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Jacksonville, FL" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/remote", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 80000 || j.SalaryMax == nil || *j.SalaryMax != 100000 ||
		j.SalaryCurrency != "USD" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %q/%q, want 80000-100000 USD/year", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want the listing's datePosted", j.PostedAt)
	}
	// The plain-text body is rebuilt into paragraphs rather than one wall of text.
	if !strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "QA") {
		t.Errorf("Description = %.200q, want paragraph markup around the posting's text", j.Description)
	}
}

func TestAppOneSalaryOptions(t *testing.T) {
	ptr := func(v int) *int { return &v }
	cases := []struct {
		name   string
		s      *apponeSalary
		lo, hi *int
		period string
	}{
		{"range", &apponeSalary{Minimum: 14, Maximum: 15.5, Option: "RANGE", PeriodType: "HOUR"}, ptr(14), ptr(16), "hour"},
		{"exact amount is both bounds", &apponeSalary{Minimum: 62000, Option: "EXACT_AMOUNT", PeriodType: "YEAR"}, ptr(62000), ptr(62000), "year"},
		{"starting amount is a floor", &apponeSalary{Minimum: 16.9, Option: "STARTING_AMOUNT", PeriodType: "HOUR"}, ptr(17), nil, "hour"},
		{"maximum amount is a ceiling", &apponeSalary{Maximum: 17, Option: "MAXIMUM_AMOUNT", PeriodType: "HOUR"}, nil, ptr(17), "hour"},
		{"no salary", nil, nil, nil, ""},
		{"unknown period writes nothing", &apponeSalary{Minimum: 900, Option: "EXACT_AMOUNT", PeriodType: "WEEK"}, nil, nil, ""},
	}
	eq := func(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var j Job
			c.s.apply(&j)
			if !eq(j.SalaryMin, c.lo) || !eq(j.SalaryMax, c.hi) || j.SalaryPeriod != c.period {
				t.Errorf("got %v-%v %q, want %v-%v %q", j.SalaryMin, j.SalaryMax, j.SalaryPeriod, c.lo, c.hi, c.period)
			}
			if (c.period == "") != (j.SalaryCurrency == "") {
				t.Errorf("SalaryCurrency = %q alongside period %q", j.SalaryCurrency, j.SalaryPeriod)
			}
		})
	}
}

func TestAppOneFetchFailsWhenThePortalIsGone(t *testing.T) {
	// An unknown portal answers 404. That fails the board — an empty board would license a sweep.
	fake := (&routedHTTP{}).routeErr("companyjobposts/", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	if _, err := newTestAppOne(fake).Fetch(context.Background(), CompanyEntry{Company: "Gone", Provider: "appone", Board: "gone"}); err == nil {
		t.Fatal("Fetch succeeded on a missing portal, want an error")
	}
}

func TestAppOneFetchMarksAnUnreadableDetail(t *testing.T) {
	// The detail is the posting's only body, so one this crawl could not read must not read as
	// gone; it becomes an Unreadable marker the pipeline counts.
	fake := (&routedHTTP{}).
		route("companyjobposts/CRWA", apponeFixture(t, "list_crwa.json")).
		routeErr("jobposting/", errors.New("timeout"))

	jobs, err := newTestAppOne(fake).Fetch(context.Background(), CompanyEntry{Company: "CRWA", Provider: "appone", Board: "CRWA"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want a marker per listed posting", len(jobs))
	}
	for _, j := range jobs {
		if !j.Unreadable || j.ExternalID == "" || !strings.HasPrefix(j.URL, "https://apply.appone.com/job/") {
			t.Errorf("job = %+v, want an Unreadable marker carrying the posting's identity", j)
		}
	}
}

func TestAppOneFetchNewSkipsDetailForSeenPostings(t *testing.T) {
	// No detail route: requesting one would come back unreadable, which the test rejects.
	fake := (&routedHTTP{}).route("companyjobposts/CRWA", apponeFixture(t, "list_crwa.json"))

	var src HydratingSource = newTestAppOne(fake)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Company: "CRWA", Provider: "appone", Board: "CRWA"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Unreadable || j.ExternalID == "" || j.Title == "" {
			t.Errorf("job = %+v, want a SeenRefresh without a detail request", j)
		}
	}
}

// flakyForbidden answers 403 a fixed number of times before serving body, the shape AppOne's
// edge was measured to have: a bare "Forbidden" on a share of requests unrelated to their rate.
type flakyForbidden struct {
	mu        sync.Mutex
	forbidden int
	calls     int
	body      string
}

func (f *flakyForbidden) GetJSON(ctx context.Context, url string, v any) error {
	f.mu.Lock()
	f.calls++
	refuse := f.calls <= f.forbidden
	f.mu.Unlock()
	if refuse {
		return &StatusError{Method: http.MethodGet, Code: http.StatusForbidden, URL: url}
	}
	return (&routedHTTP{}).route("", f.body).GetJSON(ctx, url, v)
}

func TestAppOneRetriesAStray403(t *testing.T) {
	f := &flakyForbidden{forbidden: apponeAttempts - 1, body: apponeFixture(t, "list_crwa.json")}
	postings, err := newTestAppOne(f).list(context.Background(), "CRWA")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(postings) != 2 || f.calls != apponeAttempts {
		t.Errorf("postings=%d calls=%d, want 2 postings after %d attempts", len(postings), f.calls, apponeAttempts)
	}
}

func TestAppOneGivesUpOnAPersistent403(t *testing.T) {
	f := &flakyForbidden{forbidden: apponeAttempts, body: "{}"}
	if _, err := newTestAppOne(f).list(context.Background(), "CRWA"); err == nil {
		t.Fatal("list succeeded through a 403 on every attempt")
	}
	if f.calls != apponeAttempts {
		t.Errorf("calls = %d, want %d", f.calls, apponeAttempts)
	}
}

func TestAppOneRetryRespectsCancellation(t *testing.T) {
	f := &flakyForbidden{forbidden: apponeAttempts, body: "{}"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := appone{http: f, retryDelay: time.Hour}
	if err := a.getJSON(ctx, ApponePortalURL+"CRWA", &struct{}{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled instead of waiting out the retry delay", err)
	}
}
