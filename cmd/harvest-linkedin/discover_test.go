package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queries.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadQueries(t *testing.T) {
	path := writeTemp(t, `
queries:
  - keywords: golang
    location: Germany
    jobage: 30
    pages: 3
  - keywords: data engineer
    location: Poland
`)
	got, err := loadQueries(path)
	if err != nil {
		t.Fatalf("loadQueries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d queries, want 2", len(got))
	}
	if got[0].Pages != 3 || got[0].JobAge != 30 {
		t.Errorf("explicit values lost: %+v", got[0])
	}
	if got[1].Pages != defaultPages || got[1].JobAge != defaultJobAge {
		t.Errorf("defaults not applied: %+v, want pages=%d jobage=%d", got[1], defaultPages, defaultJobAge)
	}
}

func TestLoadQueriesRefusesAQueryWithoutAMarket(t *testing.T) {
	// An unbounded market is what turns a bounded harvest into an unbounded crawl.
	path := writeTemp(t, "queries:\n  - keywords: golang\n")
	if _, err := loadQueries(path); err == nil {
		t.Fatal("loadQueries returned no error for a query without a location")
	}
}

// stubFetcher answers each URL from a canned map; an unmapped URL is an error.
type stubFetcher struct {
	pages map[string]string
	calls []string
}

func (s *stubFetcher) fetch(_ context.Context, url string) (string, error) {
	s.calls = append(s.calls, url)
	body, ok := s.pages[url]
	if !ok {
		return "", errors.New("not found: " + url)
	}
	return body, nil
}

func listing(entries ...string) string { return strings.Join(entries, "\n") }

func cardMarkup(id, company, profile, posting string) string {
	return `<div data-entity-urn="urn:li:jobPosting:` + id + `">` +
		`<a class="base-card__full-link" href="` + posting + `"></a>` +
		`<h3 class="base-search-card__title">Engineer</h3>` +
		`<h4 class="base-search-card__subtitle"><a href="` + profile + `">` + company + `</a></h4></div>`
}

func ldPosting(id string) string {
	return `<script type="application/ld+json">{"@type":"JobPosting","identifier":{"value":"` + id + `"}}</script>`
}

func ldProfile(site string) string {
	return `<script type="application/ld+json">{"@type":"Organization","sameAs":"` + site + `"}</script>`
}

func TestDiscoverCollapsesEmployersAndSkipsKnownOnes(t *testing.T) {
	queries := []query{{Keywords: "golang", Location: "Germany", Pages: 1, JobAge: 7}}
	searchURL := searchURL(queries[0], 1)
	f := &stubFetcher{pages: map[string]string{
		searchURL: listing(
			// Two postings by the same employer: the second must cost no request.
			cardMarkup("1", "Doodle", "https://ch.linkedin.com/company/doodle-ag", "https://x/jobs/view/a-1"),
			cardMarkup("2", "Doodle", "https://ch.linkedin.com/company/doodle-ag", "https://x/jobs/view/b-2"),
			// An employer the catalogue already has: dropped before any fetch.
			cardMarkup("3", "Capgemini", "https://linkedin.com/company/capgemini", "https://x/jobs/view/c-3"),
		),
		"https://x/jobs/view/a-1":                   ldPosting("teamtailor-8094978"),
		"https://ch.linkedin.com/company/doodle-ag": ldProfile("https://doodle.com"),
		"https://linkedin.com/company/capgemini":    ldProfile("https://capgemini.com"),
		"https://x/jobs/view/c-3":                   ldPosting("999"),
	}}

	got, stats, err := discover(context.Background(), f.fetch, queries, map[string]bool{"capgemini": true})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates (%+v), want 1", len(got), got)
	}
	c := got[0]
	if c.Name != "Doodle" || c.Website != "https://doodle.com" || c.ExternalID != "teamtailor-8094978" {
		t.Errorf("candidate = %+v, want Doodle / doodle.com / teamtailor-8094978", c)
	}
	if c.LinkedIn != "https://ch.linkedin.com/company/doodle-ag" {
		t.Errorf("candidate profile = %q", c.LinkedIn)
	}
	for _, u := range f.calls {
		if strings.Contains(u, "capgemini") {
			t.Errorf("spent a request on an already-known company: %s", u)
		}
	}
	if stats.emptyQueries != 0 || stats.totalQueries != 1 {
		t.Errorf("stats = %+v, want 0 empty of 1", stats)
	}
}

func TestDiscoverOmitsACandidateWithNoWebsite(t *testing.T) {
	queries := []query{{Keywords: "golang", Location: "Germany", Pages: 1, JobAge: 7}}
	f := &stubFetcher{pages: map[string]string{
		searchURL(queries[0], 1):            cardMarkup("1", "Acme", "https://linkedin.com/company/acme", "https://x/jobs/view/a-1"),
		"https://x/jobs/view/a-1":           ldPosting("4698693006"),
		"https://linkedin.com/company/acme": `<html>no structured data</html>`,
	}}
	got, _, err := discover(context.Background(), f.fetch, queries, nil)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want none (nothing for resolve to follow)", got)
	}
}

func TestDiscoverKeepsACandidateWhosePostingFetchFails(t *testing.T) {
	// A failed posting fetch costs the id, not the candidate: the careers-page path still works.
	queries := []query{{Keywords: "golang", Location: "Germany", Pages: 1, JobAge: 7}}
	f := &stubFetcher{pages: map[string]string{
		searchURL(queries[0], 1):            cardMarkup("1", "Acme", "https://linkedin.com/company/acme", "https://x/jobs/view/a-1"),
		"https://linkedin.com/company/acme": ldProfile("https://acme.com"),
	}}
	got, _, err := discover(context.Background(), f.fetch, queries, nil)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "" {
		t.Errorf("got %+v, want one candidate without an id", got)
	}
}

func TestDiscoverCountsEmptyQueries(t *testing.T) {
	// A 200 with no postings is a markup change or a block, not an empty market.
	queries := []query{
		{Keywords: "golang", Location: "Germany", Pages: 1, JobAge: 7},
		{Keywords: "rust", Location: "Poland", Pages: 1, JobAge: 7},
	}
	f := &stubFetcher{pages: map[string]string{
		searchURL(queries[0], 1): `<html>no cards</html>`,
		searchURL(queries[1], 1): `<html>no cards either</html>`,
	}}
	_, stats, err := discover(context.Background(), f.fetch, queries, nil)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if stats.emptyQueries != 2 || stats.totalQueries != 2 {
		t.Errorf("stats = %+v, want every query counted empty", stats)
	}
	if !stats.everyQueryEmpty() {
		t.Error("everyQueryEmpty = false, want true (the run must fail)")
	}
}

func TestSearchURLCarriesTheQuery(t *testing.T) {
	u := searchURL(query{Keywords: "data engineer", Location: "Poland", JobAge: 7, Pages: 2}, 2)
	for _, want := range []string{"keywords=data+engineer", "location=Poland", "f_TPR=r604800", "start=10"} {
		if !strings.Contains(u, want) {
			t.Errorf("searchURL = %q, want it to contain %q", u, want)
		}
	}
}
