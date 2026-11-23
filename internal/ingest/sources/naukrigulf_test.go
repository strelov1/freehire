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
	"time"

	"golang.org/x/time/rate"
)

//go:embed testdata/naukrigulf
var naukrigulfFixtures embed.FS

// naukrigulfFake serves the live fixtures (fetched 2026-11-22 from the crawl host: the first
// listing page trimmed to three postings, and each one's detail). Offset 0 is the listing; any
// later offset answers an empty page, as the API does past the end.
type naukrigulfFake struct {
	listErr   error
	detailErr error
	// goneIDs answer 404 on their detail.
	goneIDs map[string]bool

	mu   sync.Mutex
	urls []string
}

func naukrigulfFixture(name string) []byte {
	raw, err := naukrigulfFixtures.ReadFile("testdata/naukrigulf/" + name)
	if err != nil {
		panic(err)
	}
	return raw
}

func (f *naukrigulfFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	if headers["version"] != "v1" || headers["appid"] != "205" || headers["systemid"] != "2323" {
		return errors.New("Naukrigulf request lacks the public frontend headers")
	}
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.mu.Unlock()
	switch {
	case strings.Contains(u, "/jobapi/search?"):
		if f.listErr != nil {
			return f.listErr
		}
		if !strings.HasSuffix(u, "Offset=0") {
			return json.Unmarshal([]byte(`{"Jobs":[]}`), v)
		}
		return json.Unmarshal(naukrigulfFixture("list.json"), v)
	case strings.Contains(u, "/spapi/jobs/"):
		if f.detailErr != nil {
			return f.detailErr
		}
		id := strings.TrimSuffix(strings.TrimPrefix(u, naukrigulfBase+"/spapi/jobs/"), "?seo=1&xz=1_0_5&source=srp")
		if f.goneIDs[id] {
			return &StatusError{Method: "GET", Code: 404, URL: u}
		}
		return json.Unmarshal(naukrigulfFixture("detail_"+id+".json"), v)
	}
	return &StatusError{Method: "GET", Code: 404, URL: u}
}

func (f *naukrigulfFake) detailRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, "/spapi/jobs/") {
			n++
		}
	}
	return n
}

func newTestNaukrigulf(f *naukrigulfFake) naukrigulf {
	s := NewNaukrigulf(f).(naukrigulf)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	s.retryBackoff = time.Millisecond
	return s
}

func naukrigulfByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

const (
	naukrigulfFinanceID      = "221126000090"
	naukrigulfConfidentialID = "221126000078"
	naukrigulfProcurementID  = "221126000060"
)

func TestNaukrigulfHydratesNewPostingsAndSkipsAnonymousOnes(t *testing.T) {
	f := &naukrigulfFake{}
	jobs, err := newTestNaukrigulf(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	byID := naukrigulfByID(jobs)
	if len(byID) != 2 {
		t.Fatalf("got %d postings, want 2 (the anonymous one is dropped): %v", len(byID), jobs)
	}
	if _, ok := byID[naukrigulfConfidentialID]; ok {
		t.Error("a posting under \"Confidential Company\" must not be yielded")
	}
	if got := f.detailRequests(); got != 2 {
		t.Errorf("detail requests = %d, want 2 (none is bought for the anonymous posting)", got)
	}
	j := byID[naukrigulfFinanceID]
	if j.Title != "Finance Manager" || j.Company != "INNOVATION DIRECT EMPLOYMENT SERVICES L.L.C" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Dubai - United Arab Emirates (UAE)" {
		t.Errorf("location = %q", j.Location)
	}
	if !strings.HasPrefix(j.URL, "https://www.naukrigulf.com/finance-manager-jobs-in-dubai") {
		t.Errorf("url = %q", j.URL)
	}
	if !strings.Contains(j.Description, "VAT returns") || !strings.Contains(j.Description, "UAE VAT and tax laws") {
		t.Errorf("description lacks the body or the desired-candidate profile: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Unix(1795366168, 0).UTC()) {
		t.Errorf("postedAt = %v", j.PostedAt)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 10 {
		t.Errorf("experience min = %v, want 10", j.ExperienceYearsMin)
	}
	if j.WorkMode != "onsite" || j.EmploymentType != "full_time" {
		t.Errorf("workMode/employmentType = %q / %q", j.WorkMode, j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 12000 || j.SalaryMax == nil || *j.SalaryMax != 15000 ||
		j.SalaryCurrency != "AED" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s, want 12000-15000 AED/month", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.IsTechHint {
		t.Error("an accounting posting must not be a tech hint")
	}
}

func TestNaukrigulfRefreshesSeenPostingsWithoutBuyingTheirBody(t *testing.T) {
	f := &naukrigulfFake{}
	seen := func(id string) bool { return id == naukrigulfFinanceID }
	jobs, err := newTestNaukrigulf(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	byID := naukrigulfByID(jobs)
	if !byID[naukrigulfFinanceID].SeenRefresh || byID[naukrigulfFinanceID].Description != "" {
		t.Errorf("a stored posting must be a body-less refresh: %+v", byID[naukrigulfFinanceID])
	}
	if byID[naukrigulfProcurementID].SeenRefresh || byID[naukrigulfProcurementID].Description == "" {
		t.Errorf("an unseen posting must be hydrated: %+v", byID[naukrigulfProcurementID])
	}
	if got := f.detailRequests(); got != 1 {
		t.Errorf("detail requests = %d, want 1", got)
	}
}

func TestNaukrigulfBoundsTheDetailsOneRunReads(t *testing.T) {
	f := &naukrigulfFake{}
	s := newTestNaukrigulf(f)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// The listing is newest first, so the one posting read is the Finance Manager.
	if len(jobs) != 1 || jobs[0].ExternalID != naukrigulfFinanceID {
		t.Errorf("jobs = %+v, want only the newest posting", jobs)
	}
}

func TestNaukrigulfWalksTheListingByOffsetUntilAnEmptyPage(t *testing.T) {
	f := &naukrigulfFake{}
	if _, err := newTestNaukrigulf(f).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	var lists []string
	for _, u := range f.urls {
		if strings.Contains(u, "/jobapi/search?") {
			lists = append(lists, u)
		}
	}
	want := []string{
		fmt.Sprintf(naukrigulfSearchURL, naukrigulfPageSize, 0),
		fmt.Sprintf(naukrigulfSearchURL, naukrigulfPageSize, naukrigulfPageSize),
	}
	if strings.Join(lists, "\n") != strings.Join(want, "\n") {
		t.Errorf("listing requests = %v, want %v", lists, want)
	}
}

func TestNaukrigulfAnUnreadableListingFailsTheCrawl(t *testing.T) {
	f := &naukrigulfFake{listErr: &StatusError{Method: "GET", Code: 403, URL: "x"}}
	if _, err := newTestNaukrigulf(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a listing that cannot be read must fail the crawl")
	}
}

func TestNaukrigulfRetriesARefusedRequest(t *testing.T) {
	calls := 0
	s := newTestNaukrigulf(&naukrigulfFake{})
	s.http = headerJSONFunc(func(context.Context, string, map[string]string, any) error {
		calls++
		if calls < 3 {
			return &StatusError{Method: "GET", Code: 403, URL: "u"}
		}
		return nil
	})
	if err := s.get(context.Background(), "u", nil); err != nil || calls != 3 {
		t.Errorf("err = %v after %d calls, want success on the third", err, calls)
	}
}

type headerJSONFunc func(ctx context.Context, url string, headers map[string]string, v any) error

func (f headerJSONFunc) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	return f(ctx, url, headers, v)
}

func TestNaukrigulfAPostingTheSiteReportsGoneIsNotYielded(t *testing.T) {
	f := &naukrigulfFake{goneIDs: map[string]bool{naukrigulfFinanceID: true}}
	jobs, err := newTestNaukrigulf(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	byID := naukrigulfByID(jobs)
	if _, ok := byID[naukrigulfFinanceID]; ok || len(byID) != 1 {
		t.Errorf("jobs = %v, want only the posting still served", jobs)
	}
}

func TestNaukrigulfACrawlThatReadNoDetailFails(t *testing.T) {
	f := &naukrigulfFake{detailErr: errors.New("boom")}
	_, err := newTestNaukrigulf(f).Fetch(context.Background(), CompanyEntry{})
	if err == nil || !strings.Contains(err.Error(), "none answered") {
		t.Fatalf("err = %v, want the none-answered failure", err)
	}
}

func TestNaukrigulfIsIT(t *testing.T) {
	for fa, want := range map[string]bool{
		"IT Software": true,
		"Software Development / Application Development (IT Software)":            true,
		"System Administration / Network Administration / Security (IT Software)": true,
		"Engineering": false,
		"Helpdesk / Customer Service / Telecalling": false,
		"": false,
	} {
		if got := naukrigulfIsIT(fa); got != want {
			t.Errorf("naukrigulfIsIT(%q) = %v, want %v", fa, got, want)
		}
	}
}

func TestNaukrigulfSalaryNeedsAStatedRangeAndCurrency(t *testing.T) {
	var d naukrigulfDetail
	d.Job.Compensation.MinCurrency, d.Job.Compensation.MaxCurrency = "QAR 5,000", "6,500"
	d.Job.Other.Currency = "QAR"
	var j Job
	naukrigulfSalary(&j, d)
	if j.SalaryMin == nil || *j.SalaryMin != 5000 || *j.SalaryMax != 6500 || j.SalaryCurrency != "QAR" {
		t.Fatalf("salary = %v-%v %s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	for name, mutate := range map[string]func(*naukrigulfDetail){
		"no currency": func(d *naukrigulfDetail) { d.Job.Other.Currency = "" },
		"no figure":   func(d *naukrigulfDetail) { d.Job.Compensation.MinCurrency = "" },
		"inverted":    func(d *naukrigulfDetail) { d.Job.Compensation.MaxCurrency = "1,000" },
	} {
		d2 := d
		mutate(&d2)
		var j2 Job
		naukrigulfSalary(&j2, d2)
		if j2.SalaryMin != nil || j2.SalaryCurrency != "" {
			t.Errorf("%s: salary must stay unset, got %v %s", name, j2.SalaryMin, j2.SalaryCurrency)
		}
	}
}
