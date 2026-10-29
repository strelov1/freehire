package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// dualooFake serves the fixtures: a sitemap index naming one real customer sitemap (trimmed), a
// customer sitemap that fails and an off-host one, and one real posting page (fetched
// 2026-10-29) trimmed to its JobPosting. Any other posting answers 404, as a removed one does.
type dualooFake struct {
	mu       sync.Mutex
	sitemaps []string
	pages    []string
}

const dualooPageID = "06ea8f34-bc47-4226-abbd-df83a4b7a368"

func (f *dualooFake) GetXML(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.sitemaps = append(f.sitemaps, u)
	f.mu.Unlock()
	var file string
	switch u {
	case dualooSitemapIndex:
		file = "sitemap_index.xml"
	case "https://jobs.dualoo.com/sitemap_dottikon-es.xml":
		file = "sitemap_dottikon-es.xml"
	default:
		return &StatusError{Method: "GET", Code: 500, URL: u}
	}
	raw, err := os.ReadFile("testdata/dualoo/" + file)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *dualooFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if !strings.Contains(u, "/"+dualooPageID+"/") {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/dualoo/posting.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestDualoo(f *dualooFake) dualoo {
	s := NewDualoo(f, f).(dualoo)
	s.limiter = noWait{}
	return s
}

func TestDualooHydratesNewPostingsAndRefreshesSeen(t *testing.T) {
	f := &dualooFake{}
	seenID := "b7416d3c-a8ef-4a90-b4e8-40371367579b"
	jobs, err := newTestDualoo(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == seenID })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want the refreshed posting and the hydrated one (the 404 is gone)", jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if r := byID[seenID]; !r.SeenRefresh || r.URL != "https://jobs.dualoo.com/portal/cg0wbpxk/"+seenID+"/detail?lang=DE" {
		t.Errorf("seen posting = %+v, want a SeenRefresh at its portal URL", r)
	}
	j := byID[dualooPageID]
	if j.URL != "https://jobs.dualoo.com/portal/cg0wbpxk/"+dualooPageID+"/detail?lang=DE" {
		t.Errorf("url = %q, want the portal form over the bare link", j.URL)
	}
	if j.Title != "Chemikant als Meister-Assistent Produktion (m/w/d)" || j.Company != "Dottikon Exclusive Synthesis AG" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Dottikon, Aargau" || len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("location = %q countries = %v, want Dottikon, Aargau / [ch]", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2026-04-28T15:26:19Z" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	for _, part := range []string{"Veredelungschemikalien", "Ihre Verantwortung", "Ihr Profil", "Unser Umfeld"} {
		if !strings.Contains(j.Description, part) {
			t.Errorf("description lacks %q", part)
		}
	}
	for _, u := range f.pages {
		if strings.Contains(u, seenID) {
			t.Errorf("fetched the page of a seen posting: %q", u)
		}
	}
	for _, u := range f.sitemaps {
		if !strings.HasPrefix(u, "https://jobs.dualoo.com/") {
			t.Errorf("read sitemap %q, off the host", u)
		}
	}
}

func TestDualooListsNewestFirstAndCollapsesForms(t *testing.T) {
	postings, err := newTestDualoo(&dualooFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range postings {
		ids = append(ids, p.id)
	}
	want := []string{"550e3754-9156-43ed-8dfe-3dd89496366a", dualooPageID, "b7416d3c-a8ef-4a90-b4e8-40371367579b"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v (lastmod order, one entry per posting, portal page dropped)", ids, want)
	}
}

func TestDualooIndexFailureFailsTheCrawl(t *testing.T) {
	s := dualoo{sitemap: failingXML{}, limiter: noWait{}}
	if _, err := s.list(context.Background()); err == nil {
		t.Error("want an error when the sitemap index cannot be read")
	}
}

func TestDualooNoPostingIsNoAnswer(t *testing.T) {
	_, _, err := (dualoo{pages: emptyPageGetter{}, limiter: noWait{}}).detail(context.Background(),
		dualooPosting{id: "x", url: "https://jobs.dualoo.com/link/x/detail"})
	if !errors.Is(err, errDualooNoPosting) {
		t.Errorf("err = %v, want errDualooNoPosting", err)
	}
}

type failingXML struct{}

func (failingXML) GetXML(_ context.Context, u string, _ any) error {
	return fmt.Errorf("unreachable %s", u)
}

func TestDualooTestSitemap(t *testing.T) {
	for path, want := range map[string]bool{
		"/sitemap_michele-testing-ag.xml":               true,
		"/sitemap_smoketest-9-3-226-dualoo-ag.xml":      true,
		"/sitemap_25-1-26-dualoo-ag.xml":                true,
		"/sitemap_test-language-prod.xml":               true,
		"/sitemap_test-dualoo-ag1.xml":                  true,
		"/sitemap_dualoo-ag.xml":                        false,
		"/sitemap_testo-ag.xml":                         false,
		"/sitemap_zuercher-lehrbetriebsverband-ict.xml": false,
	} {
		if got := dualooTestSitemap.MatchString(path); got != want {
			t.Errorf("dualooTestSitemap(%q) = %v, want %v", path, got, want)
		}
	}
}
