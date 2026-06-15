package sources

import (
	"context"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func parseHTML(t *testing.T, s string) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return n
}

const sfDetailHTML = `<html><head>
<meta property="og:title" content="Fallback Title"/>
</head><body>
<div itemscope itemtype="http://schema.org/JobPosting">
  <span itemprop="title">Commissioning Engineer</span>
  <div itemprop="description"><h2>Role</h2><p>Build it.</p><script>alert(1)</script></div>
</div></body></html>`

func sfSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc><lastmod>2026-06-06</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestSuccessFactorsProvider(t *testing.T) {
	if got := NewSuccessFactors(nil).Provider(); got != "successfactors" {
		t.Errorf("Provider() = %q, want %q", got, "successfactors")
	}
}

func TestSFJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.tetrapak.com/job/Munich-Engineer/12345/":             "12345",
		"https://jobs.tetrapak.com/job/Commissioning-Engineer/98012-en_GB": "98012",
		"https://jobs.tetrapak.com/job/Slug/883999301":                     "883999301",
	}
	for loc, want := range cases {
		if got := sfJobID(loc); got != want {
			t.Errorf("sfJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestSFItempropHelpers(t *testing.T) {
	root := parseHTML(t, sfDetailHTML)
	if got := itempropText(root, "title"); got != "Commissioning Engineer" {
		t.Errorf("itempropText(title) = %q", got)
	}
	if got := itempropText(root, "missing"); got != "" {
		t.Errorf("itempropText(missing) = %q, want empty", got)
	}
	inner := itempropHTML(root, "description")
	if !strings.Contains(inner, "<h2>Role</h2>") || !strings.Contains(inner, "<p>Build it.</p>") {
		t.Errorf("itempropHTML(description) lost structure: %q", inner)
	}
	if got := metaProperty(root, "og:title"); got != "Fallback Title" {
		t.Errorf("metaProperty(og:title) = %q", got)
	}
}

func TestSFItempropHTMLPicksRichest(t *testing.T) {
	// SuccessFactors wraps several near-empty itemprop="description" layout regions around
	// the real body; the adapter must pick the one with the most content, not the first.
	h := `<div itemscope itemtype="http://schema.org/JobPosting">
		<div itemprop="description">   </div>
		<div itemprop="description"><h2>Real Body</h2><p>The actual job description text.</p></div>
		<div itemprop="description"> </div>
	</div>`
	root := parseHTML(t, h)
	got := itempropHTML(root, "description")
	if !strings.Contains(got, "Real Body") || !strings.Contains(got, "actual job description") {
		t.Errorf("itempropHTML should pick the richest description, got %q", got)
	}
}

func TestSuccessFactorsFetchSitemapThenDetailAndMaps(t *testing.T) {
	loc := "https://jobs.tetrapak.com/job/Munich-Engineer/12345/"
	fake := (&routedHTTP{}).
		route("/job_sitemap.xml", sfSitemapXML(loc)).
		route("/job/Munich-Engineer/12345", sfDetailHTML)

	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tetra Pak", Provider: "successfactors", Board: "jobs.tetrapak.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "12345" {
		t.Errorf("ExternalID = %q, want 12345", j.ExternalID)
	}
	if j.URL != loc {
		t.Errorf("URL = %q, want %q", j.URL, loc)
	}
	if j.Title != "Commissioning Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Tetra Pak" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "" {
		t.Errorf("Location = %q, want empty (enrichment fills it)", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<h2>Role</h2>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-06", j.PostedAt)
	}
}

func TestSuccessFactorsTitleFallsBackToOgTitle(t *testing.T) {
	noTitle := `<html><head><meta property="og:title" content="OG Only"/></head>
<body><div itemscope itemtype="http://schema.org/JobPosting">
<div itemprop="description"><p>body</p></div></div></body></html>`
	loc := "https://jobs.tetrapak.com/job/X/777/"
	fake := (&routedHTTP{}).route("/job_sitemap.xml", sfSitemapXML(loc)).route("/job/X/777", noTitle)
	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.tetrapak.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "OG Only" {
		t.Fatalf("title fallback failed: %v", jobs)
	}
}

func TestSuccessFactorsFailedDetailDropsOnlyThatPosting(t *testing.T) {
	ok := "https://jobs.tetrapak.com/job/Kept/111/"
	bad := "https://jobs.tetrapak.com/job/Dropped/222/"
	// No route for /job/Dropped/222 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("/job_sitemap.xml", sfSitemapXML(ok, bad)).
		route("/job/Kept/111", sfDetailHTML)

	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.tetrapak.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestSuccessFactorsDropsJobWithNoParseableID(t *testing.T) {
	// A loc with no numeric id would yield an empty external_id, which collides on the
	// (source, external_id) dedup key — drop the posting instead.
	loc := "https://jobs.tetrapak.com/job/No-Numeric-Id/"
	fake := (&routedHTTP{}).
		route("/job_sitemap.xml", sfSitemapXML(loc)).
		route("/job/No-Numeric-Id", sfDetailHTML)
	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.tetrapak.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (unparseable id dropped)", len(jobs))
	}
}

func TestSFMetaPropertyReturnsFirst(t *testing.T) {
	root := parseHTML(t, `<head><meta property="og:title" content="First"/><meta property="og:title" content="Second"/></head>`)
	if got := metaProperty(root, "og:title"); got != "First" {
		t.Errorf("metaProperty(og:title) = %q, want First", got)
	}
}

func TestSuccessFactorsEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/job_sitemap.xml", sfSitemapXML())
	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.tetrapak.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
