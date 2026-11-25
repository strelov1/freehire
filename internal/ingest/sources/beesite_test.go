package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live BeeSite responses fetched on 2026-10-15: the first three
// records of Porsche's search and the first of Mercedes-Benz's (verbatim, image/contact fields
// dropped, SearchResultCountAll rewritten to the records kept), and two jobs.porsche.com posting
// pages (ld+json blocks and the jobad-dynamic-html-container verbatim) — one whose JobPosting
// block opens with the template's stray "entryLevel" token, one with no JobPosting at all.

const (
	bsPorsche     = "porsche-beesite-production-gjb.app.beesite.de"
	bsPorschePage = "https://jobs.porsche.com/index.php?ac=jobad&id="
)

func bsFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/beesite/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bsFake answers every search URL with one fixed page and serves posting pages by URL.
type bsFake struct {
	search string
	pages  map[string]string
	// mu guards gets, which the detail workers append to concurrently.
	mu   sync.Mutex
	gets []string
}

func (f *bsFake) GetJSON(_ context.Context, u string, v any) error {
	if !strings.Contains(u, "/search?data=") {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return json.Unmarshal([]byte(f.search), v)
}

func (f *bsFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 500, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func TestBeeSiteSearchURL(t *testing.T) {
	u := beesiteSearchURL(bsPorsche, 101)
	if !strings.HasPrefix(u, "https://"+bsPorsche+"/search?data=") || !strings.Contains(u, "%22FirstItem%22%3A101") {
		t.Errorf("url = %s", u)
	}
}

func TestBeeSiteRecordWithBodyNeedsNoPage(t *testing.T) {
	f := &bsFake{search: bsFixture(t, "mercedes_search.json")}
	jobs, err := NewBeeSite(f).Fetch(context.Background(), CompanyEntry{Board: "mercedes-benz-beesite-production-gjb.app.beesite.de", Company: "Mercedes-Benz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || len(f.gets) != 0 {
		t.Fatalf("jobs=%d page requests=%d", len(jobs), len(f.gets))
	}
	j := jobs[0]
	if !strings.HasPrefix(j.URL, "https://jobs.mercedes-benz.com/") || !strings.Contains(j.Description, "Mercedes-Benz") {
		t.Errorf("job = %+v", j)
	}
	if j.Location != "Stuttgart, Germany" || len(j.Countries) != 1 || j.Countries[0] != "de" || j.PostedAt == nil {
		t.Errorf("place/date = %q %v %v", j.Location, j.Countries, j.PostedAt)
	}
	// "Internship" career level: an internship whatever its schedule.
	if j.EmploymentType != "internship" || j.Seniority != "intern" {
		t.Errorf("type = %q %q", j.EmploymentType, j.Seniority)
	}
}

func TestBeeSitePageDescription(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(bsFixture(t, "porsche_posting_ldjson.html")))
	desc := beesitePageDescription(root)
	if !strings.HasPrefix(desc, "<h2>Aufgaben</h2>") || strings.Contains(desc, "&lt;") {
		t.Errorf("ld+json body = %.200q", desc)
	}
	root, _ = html.Parse(strings.NewReader(bsFixture(t, "porsche_posting_no_ldjson.html")))
	if desc := beesitePageDescription(root); !strings.Contains(desc, "Interface person between Customer and Chassis software department") {
		t.Errorf("container body = %.300q", desc)
	}
	root, _ = html.Parse(strings.NewReader("<html><body><p>Diese Stellenanzeige ist nicht mehr verfügbar.</p></body></html>"))
	if desc := beesitePageDescription(root); desc != "" {
		t.Errorf("a page without a posting yielded %q", desc)
	}
}

func TestBeeSiteFetchHydratesOnlyNewPostings(t *testing.T) {
	f := &bsFake{search: bsFixture(t, "porsche_search.json"), pages: map[string]string{
		bsPorschePage + "17854": bsFixture(t, "porsche_posting_ldjson.html"),
		// 15853's page fails: deferred, not stored body-less.
	}}
	seen := func(id string) bool { return id == "16329" }
	jobs, err := NewBeeSite(f).(beesite).FetchNew(context.Background(), CompanyEntry{Provider: "beesite", Board: bsPorsche, Company: "Porsche"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	if j := jobs[0]; j.ExternalID != "17854" || j.URL != bsPorschePage+"17854" || j.Description == "" || j.EmploymentType != "full_time" {
		t.Errorf("hydrated = %+v", j)
	}
	if j := jobs[1]; j.ExternalID != "16329" || !j.SeenRefresh {
		t.Errorf("seen = %+v", j)
	}
	for _, g := range f.gets {
		if g == bsPorschePage+"16329" {
			t.Error("a seen posting's page was requested")
		}
	}
}

func TestBeeSiteFetchFails(t *testing.T) {
	ctx := context.Background()
	e := CompanyEntry{Board: bsPorsche, Company: "Porsche"}
	// The search states more postings than it serves.
	short := strings.Replace(bsFixture(t, "porsche_search.json"), `"SearchResultCountAll": 3`, `"SearchResultCountAll": 4`, 1)
	if _, err := NewBeeSite(&bsFake{search: short}).Fetch(ctx, e); err == nil {
		t.Error("a listing short of the stated total succeeded")
	}
	// Every new posting's page unreadable.
	if _, err := NewBeeSite(&bsFake{search: bsFixture(t, "porsche_search.json")}).Fetch(ctx, e); err == nil {
		t.Error("a crawl that read no posting page succeeded")
	}
	if _, err := NewBeeSite(&bsFake{}).Fetch(ctx, CompanyEntry{Board: "jobs.porsche.com", Company: "x"}); err == nil {
		t.Error("a non-beesite host was accepted as a board")
	}
}

func TestBeeSiteIsFullBoardListing(t *testing.T) {
	if _, ok := NewBeeSite(nil).(fullBoardListing); !ok {
		t.Fatal("beesite must claim fullBoardListing")
	}
}
