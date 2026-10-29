package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// infostudFake serves the real fixtures (fetched 2026-10-28): one search page trimmed to four
// results whose total (34) implies a second page — Veriest (remote, IT, an hourly gross USD
// range), OIP Insurtech (hybrid, IT, a monthly net EUR range), Triumf Refrigeration (no salary)
// and Lilly drogerie — and the posting pages of the first three. Page 2 answers as page 1, as the
// site does past its last page. Any other posting answers 404.
type infostudFake struct {
	laterErr error
	archived bool

	mu      sync.Mutex
	listed  []string
	details []string
}

func (f *infostudFake) GetText(_ context.Context, u string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(u, "/oglasi-za-posao?page=") {
		f.listed = append(f.listed, u)
		if !strings.HasSuffix(u, "page=1") && f.laterErr != nil {
			return "", f.laterErr
		}
		return infostudFixture("list_page1.html"), nil
	}
	f.details = append(f.details, u)
	id := u[strings.LastIndex(u, "/")+1:]
	switch id {
	case "761195", "759819", "763959":
		page := infostudFixture("post_" + id + ".html")
		if f.archived {
			page = strings.Replace(page, `"status": "live"`, `"status": "archive"`, 1)
			page = strings.Replace(page, `"active": true`, `"active": false`, 1)
		}
		return page, nil
	}
	return "", &StatusError{Method: "GET", Code: 404, URL: u}
}

func infostudFixture(name string) string {
	b, err := os.ReadFile("testdata/infostud/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func newInfostudTest(f *infostudFake) infostud {
	s := NewInfostud(f).(infostud)
	s.limiter = noWait{}
	return s
}

func infostudByID(jobs []Job) map[string]Job {
	m := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestInfostudFetchNewGated(t *testing.T) {
	f := &infostudFake{}
	jobs, err := newInfostudTest(f).FetchNewGated(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "763639" }, DetailGate{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.listed) != 2 {
		t.Errorf("listing requests = %v, want pages 1 and 2 (the total implies two)", f.listed)
	}
	if len(f.details) != 3 {
		t.Errorf("detail requests = %v, want the three unseen postings", f.details)
	}
	got := infostudByID(jobs)
	if len(got) != 4 {
		t.Fatalf("jobs = %d, want 4 (page 2 answering as page 1 adds nothing)", len(got))
	}
	if j := got["763639"]; !j.SeenRefresh || j.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less refresh", j)
	}

	remote := got["761195"]
	if remote.WorkMode != "remote" || !remote.Remote || !remote.IsTechHint {
		t.Errorf("Veriest: mode %q remote %v tech %v, want a remote IT posting", remote.WorkMode, remote.Remote, remote.IsTechHint)
	}
	if remote.Location != "" {
		t.Errorf("Veriest location = %q, want the bare \"Rad od kuće\" stripped", remote.Location)
	}
	if remote.SalaryMin == nil || *remote.SalaryMin != 40 || remote.SalaryMax == nil || *remote.SalaryMax != 65 ||
		remote.SalaryCurrency != "USD" || remote.SalaryPeriod != "hour" {
		t.Errorf("Veriest salary = %v-%v %s/%s, want 40-65 USD/hour", remote.SalaryMin, remote.SalaryMax, remote.SalaryCurrency, remote.SalaryPeriod)
	}
	if !strings.Contains(remote.Description, "bruto 40 - 65 USD (satnica)") {
		t.Errorf("Veriest body does not lead with the site's salary label: %.120q", remote.Description)
	}

	hybrid := got["759819"]
	if hybrid.WorkMode != "hybrid" || hybrid.Remote || hybrid.Location != "Beograd" {
		t.Errorf("OIP: mode %q location %q, want hybrid in Beograd", hybrid.WorkMode, hybrid.Location)
	}
	if hybrid.SalaryMin == nil || *hybrid.SalaryMin != 3000 || hybrid.SalaryMax == nil || *hybrid.SalaryMax != 3300 ||
		hybrid.SalaryCurrency != "EUR" || hybrid.SalaryPeriod != "month" {
		t.Errorf("OIP salary = %v-%v %s/%s, want 3000-3300 EUR/month", hybrid.SalaryMin, hybrid.SalaryMax, hybrid.SalaryCurrency, hybrid.SalaryPeriod)
	}

	plain := got["763959"]
	if plain.SalaryMin != nil || plain.SalaryCurrency != "" || plain.WorkMode != "" || plain.IsTechHint {
		t.Errorf("Triumf = %+v, want no salary, no mode, not tech", plain)
	}
	if plain.Company != "Triumf Refrigeration" || !strings.Contains(plain.Description, "knjigovođu") {
		t.Errorf("Triumf company %q body %.80q", plain.Company, plain.Description)
	}
	for _, j := range []Job{remote, hybrid, plain} {
		if strings.Contains(j.Description, "Reference:") {
			t.Errorf("%s body keeps the hidden Reference watermark", j.ExternalID)
		}
	}
	if plain.PostedAt == nil || plain.PostedAt.Format("2006-01-02") != "2026-10-26" {
		t.Errorf("Triumf posted = %v, want 2026-10-26", plain.PostedAt)
	}
	if plain.URL != "https://poslovi.infostud.com/posao/knjigovodja/triumf-refrigeration/763959" {
		t.Errorf("Triumf url = %q", plain.URL)
	}
	for _, j := range jobs {
		if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "rs" {
			t.Errorf("%s fallback countries = %v, want [rs]", j.ExternalID, j.FallbackCountries)
		}
	}
}

func TestInfostudArchivedPostingIsGone(t *testing.T) {
	jobs, err := newInfostudTest(&infostudFake{archived: true}).FetchNewGated(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "763639" }, DetailGate{})
	if err != nil {
		t.Fatalf("archived postings are an answer, not a failure: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Errorf("jobs = %+v, want only the seen refresh", jobs)
	}
}

func TestInfostudLaterListingPageFailsTheCrawl(t *testing.T) {
	_, err := newInfostudTest(&infostudFake{laterErr: errors.New("boom")}).FetchNewGated(context.Background(), CompanyEntry{},
		func(string) bool { return false }, DetailGate{})
	if err == nil {
		t.Fatal("a failed later listing page must fail the crawl, not yield a partial listing")
	}
}

func TestInfostudGateSkipsCoveredEmployer(t *testing.T) {
	f := &infostudFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		return map[string]bool{"OIP Insurtech": true}
	}}
	jobs, err := newInfostudTest(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range f.details {
		if strings.HasSuffix(u, "/759819") {
			t.Errorf("a covered employer's posting page was requested: %s", u)
		}
	}
	if j := infostudByID(jobs)["759819"]; j.Description != "" || j.Title == "" {
		t.Errorf("covered posting = %+v, want body-less with its identity", j)
	}
}

func TestInfostudBudgetTakesFreshestFirst(t *testing.T) {
	f := &infostudFake{}
	s := newInfostudTest(f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, DetailGate{}); err != nil {
		t.Fatal(err)
	}
	if len(f.details) != 1 || !strings.HasSuffix(f.details[0], "/763959") {
		t.Errorf("details = %v, want only the freshest posting (26.10.2026)", f.details)
	}
}

func TestInfostudStopsNearDeadline(t *testing.T) {
	f := &infostudFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newInfostudTest(f).FetchNewGated(ctx, CompanyEntry{}, func(id string) bool { return id == "763639" }, DetailGate{})
	if err != nil {
		t.Fatalf("running out of time is not a failure: %v", err)
	}
	if len(f.details) != 0 || len(jobs) != 1 {
		t.Errorf("details %v jobs %d, want no posting page inside the deadline margin", f.details, len(jobs))
	}
}

func TestInfostudPlace(t *testing.T) {
	for in, want := range map[string]string{
		"Beograd | Hibrid":       "Beograd",
		"Beograd | Terenski rad": "Beograd",
		"Rad od kuće":            "",
		"Novi Sad":               "Novi Sad",
		"Berlin (Nemačka) ":      "Berlin (Nemačka)",
	} {
		if got := infostudPlace(in); got != want {
			t.Errorf("infostudPlace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInfostudSalary(t *testing.T) {
	str := func(s string) *string { return &s }
	type sal = struct {
		From     *string `json:"from"`
		To       *string `json:"to"`
		Period   string  `json:"additional"`
		Type     string  `json:"type"`
		Currency string  `json:"currency"`
	}
	cases := []struct {
		name   string
		s      *sal
		lo     int
		hi     int // 0 = no upper bound
		ok     bool
		period string
	}{
		{"monthly net RSD", &sal{str("20000000"), str("25000000"), "monthly", "net", "RSD"}, 200000, 250000, true, "month"},
		{"one figure", &sal{str("20000000"), nil, "monthly", "net", "RSD"}, 200000, 0, true, "month"},
		{"no figure", &sal{nil, nil, "monthly", "net", "RSD"}, 0, 0, false, ""},
		{"unknown period", &sal{str("100"), str("200"), "weekly", "net", "RSD"}, 0, 0, false, ""},
		{"net or gross unstated", &sal{str("100"), str("200"), "monthly", "", "RSD"}, 0, 0, false, ""},
		{"inverted", &sal{str("300"), str("200"), "monthly", "gross", "EUR"}, 0, 0, false, ""},
	}
	for _, c := range cases {
		p := infostudPosting{}
		if c.s != nil {
			p.Salary = c.s
		}
		lo, hi, _, period, ok := infostudSalary(p)
		if ok != c.ok || lo != c.lo || period != c.period || (hi == nil) != (c.hi == 0) || (hi != nil && *hi != c.hi) {
			t.Errorf("%s: got %d-%v %q ok=%v", c.name, lo, hi, period, ok)
		}
	}
}

func TestInfostudDropHidden(t *testing.T) {
	in := `<ul><li>Keep</li><li style="display:none;" class="x">Reference: abc</li></ul><div style="display: none"><ul></ul></div><p>Tail</p>`
	got := infostudDropHidden(in)
	if strings.Contains(got, "Reference") || strings.Contains(got, "display") || !strings.Contains(got, "<li>Keep</li>") || !strings.Contains(got, "<p>Tail</p>") {
		t.Errorf("infostudDropHidden = %q", got)
	}
}
