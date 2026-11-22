package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live Eploy responses fetched on 2026-10-15: careers.pei.group's
// /live-jobs.xml (its first three <url> entries verbatim), one PEI vacancy page and one
// HomeServe vacancy page (title, ld+json blocks, h1 and the vac-details__description block
// verbatim), and the page HomeServe answers for a vacancy its sitemap still names but the site
// has closed.

const (
	eployPEI442 = "https://peigroupweb.eploy.net/vacancies/442/senior-reporter--private-equity.html"
	eployPEI470 = "https://peigroupweb.eploy.net/vacancies/470/conference-producer.html"
	eployPEI482 = "https://peigroupweb.eploy.net/vacancies/482/account-executive--new-business.html"
)

func eployFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/eploy/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func eployParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(eployFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// eployFake serves fixed bodies by URL; an unknown URL is a 404.
type eployFake struct {
	pages map[string]string
	mu    sync.Mutex
	gets  []string
}

func (f *eployFake) GetXML(_ context.Context, u string, v any) error {
	body, ok := f.pages[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return xml.Unmarshal([]byte(body), v)
}

func (f *eployFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 410, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func TestEployListingsKeepOwnAndEployNetHostsOnly(t *testing.T) {
	doc := sitemapDoc{URLs: []sitemapLoc{
		{Loc: eployPEI442},
		{Loc: "https://careers.pei.group/vacancies/500/x.html"},
		{Loc: "https://careers.pei.group/vacancies/500/x.html"}, // repeated
		{Loc: "https://evil.example/vacancies/1/x.html"},        // another host
		{Loc: "https://careers.pei.group/about-us.html"},        // not a vacancy
		{Loc: "http://careers.pei.group/vacancies/7/x.html"},    // not https
	}}
	got := eployListings(doc, "careers.pei.group")
	if len(got) != 2 || got[0].ID != "442" || got[1].ID != "500" {
		t.Fatalf("listings = %+v", got)
	}
}

func TestEployVacancyWithJobPosting(t *testing.T) {
	job, ok := parseEployVacancy(eployParse(t, "pei_vacancy.html"), eployListing{ID: "442", URL: eployPEI442})
	if !ok {
		t.Fatal("vacancy not read")
	}
	if job.Title != "Senior Reporter - Private Equity" || job.Location != "London, United Kingdom" {
		t.Errorf("job = %q / %q", job.Title, job.Location)
	}
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-09-14" {
		t.Errorf("posted = %v", job.PostedAt)
	}
	if !strings.Contains(job.Description, "Senior Private Equity Reporter") {
		t.Errorf("description = %.200s", job.Description)
	}
	// "Permanent" states tenure, not hours.
	if job.EmploymentType != "" {
		t.Errorf("employment type = %q", job.EmploymentType)
	}
}

func TestEployVacancyWithoutJobPostingStillReads(t *testing.T) {
	job, ok := parseEployVacancy(eployParse(t, "homeserve_vacancy.html"), eployListing{ID: "5348"})
	if !ok {
		t.Fatal("a template without ld+json was not read")
	}
	if job.Title != "Plumbing Engineer - Croydon" || !strings.Contains(job.Description, "Plumbing Engineer") {
		t.Errorf("job = %q %.120s", job.Title, job.Description)
	}
	if job.PostedAt != nil || job.Location != "" {
		t.Errorf("a fact was read from nowhere: %+v", job)
	}
	if _, ok := parseEployVacancy(eployParse(t, "homeserve_not_found.html"), eployListing{ID: "5495"}); ok {
		t.Error("the vacancy-not-found page was read as a vacancy")
	}
}

func TestEployFetchHydratesOnlyNewVacancies(t *testing.T) {
	f := &eployFake{pages: map[string]string{
		"https://careers.pei.group/live-jobs.xml": eployFixture(t, "pei_live_jobs.xml"),
		eployPEI442: eployFixture(t, "pei_vacancy.html"),
		// eployPEI482 answers 410: the sitemap lags the site.
	}}
	seen := func(id string) bool { return id == "470" }
	jobs, err := NewEploy(f).(eploy).FetchNew(context.Background(), CompanyEntry{Provider: "eploy", Board: "careers.pei.group", Company: "PEI Group"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	if j := jobs[0]; j.ExternalID != "442" || j.URL != eployPEI442 || j.Company != "PEI Group" || j.Description == "" {
		t.Errorf("hydrated = %+v", j)
	}
	if j := jobs[1]; j.ExternalID != "470" || !j.SeenRefresh {
		t.Errorf("seen = %+v", j)
	}
	for _, g := range f.gets {
		if g == eployPEI470 {
			t.Error("a seen vacancy's page was requested")
		}
	}
}

func TestEployFetchFails(t *testing.T) {
	ctx := context.Background()
	e := CompanyEntry{Board: "careers.pei.group", Company: "PEI Group"}
	// No sitemap.
	if _, err := NewEploy(&eployFake{pages: map[string]string{}}).Fetch(ctx, e); err == nil {
		t.Error("a missing sitemap succeeded")
	}
	// A sitemap index would split the inventory.
	index := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://careers.pei.group/a.xml</loc></sitemap></sitemapindex>`
	if _, err := NewEploy(&eployFake{pages: map[string]string{"https://careers.pei.group/live-jobs.xml": index}}).Fetch(ctx, e); err == nil {
		t.Error("a sitemap index succeeded")
	}
	// Every new vacancy unreadable.
	only := map[string]string{"https://careers.pei.group/live-jobs.xml": eployFixture(t, "pei_live_jobs.xml")}
	if _, err := NewEploy(&eployFake{pages: only}).Fetch(ctx, e); err == nil {
		t.Error("a crawl that read no vacancy page succeeded")
	}
	if _, err := NewEploy(&eployFake{}).Fetch(ctx, CompanyEntry{Board: "https://careers.pei.group/", Company: "x"}); err == nil {
		t.Error("a URL was accepted as a board")
	}
}

func TestEployIsFullBoardListing(t *testing.T) {
	if _, ok := NewEploy(nil).(fullBoardListing); !ok {
		t.Fatal("eploy must claim fullBoardListing")
	}
}
