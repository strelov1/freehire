package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// freeworkFake serves the real fixtures (fetched 2026-10-28): one listing page trimmed to four
// postings — R&S Telecom (salary stated with no currency), ODHCOM (a mission with no pay) and two
// Avanda postings in Geneva — and the posting resources of the first two. Any other posting
// answers 404, as a gone one does.
type freeworkFake struct {
	listErr  error
	laterErr error
	// lastPage drops the fixture's "next" link, so page 1 is the whole listing.
	lastPage bool
	// unpublished makes every posting resource answer a status other than "published".
	unpublished bool

	mu      sync.Mutex
	pages   []string
	details []string
}

func (f *freeworkFake) GetJSON(_ context.Context, u string, v any) error {
	if strings.Contains(u, "/job_postings?") {
		f.mu.Lock()
		f.pages = append(f.pages, u)
		f.mu.Unlock()
		if f.listErr != nil {
			return f.listErr
		}
		if !strings.Contains(u, "?page=1&") {
			if f.laterErr != nil {
				return f.laterErr
			}
			return json.Unmarshal([]byte(`{"hydra:totalItems":4,"hydra:member":[],"hydra:view":{}}`), v)
		}
		if err := freeworkFixture("testdata/freework/list.json", v); err != nil {
			return err
		}
		if f.lastPage {
			v.(*freeworkPage).View.Next = ""
		}
		return nil
	}
	f.mu.Lock()
	f.details = append(f.details, u)
	f.mu.Unlock()
	slug := u[strings.LastIndex(u, "/")+1:]
	if slug != "developpeur-data-stage-python-java" && slug != "developpeur-fullstack-devops-7" {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	if err := freeworkFixture("testdata/freework/detail_"+slug+".json", v); err != nil {
		return err
	}
	if f.unpublished {
		v.(*freeworkDetail).Status = "expired"
	}
	return nil
}

func freeworkFixture(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func newTestFreeWork(f *freeworkFake) freework {
	return freework{get: f, limiter: noWait{}, maxNewPerRun: freeworkMaxNewPerRun, maxPages: freeworkMaxPages, deadlineMargin: freeworkDeadlineMargin}
}

func TestFreeWorkHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &freeworkFake{}
	jobs, err := newTestFreeWork(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "683665" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want two hydrated and one refreshed (the fourth 404s): %+v", len(jobs), jobs)
	}
	if r := byID["683665"]; !r.SeenRefresh || r.Description != "" || r.Company != "Avanda" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	if _, ok := byID["683664"]; ok {
		t.Error("a posting whose resource 404s was yielded")
	}

	j := byID["687964"]
	if j.Title != "Développeur Data Stage / Python-Java" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Company != "R&S TELECOM" {
		t.Errorf("company = %q", j.Company)
	}
	// This posting's description already carries its profile, which must not be appended twice.
	if strings.Count(j.Description, "<h2>Profil recherché</h2>") != 0 || !strings.Contains(j.Description, "<") {
		t.Errorf("description repeated the profile or lost its markup: %.200q", j.Description)
	}
	if j.URL != freeworkSite+"developpeur-java-kotlin-groovy-scala/developpeur-data-stage-python-java" {
		t.Errorf("url = %q", j.URL)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "fr" || j.Location == "" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("work mode = %q remote = %v, want hybrid", j.WorkMode, j.Remote)
	}
	if j.Seniority != "senior" {
		t.Errorf("seniority = %q", j.Seniority)
	}
	if j.EmploymentType != "" {
		t.Errorf("employment type = %q from a posting offering both a mission and a permanent role", j.EmploymentType)
	}
	if j.SalaryMin != nil {
		t.Errorf("salary set from a posting that states no currency: %v", *j.SalaryMin)
	}
	if j.PostedAt == nil {
		t.Error("posted at missing")
	}

	o := byID["663470"]
	if o.EmploymentType != "contract" || o.Seniority != "middle" || o.SalaryMin != nil {
		t.Errorf("mission = %q %q salary %v", o.EmploymentType, o.Seniority, o.SalaryMin)
	}
	if len(f.details) != 3 {
		t.Errorf("posting requests = %v, want the three unseen postings", f.details)
	}
	for _, p := range f.pages {
		q, err := url.Parse(p)
		if err != nil || q.Query().Get("itemsPerPage") != "300" || len(q.Query()["properties[]"]) == 0 {
			t.Errorf("listing request %q does not ask for the trimmed 300-posting page", p)
		}
	}
}

func TestFreeWorkGateSkipsCoveredPostingsWithoutARequest(t *testing.T) {
	f := &freeworkFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[c] = true
		}
		return m
	}}
	jobs, err := newTestFreeWork(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.details) != 0 {
		t.Errorf("bought %d bodies of covered postings", len(f.details))
	}
	if len(jobs) != 4 {
		t.Errorf("got %d covered postings, want all four yielded body-less", len(jobs))
	}
	for _, j := range jobs {
		if j.Description != "" || j.SeenRefresh {
			t.Errorf("covered posting = %+v, want body-less and not a refresh", j)
		}
	}
}

func TestFreeWorkUnpublishedPostingIsGone(t *testing.T) {
	f := &freeworkFake{unpublished: true}
	jobs, err := newTestFreeWork(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %+v, want nothing: two postings are unpublished, the others 404", jobs)
	}
}

func TestFreeWorkBudgetDefersTheRest(t *testing.T) {
	f := &freeworkFake{}
	s := newTestFreeWork(f)
	s.maxNewPerRun = 0
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 || len(f.details) != 0 {
		t.Errorf("a zero budget yielded %d jobs and %d requests", len(jobs), len(f.details))
	}
}

func TestFreeWorkStopsAtTheLastPage(t *testing.T) {
	f := &freeworkFake{lastPage: true, laterErr: errors.New("must not be asked")}
	if _, err := newTestFreeWork(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 1 {
		t.Errorf("listing requests = %v, want one: the page carried no next link", f.pages)
	}
}

func TestFreeWorkListingFailureFailsTheCrawl(t *testing.T) {
	f := &freeworkFake{listErr: errors.New("503")}
	if _, err := newTestFreeWork(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a failed first listing page did not fail the crawl")
	}
}

// A listing cut short must not pass for a whole one: the company-scoped sweep would close the
// postings of the employers it saw on the pages it never read.
func TestFreeWorkLaterListingFailureFailsTheCrawl(t *testing.T) {
	f := &freeworkFake{laterErr: errors.New("502")}
	if _, err := newTestFreeWork(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a failed second listing page did not fail the crawl")
	}
}

func TestFreeWorkAppendsAProfileTheDescriptionLacks(t *testing.T) {
	d := freeworkDetail{Description: "<p>Missions</p>", CandidateProfile: "<p>Java, 5 ans</p>"}
	d.Title, d.Company.Name = "Développeur Java", "CELAD"
	j, ok := freeworkJob(d, freeworkCard{ID: 1, Slug: "dev-java"})
	if !ok || !strings.Contains(j.Description, "Profil recherché") || !strings.Contains(j.Description, "Java, 5 ans") {
		t.Errorf("profile not appended: %q", j.Description)
	}
}

func TestFreeWorkSalary(t *testing.T) {
	p := func(n int) *int { return &n }
	eur := "EUR"
	gbp := "gbp"
	cases := []struct {
		d      freeworkDetail
		lo, hi int
		cur    string
		period string
		ok     bool
	}{
		{freeworkDetail{MinAnnualSalary: p(50000), MaxAnnualSalary: p(60000), Currency: &eur}, 50000, 60000, "EUR", "year", true},
		{freeworkDetail{MinDailySalary: p(450), MaxDailySalary: p(550), Currency: &eur}, 450, 550, "EUR", "day", true},
		{freeworkDetail{MinDailySalary: p(500), Currency: &gbp}, 500, 500, "GBP", "day", true},
		{freeworkDetail{MinAnnualSalary: p(40000), MaxAnnualSalary: p(48000)}, 0, 0, "", "", false},
		{freeworkDetail{MinAnnualSalary: p(60000), MaxAnnualSalary: p(50000), Currency: &eur}, 0, 0, "", "", false},
		{freeworkDetail{Currency: &eur}, 0, 0, "", "", false},
	}
	for i, c := range cases {
		lo, hi, cur, period, ok := freeworkSalary(c.d)
		if lo != c.lo || hi != c.hi || cur != c.cur || period != c.period || ok != c.ok {
			t.Errorf("case %d: got %d-%d %q %q %v, want %d-%d %q %q %v", i, lo, hi, cur, period, ok, c.lo, c.hi, c.cur, c.period, c.ok)
		}
	}
}

func TestFreeWorkMappings(t *testing.T) {
	for in, want := range map[string]string{"full": "remote", "partial": "hybrid", "none": "onsite", "": ""} {
		if got := freeworkWorkMode(in); got != want {
			t.Errorf("freeworkWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"junior": "junior", "intermediate": "middle", "senior": "senior", "expert": "senior", "": ""} {
		if got := freeworkSeniority(in); got != want {
			t.Errorf("freeworkSeniority(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"permanent"}, "full_time"},
		{[]string{"contractor", "fixed-term"}, "contract"},
		{[]string{"internship"}, "internship"},
		{[]string{"contractor", "permanent"}, ""},
		{nil, ""},
	} {
		if got := freeworkEmploymentType(c.in); got != c.want {
			t.Errorf("freeworkEmploymentType(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
