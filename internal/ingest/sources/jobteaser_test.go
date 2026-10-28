package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// jobteaserFake serves fixtures cut from the real sitemaps and pages (fetched 2026-10-28; the
// sitemaps trimmed to a handful of entries, three of whose lastmods were set by hand to order them, the pages to their JobPosting and fact lines). The
// job-ads sitemap names four French postings — KPMG France, VINCI Energies (a CDI with no
// remote work), Yzzy (a CDD, partly remote, with a monthly wage) and Capgemini, whose page answers
// 404 — and one German posting. The companies sitemap names kpmg-france, vinci and
// vinci-energies in French.
type jobteaserFake struct {
	sitemapErr   map[string]error
	noJobPosting bool

	mu       sync.Mutex
	sitemaps []string
	pages    []string
}

func (f *jobteaserFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.sitemaps = append(f.sitemaps, u)
	f.mu.Unlock()
	name := u[strings.LastIndex(u, "/")+1:]
	if err := f.sitemapErr[name]; err != nil {
		return err
	}
	raw, err := os.ReadFile("testdata/jobteaser/" + name)
	if err != nil {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return xml.Unmarshal(raw, v)
}

func (f *jobteaserFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.noJobPosting {
		return html.Parse(strings.NewReader("<html><body>Just a moment...</body></html>"))
	}
	var path string
	switch {
	case strings.Contains(u, "/04617e96-"):
		path = "testdata/jobteaser/posting_cdi.html"
	case strings.Contains(u, "/30878010-"):
		path = "testdata/jobteaser/posting_cdd.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestJobTeaser(f *jobteaserFake) jobteaser {
	return jobteaser{
		sitemap:        f,
		pages:          f,
		limiter:        noWait{},
		freshPerRun:    jobteaserFreshPerRun,
		backlogPerRun:  jobteaserBacklogPerRun,
		deadlineMargin: jobteaserDeadlineMargin,
		now:            time.Now,
	}
}

var jobteaserFR = CompanyEntry{Provider: "jobteaser", Board: "fr"}

func TestJobTeaserHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &jobteaserFake{}
	jobs, err := newTestJobTeaser(f).FetchNew(context.Background(), jobteaserFR, func(id string) bool {
		return id == "9ebe5553-a4d1-44a4-bda3-0202e1f041a8"
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want two hydrated and one refreshed (one 404, one German): %+v", len(jobs), jobs)
	}
	if r := byID["9ebe5553-a4d1-44a4-bda3-0202e1f041a8"]; !r.SeenRefresh || r.Company != "kpmg-france" {
		t.Errorf("seen posting = %+v, want a SeenRefresh under the resolved employer", r)
	}
	if _, ok := byID["a5cad15c-c19f-4ecd-90b2-6ac1ce15241a"]; ok {
		t.Error("a German-language posting was read on the French board")
	}

	v := byID["04617e96-3b49-451a-8763-d7f324e0562c"]
	if v.Title != "Technicien frigoriste expérimenté F/H" || v.Company != "VINCI Energies" {
		t.Errorf("title/company = %q / %q", v.Title, v.Company)
	}
	if v.WorkMode != "onsite" || v.EmploymentType != "full_time" {
		t.Errorf("work mode / employment = %q / %q, want onsite / full_time", v.WorkMode, v.EmploymentType)
	}
	if len(v.Countries) != 1 || v.Countries[0] != "fr" || v.Location == "" {
		t.Errorf("location = %q countries = %v", v.Location, v.Countries)
	}
	if v.Description == "" || v.PostedAt == nil {
		t.Errorf("body or posted-at missing: %+v", v)
	}

	y := byID["30878010-9e4e-4894-a9ba-0548ca3b0f95"]
	if y.WorkMode != "hybrid" || y.Remote || y.EmploymentType != "contract" {
		t.Errorf("work mode / employment = %q / %q, want hybrid / contract", y.WorkMode, y.EmploymentType)
	}
	if y.SalaryMin == nil || *y.SalaryMin != 750 || *y.SalaryMax != 1500 || y.SalaryCurrency != "EUR" || y.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q %q, want 750-1500 EUR month", y.SalaryMin, y.SalaryMax, y.SalaryCurrency, y.SalaryPeriod)
	}
	if len(f.pages) != 3 {
		t.Errorf("page requests = %v, want the three unseen French postings", f.pages)
	}
}

func TestJobTeaserGateSkipsCoveredEmployersWithoutARequest(t *testing.T) {
	f := &jobteaserFake{}
	var asked []string
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		asked = cs
		return map[string]bool{"vinci-energies": true}
	}}
	jobs, err := newTestJobTeaser(f).FetchNewGated(context.Background(), jobteaserFR, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "vinci-energies,kpmg-france" && strings.Join(asked, ",") != "kpmg-france,vinci-energies" {
		t.Errorf("gate asked about %v, want only the employers the companies sitemap resolves", asked)
	}
	for _, p := range f.pages {
		if strings.Contains(p, "/04617e96-") {
			t.Error("bought the page of a covered employer's posting")
		}
	}
	for _, j := range jobs {
		if j.ExternalID == "04617e96-3b49-451a-8763-d7f324e0562c" && (j.Description != "" || j.SeenRefresh || j.Company != "vinci-energies") {
			t.Errorf("covered posting = %+v, want body-less under its resolved employer", j)
		}
	}
}

func TestJobTeaserResolvesTheLongestCompanySlug(t *testing.T) {
	companies := map[string]bool{"vinci": true, "vinci-energies": true}
	p := jobteaserPosting{slug: "vinci-energies-technicien-frigoriste-experimente-f-h"}
	p.resolve(companies)
	if p.employer != "vinci-energies" || p.title != "technicien frigoriste experimente f h" {
		t.Errorf("resolved %q / %q", p.employer, p.title)
	}
	q := jobteaserPosting{slug: "yzzy-alternance-charge-e-rh"}
	q.resolve(companies)
	if q.employer != "" || q.title != "yzzy alternance charge e rh" {
		t.Errorf("unresolved %q / %q", q.employer, q.title)
	}
}

// A page with no JobPosting is a challenge, not a gone posting: a crawl that read nothing else
// must fail rather than report a quiet board.
func TestJobTeaserChallengePagesFailTheCrawl(t *testing.T) {
	f := &jobteaserFake{noJobPosting: true}
	if _, err := newTestJobTeaser(f).FetchNew(context.Background(), jobteaserFR, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no page did not fail")
	}
}

func TestJobTeaserSitemapFailures(t *testing.T) {
	f := &jobteaserFake{sitemapErr: map[string]error{"job_ads_sitemap.xml": errors.New("502")}}
	if _, err := newTestJobTeaser(f).FetchNew(context.Background(), jobteaserFR, func(string) bool { return true }); err == nil {
		t.Error("a failed job-ads sitemap did not fail the crawl")
	}
	f = &jobteaserFake{sitemapErr: map[string]error{"companies_sitemap.xml": errors.New("502")}}
	jobs, err := newTestJobTeaser(f).FetchNew(context.Background(), jobteaserFR, func(string) bool { return true })
	if err != nil || len(jobs) != 4 {
		t.Errorf("a failed companies sitemap cost the crawl: %d jobs, err %v", len(jobs), err)
	}
	if _, err := newTestJobTeaser(&jobteaserFake{}).FetchNew(context.Background(), CompanyEntry{Board: "france"}, func(string) bool { return true }); err == nil {
		t.Error("a board that is not a language was accepted")
	}
}

func TestJobTeaserFactMappings(t *testing.T) {
	for in, want := range map[string]string{"Partiel": "hybrid", "Pas de télétravail": "onsite", "Non spécifié": "", "Total": "remote", "": ""} {
		if got := jobteaserWorkMode(in); got != want {
			t.Errorf("jobteaserWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"CDI": "full_time", "CDD 12 à 24 mois": "contract", "Stage 6 mois": "internship", "Alternance": "internship", "VIE / VIA": "contract", "": ""} {
		if got := jobteaserEmploymentType(in); got != want {
			t.Errorf("jobteaserEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
