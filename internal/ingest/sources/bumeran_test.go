package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// bumeranFake serves the real fixtures (fetched 2026-10-27): one search page trimmed to five
// results — Calsa (hybrid) and Nutrishop (a stated salary), Corporación America, a confidential
// posting and one filed under a bare "Consultora" — and the posting API's answers for the first
// two. Any other posting answers 404, as a gone one does.
type bumeranFake struct {
	listErr error
	// laterErr fails every listing page after the first.
	laterErr error
	// expired makes the posting API answer the fixture with estado "vencido".
	expired bool

	mu       sync.Mutex
	posts    []string
	details  []string
	badHeads int
}

func (f *bumeranFake) checkHeaders(h map[string]string) {
	if h["x-site-id"] != bumeranSiteID || h["Referer"] != bumeranReferer {
		f.badHeads++
	}
}

func (f *bumeranFake) PostJSONWithHeaders(_ context.Context, u string, h map[string]string, _ any, v any) error {
	f.mu.Lock()
	f.posts = append(f.posts, u)
	f.checkHeaders(h)
	f.mu.Unlock()
	if f.listErr != nil {
		return f.listErr
	}
	if !strings.Contains(u, "&page=0&") {
		if f.laterErr != nil {
			return f.laterErr
		}
		return json.Unmarshal([]byte(`{"total":6,"content":[]}`), v)
	}
	return bumeranFixture("testdata/bumeran/list.json", v)
}

func (f *bumeranFake) GetJSONWithHeaders(_ context.Context, u string, h map[string]string, v any) error {
	f.mu.Lock()
	f.details = append(f.details, u)
	f.checkHeaders(h)
	f.mu.Unlock()
	id := u[strings.LastIndex(u, "/")+1:]
	if id != "1118498914" && id != "1118468906" {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	if err := bumeranFixture("testdata/bumeran/ficha_"+id+".json", v); err != nil {
		return err
	}
	if f.expired {
		v.(*bumeranDetail).Aviso.Status = "vencido"
	}
	return nil
}

func bumeranFixture(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func newTestBumeran(f *bumeranFake) bumeran {
	return bumeran{http: f, get: f, limiter: noWait{}, maxNewPerRun: bumeranMaxNewPerRun, maxPages: bumeranMaxPages, deadlineMargin: bumeranDeadlineMargin}
}

func TestBumeranHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &bumeranFake{}
	jobs, err := newTestBumeran(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "1118498873" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want two hydrated and one refreshed (hidden employers dropped): %+v", len(jobs), jobs)
	}
	for _, hidden := range []string{"1118499004", "1118224980"} {
		if _, ok := byID[hidden]; ok {
			t.Errorf("posting %s with a hidden employer was yielded", hidden)
		}
	}
	if r := byID["1118498873"]; !r.SeenRefresh || r.Description != "" || r.Company != "Corporación America SA" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	j := byID["1118498914"]
	if j.Title != "Analista Contable" || j.Company != "Calsa" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if !strings.Contains(j.Description, "<p>") {
		t.Errorf("description lost its markup: %.120q", j.Description)
	}
	if j.Location != "Capital Federal, Buenos Aires, Argentina" || len(j.Countries) != 1 || j.Countries[0] != "ar" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.WorkMode != "hybrid" || j.EmploymentType != "full_time" {
		t.Errorf("work mode / employment = %q / %q", j.WorkMode, j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02 15:04") != "2026-10-27 21:17" {
		t.Errorf("posted at = %v, want 18:17 Argentine time", j.PostedAt)
	}
	if j.URL != bumeranBaseURL+"/empleos/analista-contable-calsa-1118498914.html" {
		t.Errorf("url = %q", j.URL)
	}
	if j.SalaryMin != nil {
		t.Errorf("salary set from a posting that states none: %v", *j.SalaryMin)
	}
	n := byID["1118468906"]
	if n.SalaryMin == nil || *n.SalaryMin != 1200000 || *n.SalaryMax != 1300000 || n.SalaryCurrency != "ARS" || n.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q %q, want 1200000-1300000 ARS month", n.SalaryMin, n.SalaryMax, n.SalaryCurrency, n.SalaryPeriod)
	}
	if n.WorkMode != "" {
		t.Errorf("the form's default Presencial became work mode %q", n.WorkMode)
	}
	if f.badHeads != 0 {
		t.Errorf("%d requests went out without the site id and Referer the edge requires", f.badHeads)
	}
	if len(f.details) != 2 {
		t.Errorf("posting requests = %v, want only the two unseen named postings", f.details)
	}
}

func TestBumeranHiddenEmployer(t *testing.T) {
	for name, want := range map[string]bool{
		"Confidencial":                      true,
		"Consultora":                        true,
		"Importante Empresa de Retail":      true,
		"Empresa Alimenticia":               true,
		"Empresa Distribuidora Sur S.A.":    false,
		"Consultora Fashion RH":             false,
		"ADECCO RECURSOS HUMANOS ARGENTINA": false,
	} {
		if got := bumeranHiddenEmployer(name, false); got != want {
			t.Errorf("bumeranHiddenEmployer(%q) = %v, want %v", name, got, want)
		}
	}
	if !bumeranHiddenEmployer("Grupo Gestión", true) {
		t.Error("the confidential flag did not hide the employer")
	}
}

func TestBumeranGateSkipsCoveredPostingsWithoutARequest(t *testing.T) {
	f := &bumeranFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[c] = true
		}
		return m
	}}
	jobs, err := newTestBumeran(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.details) != 0 {
		t.Errorf("bought %d bodies of covered postings", len(f.details))
	}
	for _, j := range jobs {
		if j.Description != "" || j.SeenRefresh {
			t.Errorf("covered posting = %+v, want body-less and not a refresh", j)
		}
	}
}

func TestBumeranExpiredPostingIsGone(t *testing.T) {
	f := &bumeranFake{expired: true}
	jobs, err := newTestBumeran(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %+v, want nothing: the hydrated postings expired, the other one 404s", jobs)
	}
}

func TestBumeranBudgetDefersTheRest(t *testing.T) {
	f := &bumeranFake{}
	s := newTestBumeran(f)
	s.maxNewPerRun = 0
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 || len(f.details) != 0 {
		t.Errorf("a zero budget yielded %d jobs and %d requests", len(jobs), len(f.details))
	}
}

func TestBumeranListingFailureFailsTheCrawl(t *testing.T) {
	f := &bumeranFake{listErr: errors.New("403")}
	if _, err := newTestBumeran(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a failed first listing page did not fail the crawl")
	}
}

// A listing cut short must not pass for a whole one: the company-scoped sweep would close the
// postings of the employers it saw on the pages it never read.
func TestBumeranLaterListingFailureFailsTheCrawl(t *testing.T) {
	f := &bumeranFake{laterErr: errors.New("502")}
	s := newTestBumeran(f)
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a failed second listing page did not fail the crawl")
	}
}

func TestBumeranSalary(t *testing.T) {
	month := &struct {
		Name string `json:"nombre"`
	}{Name: "por Mes"}
	p := func(n int) *int { return &n }
	cases := []struct {
		in     *bumeranSalaryField
		lo, hi int
		ok     bool
	}{
		{&bumeranSalaryField{Min: p(1200000), Max: p(1300000), Period: month}, 1200000, 1300000, true},
		{&bumeranSalaryField{Min: p(1), Max: p(2500000), Period: month}, 0, 0, false},
		{&bumeranSalaryField{Min: p(11111), Max: p(111111), Period: month}, 0, 0, false},
		{&bumeranSalaryField{Min: nil, Max: nil, Period: nil}, 0, 0, false},
		{&bumeranSalaryField{Min: p(1500000), Max: p(1000000), Period: month}, 0, 0, false},
		{nil, 0, 0, false},
	}
	for i, c := range cases {
		lo, hi, ok := bumeranSalary(c.in)
		if lo != c.lo || hi != c.hi || ok != c.ok {
			t.Errorf("case %d: got %d-%d %v, want %d-%d %v", i, lo, hi, ok, c.lo, c.hi, c.ok)
		}
	}
}

func TestBumeranWorkModeIgnoresTheFormDefault(t *testing.T) {
	for in, want := range map[string]string{"Presencial": "", "Remoto": "remote", "Híbrido": "hybrid", "": ""} {
		if got := bumeranWorkMode(in); got != want {
			t.Errorf("bumeranWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	if !bumeranTechSubarea(19, 2621) || bumeranTechSubarea(19, 107) || bumeranTechSubarea(1, 90) {
		t.Error("tech subarea mapping is off")
	}
}
