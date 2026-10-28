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

// cadremploiFake serves the real fixtures (fetched 2026-10-28): the sitemap's first page trimmed
// to three postings and its second to one; the first posting's page shows the expired notice,
// the second carries a JobPosting (Expectra, a stated location, no salary), and every other
// posting answers 404, as an unknown id does. The third sitemap page answers 404, as the page
// past the last does.
type cadremploiFake struct {
	// refuseFirst answers 403 to the first request of every URL, as the edge does to one page in
	// six; the retry must clear it.
	refuseFirst bool
	// sitemapErr fails every request for the given sitemap page.
	sitemapErr   map[int]error
	noJobPosting bool
	// allChallenged answers every posting page with a challenge, expired ones included.
	allChallenged bool

	mu       sync.Mutex
	asked    map[string]int
	sitemaps []string
	pages    []string
}

func (f *cadremploiFake) refused(u string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[u]++
	return f.refuseFirst && f.asked[u] == 1
}

func (f *cadremploiFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.sitemaps = append(f.sitemaps, u)
	f.mu.Unlock()
	if f.refused(u) {
		return &StatusError{Method: "GET", Code: 403, URL: u}
	}
	for page, err := range f.sitemapErr {
		if strings.HasSuffix(u, "-"+string(rune('0'+page))+".xml") {
			return err
		}
	}
	var path string
	switch {
	case strings.HasSuffix(u, "-1.xml"):
		path = "testdata/cadremploi/sitemap_1.xml"
	case strings.HasSuffix(u, "-2.xml"):
		path = "testdata/cadremploi/sitemap_2.xml"
	default:
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *cadremploiFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.refused(u) {
		return nil, &StatusError{Method: "GET", Code: 403, URL: u}
	}
	if f.allChallenged {
		return html.Parse(strings.NewReader("<html><body>Just a moment...</body></html>"))
	}
	var path string
	switch {
	case strings.HasSuffix(u, "=156588566185950368"):
		path = "testdata/cadremploi/posting_expired.html"
	case strings.HasSuffix(u, "=156588566186949459"):
		path = "testdata/cadremploi/posting_156588566186949459.html"
		if f.noJobPosting {
			return html.Parse(strings.NewReader("<html><body>Just a moment...</body></html>"))
		}
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestCadremploi(f *cadremploiFake) cadremploi {
	return cadremploi{
		http:           f,
		limiter:        noWait{},
		freshPerRun:    cadremploiFreshPerRun,
		backlogPerRun:  cadremploiBacklogPerRun,
		deadlineMargin: cadremploiDeadlineMargin,
		retryBackoff:   time.Millisecond,
		now:            time.Now,
	}
}

func TestCadremploiHydratesNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &cadremploiFake{}
	jobs, err := newTestCadremploi(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "156271694951931808" })
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want one hydrated and one refreshed (one expired, one 404): %+v", len(jobs), jobs)
	}
	if r := byID["156271694951931808"]; !r.SeenRefresh || r.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", r)
	}
	if _, ok := byID["156588566185950368"]; ok {
		t.Error("an expired posting was yielded")
	}
	j := byID["156588566186949459"]
	if j.Title != "CONDUCTEUR DE TRAVAUX MCO - SECTEUR NAVAL (F/H)" || j.Company != "Expectra" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != cadremploiBaseURL+"/emploi/detail_offre?offreId=156588566186949459" {
		t.Errorf("url = %q", j.URL)
	}
	if !strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "Randstad professional") {
		t.Errorf("plain-text description not rebuilt as paragraphs: %.200q", j.Description)
	}
	if !strings.Contains(j.Location, "Toulon") || len(j.Countries) != 1 || j.Countries[0] != "fr" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02") != "2026-10-28" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if j.EmploymentType != "" || j.SalaryMin != nil {
		t.Errorf("employment %q / salary %v from a posting stating OTHER and no pay", j.EmploymentType, j.SalaryMin)
	}
	if len(f.sitemaps) != 3 {
		t.Errorf("sitemap requests = %v, want pages 1 and 2 and the 404 past the end", f.sitemaps)
	}
}

func TestCadremploiRetriesARefusal(t *testing.T) {
	f := &cadremploiFake{refuseFirst: true}
	jobs, err := newTestCadremploi(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	hydrated := 0
	for _, j := range jobs {
		if j.Description != "" {
			hydrated++
		}
	}
	if hydrated != 1 {
		t.Errorf("hydrated %d postings through the refusals, want 1", hydrated)
	}
}

// A page with no JobPosting and no expired notice is a challenge, not a gone posting: it is no
// answer, and the posting is not yielded.
func TestCadremploiPageWithoutPostingIsNoAnswer(t *testing.T) {
	f := &cadremploiFake{noJobPosting: true}
	s := newTestCadremploi(f)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID == "156588566186949459" {
			t.Errorf("a challenge page was read as a posting: %+v", j)
		}
	}
	if _, _, err := s.detail(context.Background(), cadremploiPosting{id: "156588566186949459", url: cadremploiBaseURL + "/emploi/detail_offre?offreId=156588566186949459"}); !errors.Is(err, errCadremploiNoPosting) {
		t.Errorf("detail error = %v, want errCadremploiNoPosting", err)
	}
}

func TestCadremploiSitemapFailureFailsTheCrawl(t *testing.T) {
	for _, page := range []int{1, 2} {
		f := &cadremploiFake{sitemapErr: map[int]error{page: errors.New("502 bad gateway")}}
		if _, err := newTestCadremploi(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
			t.Errorf("a failed sitemap page %d did not fail the crawl", page)
		}
	}
}

func TestCadremploiParsePosting(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.cadremploi.fr/emploi/detail_offre?offreId=156588566186949459": "156588566186949459",
		"https://www.cadremploi.fr/emploi/detail_offre?offreId=abc":                "",
		"https://evil.example/emploi/detail_offre?offreId=156588566186949459":      "",
		"https://www.cadremploi.fr/emploi/liste_offres?offreId=156588566186949459": "",
	} {
		p, ok := cadremploiParsePosting(sitemapLoc{Loc: raw})
		if ok != (want != "") || p.id != want {
			t.Errorf("cadremploiParsePosting(%q) = %q %v, want %q", raw, p.id, ok, want)
		}
	}
}

func TestCadremploiSalaryAndBody(t *testing.T) {
	var ld cadremploiLD
	ld.Title, ld.HiringOrganization.Name = "Chef de Mission Comptable H/F", "CABEXPERTS RECRUTEMENT"
	ld.Description = "<p>Au sein d'un cabinet</p>"
	ld.Qualifications = "De formation supérieure, BAC + 5.\nAutonomie."
	ld.BaseSalary.Currency, ld.BaseSalary.Value.UnitText = "EUR", "YEAR"
	ld.BaseSalary.Value.MinValue, ld.BaseSalary.Value.MaxValue = 33000, 43000
	ld.EmploymentType = "CONTRACTOR"
	j, ok := cadremploiJob(ld, cadremploiPosting{id: "1"})
	if !ok {
		t.Fatal("posting rejected")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 33000 || *j.SalaryMax != 43000 || j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %q %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !strings.Contains(j.Description, "Profil recherché") || !strings.Contains(j.Description, "BAC + 5") {
		t.Errorf("description = %q", j.Description)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("employment = %q", j.EmploymentType)
	}
	ld.BaseSalary.Value.UnitText = ""
	if j, _ := cadremploiJob(ld, cadremploiPosting{id: "1"}); j.SalaryMin != nil {
		t.Error("salary set without a stated period")
	}
}

// A crawl whose every page is a challenge read nothing: it must fail rather than report a quiet
// board.
func TestCadremploiAllChallengedFailsTheCrawl(t *testing.T) {
	f := &cadremploiFake{allChallenged: true}
	if _, err := newTestCadremploi(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no page did not fail")
	}
}
