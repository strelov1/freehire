package aggregators

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

// careerjetHTTP serves the testdata/careerjet fixtures by page number and records each request.
type careerjetHTTP struct {
	t       *testing.T
	pages   []string      // fixture file per page, 1-based by index
	errs    map[int]error // page -> transport error
	gotURLs []string
	headers []map[string]string
}

func (f *careerjetHTTP) GetJSONWithHeaders(_ context.Context, u string, h map[string]string, v any) error {
	f.gotURLs = append(f.gotURLs, u)
	f.headers = append(f.headers, h)
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(parsed.Query().Get("page"))
	if err := f.errs[page]; err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join("testdata", "careerjet", f.pages[page-1]))
	if err != nil {
		f.t.Fatal(err)
	}
	return json.Unmarshal(b, v)
}

var careerjetBoard = kit.CompanyEntry{Company: "Careerjet UK", Region: "en_GB", Board: "software engineer"}

func TestCareerjetFetchMapsJobsAndPaginates(t *testing.T) {
	f := &careerjetHTTP{t: t, pages: []string{"jobs_page1.json", "jobs_page2.json"}}
	jobs, err := NewCareerjet(f, "key", "203.0.113.7").Fetch(context.Background(), careerjetBoard)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || len(f.gotURLs) != 2 {
		t.Fatalf("want 3 jobs over 2 requests (stop at pages=2), got %d jobs, %d requests", len(jobs), len(f.gotURLs))
	}

	j := jobs[0]
	if j.Title != "Backend Engineer" || j.Company != "Acme Ltd" || j.Location != "Leeds, West Yorkshire" ||
		j.URL != "https://jobviewtrack.com/v2/aaa111" || j.PostedAt == nil || j.ExternalID == "" {
		t.Fatalf("mapping: %+v", j)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 60000 || j.SalaryMax == nil || *j.SalaryMax != 75000 ||
		j.SalaryCurrency != "GBP" || j.SalaryPeriod != "year" {
		t.Fatalf("salary: %+v", j)
	}
	// An hourly one-sided figure keeps its bound; a zero bound is unset.
	if h := jobs[1]; h.SalaryMin == nil || *h.SalaryMin != 25 || h.SalaryMax != nil || h.SalaryPeriod != "hour" {
		t.Fatalf("hourly salary: %+v", h)
	}
	// Weekly has no freehire period: the whole range is dropped.
	if w := jobs[2]; w.SalaryMin != nil || w.SalaryMax != nil || w.SalaryPeriod != "" || w.SalaryCurrency != "" {
		t.Fatalf("weekly salary should be dropped: %+v", w)
	}
	if jobs[0].ExternalID == jobs[1].ExternalID {
		t.Fatal("distinct postings share an external id")
	}
}

func TestCareerjetFetchSendsCredentialsAndRequiredParams(t *testing.T) {
	f := &careerjetHTTP{t: t, pages: []string{"jobs_page1.json", "jobs_page2.json"}}
	if _, err := NewCareerjet(f, "secret", "203.0.113.7").Fetch(context.Background(), careerjetBoard); err != nil {
		t.Fatal(err)
	}
	if got, want := f.headers[0]["Authorization"], "Basic "+base64.StdEncoding.EncodeToString([]byte("secret:")); got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	q, _ := url.Parse(f.gotURLs[0])
	for k, want := range map[string]string{
		"locale_code": "en_GB", "keywords": "software engineer", "sort": "date",
		"page_size": "100", "user_ip": "203.0.113.7", "user_agent": careerjetUserAgent,
	} {
		if got := q.Query().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// type LOCATIONS means no search ran: a board config error, never an empty feed.
func TestCareerjetFetchLocationsModeIsAnError(t *testing.T) {
	f := &careerjetHTTP{t: t, pages: []string{"locations.json"}}
	jobs, err := NewCareerjet(f, "k", "1.2.3.4").Fetch(context.Background(), careerjetBoard)
	if err == nil || !strings.Contains(err.Error(), "LOCATIONS") || jobs != nil {
		t.Fatalf("want a LOCATIONS error, got jobs=%v err=%v", jobs, err)
	}
}

func TestCareerjetFetchAuthFailuresFailTheBoard(t *testing.T) {
	for _, code := range []int{401, 403} {
		f := &careerjetHTTP{t: t, errs: map[int]error{1: &kit.StatusError{Method: "GET", Code: code}}}
		_, err := NewCareerjet(f, "k", "1.2.3.4").Fetch(context.Background(), careerjetBoard)
		var se *kit.StatusError
		if !errors.As(err, &se) || se.Code != code {
			t.Errorf("status %d: want it wrapped in the error, got %v", code, err)
		}
	}
}

func TestCareerjetFetchLaterPageFailureReturnsPartial(t *testing.T) {
	f := &careerjetHTTP{t: t, pages: []string{"jobs_page1.json"}, errs: map[int]error{2: errors.New("boom")}}
	jobs, err := NewCareerjet(f, "k", "1.2.3.4").Fetch(context.Background(), careerjetBoard)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("want the 2 page-1 jobs and no error, got %d, %v", len(jobs), err)
	}
}

func TestCareerjetFetchRejectsBlankBoard(t *testing.T) {
	c := NewCareerjet(&careerjetHTTP{t: t}, "k", "1.2.3.4")
	for _, e := range []kit.CompanyEntry{{Board: "go"}, {Region: "en_GB"}} {
		if _, err := c.Fetch(context.Background(), e); err == nil {
			t.Errorf("want an error for %+v", e)
		}
	}
}

// A feed reporting 50 pages is walked only to the API's page cap, and one board's budget is the
// API's own depth.
func TestCareerjetPageBudget(t *testing.T) {
	if careerjetMaxPages > 10 || careerjetPageSize > 100 {
		t.Fatalf("budget %d pages x %d exceeds the API's 10 x 100 depth", careerjetMaxPages, careerjetPageSize)
	}
	n := 0
	f := careerjetFunc(func(v any) error {
		n++
		return json.Unmarshal([]byte(`{"type":"JOBS","pages":50,"jobs":[{"title":"T`+strconv.Itoa(n)+`","company":"C","url":"https://jobviewtrack.com/v2/`+strconv.Itoa(n)+`"}]}`), v)
	})
	jobs, err := NewCareerjet(f, "k", "1.2.3.4").Fetch(context.Background(), careerjetBoard)
	if err != nil || n != careerjetMaxPages || len(jobs) != careerjetMaxPages {
		t.Fatalf("want %d requests and jobs, got %d requests, %d jobs, err %v", careerjetMaxPages, n, len(jobs), err)
	}
}

type careerjetFunc func(v any) error

func (f careerjetFunc) GetJSONWithHeaders(_ context.Context, _ string, _ map[string]string, v any) error {
	return f(v)
}

// Two vacancies of one employer with the same title and place must not share an id.
func TestCareerjetExternalIDSeparatesSameTitleVacancies(t *testing.T) {
	a := careerjetPosting{Title: "Nurse", Company: "NHS", Locations: "Leeds", Description: "Ward A"}
	b := a
	b.Description = "Ward B"
	if a.externalID() == b.externalID() {
		t.Fatal("different vacancies share an id")
	}
	b = a
	b.Date = "Mon, 13 Nov 2025 10:00:00 GMT"
	if a.externalID() != b.externalID() {
		t.Fatal("a re-dated repost must keep its id")
	}
}
