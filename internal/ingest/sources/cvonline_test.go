package sources

import (
	"context"
	"embed"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

//go:embed testdata/cvonline
var cvonlineFixtures embed.FS

func cvonlineFixture(tb testing.TB, name string) string {
	tb.Helper()
	raw, err := cvonlineFixtures.ReadFile("testdata/cvonline/" + name)
	if err != nil {
		tb.Fatal(err)
	}
	return string(raw)
}

// cvonlineFake serves the live fixtures (fetched 2026-11-22: trimmed sitemaps and three
// postings' __NEXT_DATA__ with the unused parts removed).
type cvonlineFake struct {
	t        testing.TB
	sitemaps map[string]string // sitemap URL -> fixture
	pages    map[string]string // page URL -> fixture
	pageErr  error

	mu   sync.Mutex
	read []string
}

func newCVOnlineFake(tb testing.TB) *cvonlineFake {
	tb.Helper()
	return &cvonlineFake{
		t: tb,
		sitemaps: map[string]string{
			"https://www.cv.lv/jobs-sitemap.xml": "sitemap_lv.xml",
			"https://www.cv.ee/jobs-sitemap.xml": "sitemap_ee.xml",
		},
		pages: map[string]string{
			"https://www.cv.lv/lv/vacancy/1683926/rica-nams-sia/gramatvedis":                     "lv_1683926.html",
			"https://www.cv.lv/lv/vacancy/1677104/compensa/x":                                    "lv_1677104.html",
			"https://www.cv.ee/et/vacancy/1681933/ds-smith-packaging-estonia-as/mehaanik-tehnik": "ee_1681933.html",
		},
	}
}

func (f *cvonlineFake) GetStream(_ context.Context, u, _ string, fn func(io.Reader) error) error {
	name, ok := f.sitemaps[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return fn(strings.NewReader(cvonlineFixture(f.t, name)))
}

func (f *cvonlineFake) GetText(_ context.Context, u string) (string, error) {
	f.mu.Lock()
	f.read = append(f.read, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return "", f.pageErr
	}
	name, ok := f.pages[u]
	if !ok {
		return "", &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return cvonlineFixture(f.t, name), nil
}

func newTestCVOnline(f cvonlineHTTP) cvonline {
	s := NewCVOnline(f).(cvonline)
	s.limiter = noWait{}
	return s
}

func cvonlineByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestCVOnlineFetchReadsNewAndRefreshesSeen(t *testing.T) {
	f := newCVOnlineFake(t)
	s := newTestCVOnline(f)
	seen := func(id string) bool { return id == "1675716" }
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	by := cvonlineByID(jobs)
	if len(jobs) != 3 {
		t.Fatalf("want 3 jobs, got %d", len(jobs))
	}
	if r := by["1675716"]; !r.SeenRefresh || r.Title != "" {
		t.Errorf("seen posting should be a bare refresh: %+v", r)
	}
	for _, u := range f.read {
		if strings.Contains(u, "1675716") {
			t.Errorf("seen posting was fetched: %s", u)
		}
	}
	j := by["1683926"]
	if j.Title != "Grāmatvedis" || j.Company != "RICA NAMS SIA" {
		t.Errorf("title/company: %q / %q", j.Title, j.Company)
	}
	if j.Location == "" {
		t.Errorf("location empty")
	}
	if !strings.Contains(j.Description, "<h3>") || !strings.Contains(j.Description, "<li>") && !strings.Contains(j.Description, "<div>") {
		t.Errorf("description not assembled: %q", j.Description)
	}
	if j.WorkMode != "hybrid" || j.EmploymentType != "full_time" {
		t.Errorf("work mode / type: %q %q", j.WorkMode, j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 1600 || *j.SalaryMax != 2200 || j.SalaryPeriod != "month" || j.SalaryCurrency != "EUR" {
		t.Errorf("salary: %+v", j)
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "lv" {
		t.Errorf("country: %v", j.FallbackCountries)
	}
	if j.PostedAt == nil {
		t.Errorf("posted date missing")
	}
	if j.IsTechHint {
		t.Errorf("finance posting hinted technical")
	}
}

func TestCVOnlineKeepsBodylessPosting(t *testing.T) {
	s := newTestCVOnline(newCVOnlineFake(t))
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	j, ok := cvonlineByID(jobs)["1677104"]
	if !ok {
		t.Fatal("posting whose text is an attached file was dropped")
	}
	if j.Description != "" || j.Title == "" {
		t.Errorf("want title and an empty body: %+v", j)
	}
}

func TestCVOnlineEstonianBoard(t *testing.T) {
	s := newTestCVOnline(newCVOnlineFake(t))
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvonline", Board: "ee"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	j := jobs[0]
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "ee" {
		t.Errorf("country: %v", j.FallbackCountries)
	}
	if j.SalaryMin != nil {
		t.Errorf("a monthly 13 is not pay: %+v", j)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("FULL_TIME_WITH_SHIFTS not read as full time: %q", j.EmploymentType)
	}
}

func TestCVOnlineITCategoryHintsTech(t *testing.T) {
	var v cvonlineVacancy
	v.Position, v.EmployerName = "Engineer", "Acme"
	v.Settings.Categories = []string{"ELECTRONICS_TELECOM", "INFORMATION_TECHNOLOGY"}
	j, ok := cvonlineJob(v, cvonlineLocations{}, cvonlineSites["lv"], cvonlinePosting{id: 1, url: "u"})
	if !ok || !j.IsTechHint {
		t.Fatalf("IT category should hint technical: %+v", j)
	}
}

func TestCVOnlineGonePostingIsDroppedAndNoAnswerFailsBoard(t *testing.T) {
	f := newCVOnlineFake(t)
	f.pages = map[string]string{} // every page answers 404: gone, which is an answer
	s := newTestCVOnline(f)
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("404s are answers: jobs=%d err=%v", len(jobs), err)
	}
	f.pageErr = errors.New("boom")
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"}); err == nil {
		t.Fatal("a crawl that read no page must fail")
	}
}

func TestCVOnlinePublishedPostingWithoutEmployerIsUnreadable(t *testing.T) {
	const page = "https://www.cv.lv/lv/vacancy/1683926/rica-nams-sia/gramatvedis"
	f := newCVOnlineFake(t)
	f.pages[page] = "lv_1683926_missing_employer.html"
	s := newTestCVOnline(f)
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"})
	if err != nil {
		t.Fatal(err)
	}
	if j := cvonlineByID(jobs)["1683926"]; !j.Unreadable || j.URL != page {
		t.Errorf("want the malformed published posting's unreadable marker, got %+v", j)
	}
}

func TestCVOnlineBoundsNewReadsPerRun(t *testing.T) {
	f := newCVOnlineFake(t)
	s := newTestCVOnline(f)
	s.maxNewPerRun = 1
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "cvonline", Board: "lv"}); err != nil {
		t.Fatal(err)
	}
	if len(f.read) != 1 || !strings.Contains(f.read[0], "1683926") {
		t.Errorf("want exactly the newest id read, got %v", f.read)
	}
}

func TestCVOnlineBoardValidation(t *testing.T) {
	s := NewCVOnline(nil).(cvonline)
	for _, b := range []string{"lv", "ee"} {
		if err := s.ValidateBoard(b); err != nil {
			t.Errorf("%s: %v", b, err)
		}
	}
	if err := s.ValidateBoard("lt"); err == nil {
		t.Error("lt is not a CV-Online board")
	}
	if s.SweepGrace() < 24*time.Hour {
		t.Error("sweep grace too short")
	}
}
