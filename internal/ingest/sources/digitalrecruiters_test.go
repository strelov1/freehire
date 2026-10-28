package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// drFake serves the captured API answers (recrute.ouicare.com, 2026-10-28) keyed by request
// path; an answer may be overridden by an error or by literal JSON.
type drFake struct {
	t      *testing.T
	files  map[string]string // path -> fixture
	bodies map[string]string // path -> literal JSON
	errs   map[string]error
	mu     sync.Mutex
	reqs   []string
}

func (f *drFake) answer(raw string, v any) error {
	f.mu.Lock()
	f.reqs = append(f.reqs, raw)
	f.mu.Unlock()
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if e := f.errs[u.Path]; e != nil {
		return e
	}
	body, ok := f.bodies[u.Path]
	if !ok {
		name, ok := f.files[u.Path]
		if !ok {
			return &StatusError{Code: 404}
		}
		b, err := os.ReadFile(filepath.Join("testdata", "digitalrecruiters", name))
		if err != nil {
			f.t.Fatalf("fixture %s: %v", name, err)
		}
		body = string(b)
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *drFake) GetJSON(_ context.Context, u string, v any) error { return f.answer(u, v) }
func (f *drFake) PostJSON(_ context.Context, u string, _ any, v any) error {
	return f.answer(u, v)
}

const (
	drTestBoard   = "recrute.ouicare.com"
	drSitePath    = "/careers/v1/careers-sites/" + drTestBoard
	drListingPath = "/public/v1/careers-site/job-ads"
	drJobPath     = "/public/v1/careers-site/job-ads/4664997"
)

func newDRFake(t *testing.T) *drFake {
	return &drFake{t: t, files: map[string]string{
		drSitePath:    "site.json",
		drListingPath: "listing.json",
		drJobPath:     "job_4664997.json",
	}, bodies: map[string]string{}, errs: map[string]error{}}
}

func newDRTest(f *drFake) digitalrecruiters {
	return digitalrecruiters{http: f, maxNewPerRun: 100, deadlineMargin: time.Minute}
}

func TestDigitalrecruitersReadsOnlyTheBoardsOwnPostings(t *testing.T) {
	f := newDRFake(t)
	jobs, err := newDRTest(f).FetchNew(context.Background(), CompanyEntry{Board: drTestBoard, Company: "Oui Care"},
		func(id string) bool { return id != "4664997" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// The listing names five postings; two belong to the sites it aggregates.
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want the site's own 3: %+v", len(jobs), jobs)
	}
	var j *Job
	for i := range jobs {
		if jobs[i].ExternalID == "4667799" || jobs[i].ExternalID == "4667751" {
			t.Errorf("an aggregated posting was read: %+v", jobs[i])
		}
		if !jobs[i].SeenRefresh {
			j = &jobs[i]
		}
	}
	if j == nil {
		t.Fatal("the new posting was not hydrated")
	}
	if j.ExternalID != "4664997" || j.URL != "https://recrute.ouicare.com/fr/annonce/4664997-charge-de-recrutement-hf-75008-paris" {
		t.Errorf("identity = %s %s", j.ExternalID, j.URL)
	}
	if j.Title != "Chargé de recrutement H/F" || j.Company != "Oui Care" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Paris, Île-de-France, France" || len(j.Countries) != 1 || j.Countries[0] != "fr" {
		t.Errorf("location = %q %v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 2500 || j.SalaryMax == nil || *j.SalaryMax != 2800 || j.SalaryPeriod != "month" || j.SalaryCurrency != "EUR" {
		t.Errorf("salary = %v-%v %s %s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || !strings.Contains(j.Description, "<h3>Profile</h3>") {
		t.Errorf("posted %v, description %.200q", j.PostedAt, j.Description)
	}
	for _, r := range f.reqs {
		if strings.Contains(r, "/job-ads?") && !strings.Contains(r, "locale=fr_FR") {
			t.Errorf("listing asked in the wrong locale: %s", r)
		}
	}
}

func TestDigitalrecruitersListingMustMatchItsCount(t *testing.T) {
	f := newDRFake(t)
	f.bodies[drListingPath] = `{"count": 6, "items": []}`
	if _, err := newDRTest(f).Fetch(context.Background(), CompanyEntry{Board: drTestBoard}); err == nil {
		t.Fatal("a listing short of its stated count must fail the crawl")
	}
	f.bodies[drListingPath] = `{"items": []}`
	if _, err := newDRTest(f).Fetch(context.Background(), CompanyEntry{Board: drTestBoard}); err == nil {
		t.Fatal("a listing that states no count must fail the crawl")
	}
}

func TestDigitalrecruitersSiteVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name, site string
		err        error
		gone       bool
	}{
		{name: "unknown site", err: &StatusError{Code: 404}, gone: true},
		{name: "offline site", site: `{"is_online": false, "is_internal": false, "default_locale": {"iso_code": "fr"}}`, gone: true},
		{name: "internal mobility", site: `{"is_online": true, "is_internal": true, "default_locale": {"iso_code": "fr"}}`},
		{name: "transient failure", err: &StatusError{Code: 503}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDRFake(t)
			if tc.err != nil {
				f.errs[drSitePath] = tc.err
			} else {
				f.bodies[drSitePath] = tc.site
			}
			_, err := newDRTest(f).Fetch(context.Background(), CompanyEntry{Board: drTestBoard})
			if err == nil {
				t.Fatal("want the crawl to fail")
			}
			if got := errors.Is(err, ErrBoardGone); got != tc.gone {
				t.Errorf("ErrBoardGone = %v, want %v (%v)", got, tc.gone, err)
			}
		})
	}
}

func TestDigitalrecruitersPostingVerdicts(t *testing.T) {
	seen := func(id string) bool { return id != "4664997" }
	f := newDRFake(t)
	f.errs[drJobPath] = &StatusError{Code: 404}
	jobs, err := newDRTest(f).FetchNew(context.Background(), CompanyEntry{Board: drTestBoard}, seen)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("a withdrawn posting must be dropped: %d jobs, %v", len(jobs), err)
	}
	f = newDRFake(t)
	f.errs[drJobPath] = errors.New("timeout")
	jobs, err = newDRTest(f).FetchNew(context.Background(), CompanyEntry{Board: drTestBoard}, seen)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.ExternalID == "4664997" {
			found = j.Unreadable
		}
	}
	if !found {
		t.Fatal("a posting whose read failed must be an unreadable marker")
	}
}

func TestDigitalrecruitersBudgetDefers(t *testing.T) {
	f := newDRFake(t)
	s := newDRTest(f)
	s.maxNewPerRun = 0
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: drTestBoard})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%d err=%v", len(jobs), err)
	}
	for _, r := range f.reqs {
		if strings.Contains(r, drJobPath) {
			t.Fatal("no posting may be read past the budget")
		}
	}
}

func TestDigitalrecruitersAPILocale(t *testing.T) {
	for in, want := range map[string]string{"fr": "fr_FR", "fr_CA": "fr_CA", "de_CH": "de_CH", "en": "en_GB", "fr-CH": "fr_CH", "nl": "nl_BE", "xx": "xx"} {
		if got := drAPILocale(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestDigitalrecruitersRejectsANonHostBoard(t *testing.T) {
	f := newDRFake(t)
	for _, b := range []string{"", "https://x.example.com", "x.example.com/fr", "nodot"} {
		if _, err := newDRTest(f).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
	if len(f.reqs) != 0 {
		t.Errorf("requests sent: %q", f.reqs)
	}
}
