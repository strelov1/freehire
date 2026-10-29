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

// taleezFake serves the real fixtures (fetched 2026-10-28): the job sitemap trimmed to three
// postings plus an off-host URL, and one posting page trimmed to its JobPosting. A posting with
// no page fixture answers 404, as a removed posting does.
type taleezFake struct {
	pageErr error

	mu    sync.Mutex
	pages []string
}

const taleezPageToken = "senior-consultant-in-operational-research-paris-artelys"

func (f *taleezFake) GetXML(_ context.Context, u string, v any) error {
	if u != taleezSitemapURL {
		return fmt.Errorf("taleezFake: no sitemap %s", u)
	}
	raw, err := os.ReadFile("testdata/taleez/sitemap.xml")
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}

func (f *taleezFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	if !strings.HasSuffix(u, "/apply/"+taleezPageToken) {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile("testdata/taleez/posting.html")
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestTaleez(f *taleezFake) taleez {
	s := NewTaleez(f, f).(taleez)
	s.limiter = noWait{}
	return s
}

func TestTaleezHydratesNewPostingsAndRefreshesSeen(t *testing.T) {
	f := &taleezFake{}
	jobs, err := newTestTaleez(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "fmudc" })
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
	if r := byID["fmudc"]; !r.SeenRefresh || r.URL != "https://taleez.com/apply/fmudc" {
		t.Errorf("seen posting = %+v, want a SeenRefresh at its URL", r)
	}
	j := byID[taleezPageToken]
	if j.Title != "Senior Consultant in Operational Research" || j.Company != "Artelys" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Paris" || len(j.Countries) != 1 || j.Countries[0] != "fr" {
		t.Errorf("location = %q countries = %v, want Paris / [fr]", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2025-09-04T11:41:27Z" {
		t.Errorf("posted at = %v, want 2025-09-04T11:41:27Z", j.PostedAt)
	}
	if !strings.Contains(j.Description, "optimization skills") || !strings.Contains(j.Description, "Qualifications") ||
		!strings.Contains(j.Description, "engineering degree") {
		t.Errorf("description lacks the description or the qualifications: %q", j.Description)
	}
	for _, u := range f.pages {
		if !strings.HasPrefix(u, "https://taleez.com/apply/") {
			t.Errorf("fetched %q, outside taleez.com/apply", u)
		}
		if strings.HasSuffix(u, "/fmudc") {
			t.Errorf("fetched the page of a seen posting: %q", u)
		}
	}
}

func TestTaleezReadsNewestFirst(t *testing.T) {
	postings, err := newTestTaleez(&taleezFake{}).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var tokens []string
	for _, p := range postings {
		tokens = append(tokens, p.token)
	}
	want := []string{"medecin-coordonnateur-ehpad-h-f-epernay-reseau-talents-cdi--", taleezPageToken, "fmudc"}
	if strings.Join(tokens, ",") != strings.Join(want, ",") {
		t.Errorf("tokens = %v, want %v (reversed sitemap order, off-host entry dropped)", tokens, want)
	}
}

func TestTaleezFailsWhenNoPageAnswers(t *testing.T) {
	f := &taleezFake{pageErr: &StatusError{Method: "GET", Code: 503, URL: "x"}}
	if _, err := newTestTaleez(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when every requested page fails")
	}
}

func TestTaleezDefersPastDeadline(t *testing.T) {
	f := &taleezFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestTaleez(f).FetchNew(ctx, CompanyEntry{}, func(id string) bool { return id == "fmudc" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh || len(f.pages) != 0 {
		t.Errorf("jobs = %+v pages = %v, want only the refresh and no page read inside the margin", jobs, f.pages)
	}
}

func TestTaleezParsePostingPinsHost(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://taleez.com/apply/fmudc":              true,
		"http://taleez.com/apply/fmudc":               false,
		"https://evil.example.com/apply/fmudc":        false,
		"https://taleez.com/careers/artelys":          false,
		"https://taleez.com/apply/fmudc?utm_source=x": false,
		"https://taleez.com/apply/a/b":                false,
	} {
		if _, got := taleezParsePosting(raw); got != ok {
			t.Errorf("taleezParsePosting(%q) ok = %v, want %v", raw, got, ok)
		}
	}
}

func TestTaleezNoPostingIsNoAnswer(t *testing.T) {
	_, _, err := (taleez{pages: emptyPageGetter{}, limiter: noWait{}}).detail(context.Background(),
		taleezPosting{token: "x", url: "https://taleez.com/apply/x"})
	if !errors.Is(err, errTaleezNoPosting) {
		t.Errorf("err = %v, want errTaleezNoPosting", err)
	}
}

type emptyPageGetter struct{}

func (emptyPageGetter) GetHTML(context.Context, string) (*html.Node, error) {
	return html.Parse(strings.NewReader("<html><body>challenge</body></html>"))
}
