package sources

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// founditFixtures holds the trimmed live responses the tests serve.
//
//go:embed testdata/foundit
var founditFixtures embed.FS

// founditFake serves the live fixtures (fetched 2026-11-17: a trimmed search page and the NTT
// Data posting's detail). It rejects a request without the foundit Referer the way the real API
// does, so a regression in the headers fails here instead of in production.
type founditFake struct {
	search map[string]string // slice query -> body; "*" serves any slice
	detail map[string]string // id -> body
	// total, when set, makes the fake page: it serves `total` synthetic rows across pages.
	total     int
	searchErr map[string]error // slice query -> error on every page
	detailErr error

	mu      sync.Mutex
	starts  []int
	queries []string
	details []string
}

func (f *founditFake) GetJSONWithHeaders(_ context.Context, u string, h map[string]string, v any) error {
	if h["Referer"] == "" {
		return &StatusError{Method: "GET", Code: 400, URL: u}
	}
	if rest, ok := strings.CutPrefix(u, founditSearchURL); ok {
		q, err := url.ParseQuery(rest)
		if err != nil {
			return err
		}
		if q.Get("sort") != founditSortDate || q.Get("jobFreshness") != strconv.Itoa(founditFreshnessDays) {
			return fmt.Errorf("founditFake: unexpected search %s", rest)
		}
		start, _ := strconv.Atoi(q.Get("start"))
		f.mu.Lock()
		f.starts = append(f.starts, start)
		f.queries = append(f.queries, q.Get("query"))
		f.mu.Unlock()
		if err := f.searchErr[q.Get("query")]; err != nil {
			return err
		}
		if f.total > 0 {
			return json.Unmarshal([]byte(f.pagedBody(start)), v)
		}
		raw, ok := f.search[q.Get("query")]
		if !ok {
			raw, ok = f.search["*"]
		}
		if !ok {
			return fmt.Errorf("founditFake: no search for %q", q.Get("query"))
		}
		return json.Unmarshal([]byte(raw), v)
	}
	if id, ok := strings.CutPrefix(u, founditBase+"/middleware/jobdetail/"); ok {
		f.mu.Lock()
		f.details = append(f.details, id)
		f.mu.Unlock()
		if f.detailErr != nil {
			return f.detailErr
		}
		raw, ok := f.detail[id]
		if !ok {
			return json.Unmarshal([]byte(`{"jobDetailStatus":404,"jobDetailStatusText":"Not Found"}`), v)
		}
		return json.Unmarshal([]byte(raw), v)
	}
	return fmt.Errorf("founditFake: no route for %s", u)
}

// pagedBody serves rows for ids start..start+99 of the fake's total, plus one ad slot.
func (f *founditFake) pagedBody(start int) string {
	var rows []string
	for i := start; i < start+founditPageSize && i < f.total; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"%d","title":"Engineer %d","companyName":"Co","seoJdUrl":"/job/e-%d","createdAt":%d}`, i+1, i, i+1, 1794882757000-int64(i)))
	}
	rows = append(rows, `{"type":"adsense"}`)
	return fmt.Sprintf(`{"jobSearchStatus":200,"jobSearchResponse":{"data":[%s],"meta":{"paging":{"total":%d}}}}`, strings.Join(rows, ","), f.total)
}

func founditFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := founditFixtures.ReadFile("testdata/foundit/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newFounditFake(t *testing.T) *founditFake {
	t.Helper()
	return &founditFake{
		search: map[string]string{"*": founditFixture(t, "search.json")},
		detail: map[string]string{"69831543": founditFixture(t, "detail_69831543.json")},
	}
}

// slices narrows the crawl to the named queries for one test.
func withFounditSlices(t *testing.T, slices ...string) {
	t.Helper()
	old := founditSlices
	founditSlices = slices
	t.Cleanup(func() { founditSlices = old })
}

func TestFounditReadsNewPostingsFromTheirDetailAndDedupsSlices(t *testing.T) {
	withFounditSlices(t, "java", "developer")
	f := newFounditFake(t)
	jobs, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id != "69831543" })
	if err != nil {
		t.Fatal(err)
	}
	// The same four postings answer both slices; each is yielded once and the ad slot never.
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4: %+v", len(jobs), jobs)
	}
	// Only the one unseen posting costs a detail request.
	if len(f.details) != 1 || f.details[0] != "69831543" {
		t.Fatalf("detail requests = %v, want [69831543]", f.details)
	}
	by := jobsByID(jobs)
	j := by["69831543"]
	if j.SeenRefresh || !strings.Contains(j.Description, "Patch Microsoft 365") {
		t.Errorf("unseen posting not hydrated: %+v", j)
	}
	if j.Location != "Noida, Uttar Pradesh, India" {
		t.Errorf("Location = %q, want the structured place without the country-only duplicate", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("a Work From Home job type should state remote work: %+v", j)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.URL != "https://www.foundit.in/job/systems-engineering-advisor-patching-m365-infrastructure-engineer-ntt-data-noida-india-69831543" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "NTT Data" || j.Title == "" || j.PostedAt == nil {
		t.Errorf("identity fields lost: %+v", j)
	}
	if s := by["69831550"]; !s.SeenRefresh || s.Description != "" {
		t.Errorf("a seen posting must be a body-less liveness refresh: %+v", s)
	}
	for _, q := range []string{"java", "developer"} {
		found := false
		for _, got := range f.queries {
			found = found || got == q
		}
		if !found {
			t.Errorf("slice %q never searched: %v", q, f.queries)
		}
	}
}

func TestFounditMapsSalaryExperienceAndContract(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	jobs, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{},
		func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	j := jobsByID(jobs)["69831999"]
	if j.SalaryMin == nil || *j.SalaryMin != 2100000 || *j.SalaryMax != 2800000 || j.SalaryCurrency != "INR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %+v", j)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 4 {
		t.Errorf("ExperienceYearsMin = %v, want 4", j.ExperienceYearsMin)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j.EmploymentType)
	}
	// A posting stating no salary publishes none: the live rows carry zeros.
	if z := jobsByID(jobs)["69831550"]; z.SalaryMin != nil || z.SalaryCurrency != "" {
		t.Errorf("a zero salary must not be published: %+v", z)
	}
}

func TestFounditListsNewestFirst(t *testing.T) {
	withFounditSlices(t, "java")
	f := &founditFake{total: 250}
	rows, err := foundit{http: f, spent: nil}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 250 {
		t.Fatalf("got %d rows, want 250", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].CreatedAt > rows[i-1].CreatedAt {
			t.Fatalf("row %d is newer than row %d", i, i-1)
		}
	}
	// 250 rows are three pages; the walk stops at the declared total.
	if fmt.Sprint(f.starts) != "[0 100 200]" {
		t.Errorf("page starts = %v, want [0 100 200]", f.starts)
	}
}

func TestFounditStopsAtTheSearchWindow(t *testing.T) {
	withFounditSlices(t, "python")
	f := &founditFake{total: 50_000}
	rows, err := foundit{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != founditWindow {
		t.Errorf("got %d rows, want the window's %d", len(rows), founditWindow)
	}
	if last := f.starts[len(f.starts)-1]; last != founditWindow-founditPageSize {
		t.Errorf("last page start = %d, want %d", last, founditWindow-founditPageSize)
	}
}

func TestFounditSendsTheRefererTheAPIRequires(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	var seen []map[string]string
	spy := headerSpy{inner: f, seen: &seen}
	if _, err := NewFoundit(spy).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no request made")
	}
	for _, h := range seen {
		if h["Referer"] != founditBase+"/" {
			t.Errorf("request sent headers %v, want the foundit Referer", h)
		}
	}
}

type headerSpy struct {
	inner HeaderJSONGetter
	seen  *[]map[string]string
}

func (s headerSpy) GetJSONWithHeaders(ctx context.Context, u string, h map[string]string, v any) error {
	*s.seen = append(*s.seen, h)
	return s.inner.GetJSONWithHeaders(ctx, u, h, v)
}

func TestFounditGateSpendsNoDetailOnCondemnedPostings(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	gate := DetailGate{
		Covered: func([]string) map[string]bool { return map[string]bool{"Acme": true} },
	}
	jobs, err := NewFoundit(f).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	// Acme is covered, so its posting must not be read.
	for _, id := range f.details {
		if id == "69831999" {
			t.Errorf("a condemned posting %s cost a detail request", id)
		}
	}
	by := jobsByID(jobs)
	for _, id := range []string{"69831999"} {
		j, ok := by[id]
		if !ok || j.Description != "" {
			t.Errorf("condemned posting %s should be yielded body-less: %+v ok=%v", id, j, ok)
		}
	}
}

func TestFounditDefersPastTheDetailBudget(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	s := NewFoundit(f).(foundit)
	s.spent.Store(founditDetailBudget) // the run has spent its budget
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 || len(f.details) != 0 {
		t.Errorf("budget spent: want nothing yielded or requested, got %d jobs, %d details", len(jobs), len(f.details))
	}
}

func TestFounditDropsAPostingThePlatformSaysIsGone(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	// Only 69831543 has a detail; the other three answer jobDetailStatus 404.
	jobs, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "69831543" {
		t.Errorf("got %+v, want only the posting that still has a body", jobs)
	}
}

func TestFounditFailsACrawlWhoseDetailsAllFail(t *testing.T) {
	withFounditSlices(t, "java")
	f := newFounditFake(t)
	f.detailErr = errors.New("scrapling: 503")
	_, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "none answered") {
		t.Fatalf("err = %v, want a none-answered failure", err)
	}
}

func TestFounditFailsWhenNoSliceAnswers(t *testing.T) {
	withFounditSlices(t, "java", "developer")
	f := newFounditFake(t)
	f.searchErr = map[string]error{
		"java":      &StatusError{Method: "GET", Code: 403, URL: "x"},
		"developer": &StatusError{Method: "GET", Code: 403, URL: "x"},
	}
	_, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err == nil {
		t.Fatal("a crawl in which no slice answered must fail, not pass for a quiet source")
	}
}

func TestFounditKeepsWhatOtherSlicesRead(t *testing.T) {
	withFounditSlices(t, "java", "developer")
	f := newFounditFake(t)
	f.searchErr = map[string]error{"java": &StatusError{Method: "GET", Code: 500, URL: "x"}}
	jobs, err := NewFoundit(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("one failing slice must not fail the crawl: %v", err)
	}
	if len(jobs) != 4 {
		t.Errorf("got %d jobs, want the working slice's 4", len(jobs))
	}
}

func TestFounditReadsAnErrorStatusInsideAnHTTP200(t *testing.T) {
	withFounditSlices(t, "java")
	f := &founditFake{search: map[string]string{"*": `{"jobSearchStatus":500,"jobSearchResponse":{}}`}}
	if _, err := (foundit{http: f}).list(context.Background()); err == nil {
		t.Fatal("a search answering status 500 inside HTTP 200 must fail the slice")
	}
}

func TestFounditToJobRefusesWhatIsNotAPublicPosting(t *testing.T) {
	inactive := false
	cases := map[string]founditListing{
		"ad slot":      {Type: "banner"},
		"no company":   {ID: "1", Title: "Dev", SeoJdURL: "/job/dev-1"},
		"hidden co":    {ID: "1", Title: "Dev", CompanyName: "X", HideCompanyName: 1, SeoJdURL: "/job/dev-1"},
		"no job path":  {ID: "1", Title: "Dev", CompanyName: "X", SeoJdURL: "/search/x"},
		"inactive job": {ID: "1", Title: "Dev", CompanyName: "X", SeoJdURL: "/job/dev-1", IsJobActive: &inactive},
	}
	for name, r := range cases {
		if _, ok := r.toJob(); ok {
			t.Errorf("%s: toJob accepted a row it must refuse", name)
		}
	}
}

func TestFounditHidesAConfidentialSalary(t *testing.T) {
	r := founditListing{
		ID: "1", Title: "Dev", CompanyName: "X", SeoJdURL: "/job/dev-1", HideSalary: 1,
		MinimumSalary: founditMoney{Currency: "INR", AbsoluteValue: 100}, MaximumSalary: founditMoney{Currency: "INR", AbsoluteValue: 200},
	}
	if j, ok := r.toJob(); !ok || j.SalaryMin != nil {
		t.Errorf("hideSalary=1 must not publish a salary: %+v", j)
	}
	r.HideSalary, r.SalaryConfidental = 0, true
	if j, _ := r.toJob(); j.SalaryMin != nil {
		t.Errorf("a confidential salary must not be published: %+v", j)
	}
}

func TestFounditEmploymentType(t *testing.T) {
	cases := []struct {
		jobTypes, schedules []string
		want                string
	}{
		{nil, []string{"Full time"}, "full_time"},
		{nil, []string{"Part time"}, "part_time"},
		{[]string{"Contract Job"}, []string{"Full time"}, "contract"},
		{[]string{"Internship"}, []string{"Full time"}, "internship"},
		{[]string{"Jobs for Women"}, nil, ""},
		{nil, nil, ""},
	}
	for _, c := range cases {
		if got := founditEmploymentType(c.jobTypes, c.schedules); got != c.want {
			t.Errorf("founditEmploymentType(%v, %v) = %q, want %q", c.jobTypes, c.schedules, got, c.want)
		}
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
	if g, ok := s.(interface{ sweepGrace() time.Duration }); !ok || g.sweepGrace() != founditSweepGrace {
		t.Error("foundit should declare its sweep grace")
	}
}
