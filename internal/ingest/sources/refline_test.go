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

// reflineFake serves the fixtures: robots.txt naming two sitemaps on the host (the second fails)
// and one off it, one sitemap trimmed to three jobs across six publications, and one real
// posting page (fetched 2026-10-29) trimmed to its JobPosting. Any other job answers 404.
type reflineFake struct {
	mu    sync.Mutex
	pages []string
}

func (f *reflineFake) GetText(_ context.Context, u string) (string, error) {
	if u != reflineRobotsURL {
		return "", &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/refline/robots.txt")
	return string(raw), err
}

func (f *reflineFake) GetXML(_ context.Context, u string, v any) error {
	if u != "https://apply.refline.ch/rec01/0/sitemap.xml" {
		return &StatusError{Method: "GET", Code: 502, URL: u}
	}
	raw, err := os.ReadFile("testdata/refline/sitemap.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *reflineFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if !strings.Contains(u, "/215876/2383/") {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/refline/posting.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestRefline(f *reflineFake) refline {
	s := NewRefline(f, f, f).(refline)
	s.limiter = noWait{}
	return s
}

func TestReflineHydratesNewJobsAndRefreshesSeen(t *testing.T) {
	f := &reflineFake{}
	jobs, err := newTestRefline(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "514915/2891" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v, want the refreshed job and the hydrated one (the 404 is gone)", jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if r := byID["514915/2891"]; !r.SeenRefresh || r.URL != "https://apply.refline.ch/514915/2891/pub/de/index.html" {
		t.Errorf("seen job = %+v, want a SeenRefresh at its de publication", r)
	}
	j := byID["215876/2383"]
	if j.URL != "https://apply.refline.ch/215876/2383/pub/8/index.html" {
		t.Errorf("url = %q, want the lowest channel publication", j.URL)
	}
	if j.Title != "Abacus Berater:in BPE, REST & AbaReport 80–100 %" || j.Company != "Abraxas Informatik AG" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "St. Gallen, SG" || len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("location = %q countries = %v, want St. Gallen, SG / [ch]", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2026-02-02T10:29:39Z" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Behörden") {
		t.Errorf("description = %q, want the decoded body", j.Description)
	}
}

func TestReflineListsOnePublicationPerJob(t *testing.T) {
	postings, err := newTestRefline(&reflineFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range postings {
		got = append(got, p.id+"@"+p.pub)
	}
	want := []string{"215876/2383@8", "514915/2891@de", "673276/2250@1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("postings = %v, want %v (newest lastmod first, preferred publication)", got, want)
	}
}

func TestReflineSitemapsReadFromRobots(t *testing.T) {
	raw, err := os.ReadFile("testdata/refline/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := reflineSitemaps(string(raw))
	want := []string{"https://apply.refline.ch/rec01/0/sitemap.xml", "https://apply.refline.ch/rec01/2/sitemap.xml"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sitemaps = %v, want %v", got, want)
	}
}

func TestReflinePreferPub(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"1", "6", true}, {"13", "8", false}, {"8", "de", true}, {"de", "1", false}, {"de", "it", true},
	} {
		if got := reflinePreferPub(tc.a, tc.b); got != tc.want {
			t.Errorf("reflinePreferPub(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReflineNoPostingIsNoAnswer(t *testing.T) {
	_, _, err := (refline{pages: emptyPageGetter{}, limiter: noWait{}}).detail(context.Background(),
		reflinePosting{id: "1/1", url: "https://apply.refline.ch/1/1/pub/1/index.html"})
	if !errors.Is(err, errReflineNoPosting) {
		t.Errorf("err = %v, want errReflineNoPosting", err)
	}
}
