package sources

import (
	"context"
	"errors"
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

// SuccessFactors earns fullBoardListing because Fetch is a single sitemap fetch with no
// pagination loop — it either succeeds with the whole listing or fails outright.
func TestSuccessFactorsMarkers(t *testing.T) {
	s := NewSuccessFactors(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("successfactors should implement the fullBoardListing marker")
	}
}

func TestSuccessFactorsRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["successfactors"] {
		t.Error("FullBoardListingProviders(All(nil)) should include successfactors")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestSuccessFactorsMarkers' fullBoardListing claim rests on.
func TestSuccessFactorsFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
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

func TestSFTenant(t *testing.T) {
	cases := map[string]string{
		// A hub site serves every tenant from one host and names the tenant in the first
		// path segment.
		"https://jobsearch.createyourowncareer.com/Riverty/job/Berlin-Software-Engineer-10623/1425618633/": "Riverty",
		"https://jobsearch.createyourowncareer.com/PRH_US/job/New-York-Assistant-Editor-10019/1414466433/": "PRH_US",
		// The same hub sitemap also lists postings with no tenant segment at all; "job" is
		// the platform's own word and must never read as a tenant.
		"https://jobsearch.createyourowncareer.com/job/Dortmund-Delphi-Entwickler-44369/1431430433/": "",
		// An ordinary single-tenant SuccessFactors site has the same shape, so it too
		// yields no tenant and falls back to the configured company.
		"https://jobs.tetrapak.com/job/Munich-Engineer/12345/": "",
		"https://jobs.tetrapak.com/":                           "",
		"https://jobs.tetrapak.com":                            "",
		"://not a url":                                         "",
		// A tenant's own landing page is all path and no posting; it still names the tenant.
		"https://jobsearch.createyourowncareer.com/Riverty": "Riverty",
	}
	for loc, want := range cases {
		if got := sfTenant(loc); got != want {
			t.Errorf("sfTenant(%q) = %q, want %q", loc, got, want)
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

// A hub site serves many employers from one host and one shared sitemap, naming the tenant
// only in the job URL. The configured company is the hub's own name and the fallback.
func TestSuccessFactorsHubResolvesEmployerPerPosting(t *testing.T) {
	const host = "https://jobsearch.createyourowncareer.com"
	mapped := host + "/Riverty/job/Berlin-Software-Engineer-10623/1425618633/"
	unmapped := host + "/Sonopress/job/Guetersloh-Operator-33333/1400000001/"
	tenantless := host + "/job/Dortmund-Delphi-Entwickler-44369/1431430433/"

	fake := (&routedHTTP{}).
		route("/job_sitemap.xml", sfSitemapXML(mapped, unmapped, tenantless)).
		route("/Riverty/job/Berlin-Software-Engineer-10623/1425618633", sfDetailHTML).
		route("/Sonopress/job/Guetersloh-Operator-33333/1400000001", sfDetailHTML).
		route("/job/Dortmund-Delphi-Entwickler-44369/1431430433", sfDetailHTML)

	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{
		Company:  "Bertelsmann",
		Provider: "successfactors",
		Board:    "jobsearch.createyourowncareer.com",
		Hub:      true,
		Tenants:  map[string]string{"Riverty": "Riverty"},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}

	byID := map[string]string{}
	for _, j := range jobs {
		byID[j.ExternalID] = j.Company
	}
	want := map[string]string{
		"1425618633": "Riverty",     // mapped tenant
		"1400000001": "Bertelsmann", // unmapped tenant falls back, never "Sonopress"
		"1431430433": "Bertelsmann", // no tenant segment; never a company called "job"
	}
	for id, wantCompany := range want {
		if byID[id] != wantCompany {
			t.Errorf("job %s company = %q, want %q", id, byID[id], wantCompany)
		}
	}
}

// The hub branch must not reach an ordinary board: a single-tenant site's URLs carry path
// segments too, and reading one as an employer would rename the company.
func TestSuccessFactorsNonHubIgnoresTheURLPath(t *testing.T) {
	loc := "https://jobs.tetrapak.com/Riverty/job/Munich-Engineer/12345/"
	fake := (&routedHTTP{}).
		route("/job_sitemap.xml", sfSitemapXML(loc)).
		route("/Riverty/job/Munich-Engineer/12345", sfDetailHTML)

	jobs, err := NewSuccessFactors(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tetra Pak", Board: "jobs.tetrapak.com",
		Tenants: map[string]string{"Riverty": "Riverty"}, // present but not honoured without Hub
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Tetra Pak" {
		t.Fatalf("company = %+v, want Tetra Pak", jobs)
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
