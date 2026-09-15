package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

func techtreeFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func techtreeSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

const techtreeNoLDJSON = `<html><head><title>No structured data</title></head><body>
<div class="prose prose-base"><p>Body text</p></div>
</body></html>`

const techtreeNoHiringOrg = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Untitled Role","description":"Short summary."}
</script></head><body>
<div class="prose prose-base"><p>Full body text.</p></div>
</body></html>`

const techtreeNoProseBody = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Untitled Role","description":"Short summary.","hiringOrganization":{"@type":"Organization","name":"Acme"},"jobLocation":{"@type":"Place","address":"Remote"},"employmentType":"FULL_TIME"}
</script></head><body>
<div class="not-prose"><p>Some other text.</p></div>
</body></html>`

func TestTechTreeProvider(t *testing.T) {
	if got := NewTechTree(nil).Provider(); got != "techtree" {
		t.Errorf("Provider() = %q, want techtree", got)
	}
}

func TestTechTreeIsBoardlessAggregator(t *testing.T) {
	s := NewTechTree(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("techtree should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("techtree should implement the aggregator marker")
	}
}

func TestTechTreeRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["techtree"]; !ok {
		t.Error("All() should register provider techtree")
	}
	if !slices.Contains(FilterableProviders(), "techtree") {
		t.Error("FilterableProviders() should include techtree")
	}
}

func TestTechTreeSitemapEnumerationExcludesStaticPages(t *testing.T) {
	fake := (&routedHTTP{}).route("/sitemap.xml", techtreeFixture(t, "techtree_sitemap.xml"))
	locs, err := sitemapJobLocs(context.Background(), fake, techtreeSitemapURL, techtreeJobID)
	if err != nil {
		t.Fatalf("sitemapJobLocs: %v", err)
	}
	if len(locs) != 100 {
		t.Errorf("got %d job locs, want 100", len(locs))
	}
	for _, l := range locs {
		if !strings.Contains(l, "/job/") {
			t.Errorf("non-job entry leaked into job locs: %s", l)
		}
	}
}

func TestTechTreeFetchMapsRealFixtures(t *testing.T) {
	telepatiaURL := "https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8"
	secondURL := "https://jobs.techtree.dev/job/0515f6f5-8f30-4cb6-abd2-16bf87137a4d"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", techtreeSitemapXML(telepatiaURL, secondURL, "https://jobs.techtree.dev/talent-scout")).
		route(telepatiaURL, techtreeFixture(t, "techtree_job_telepatia.html")).
		route(secondURL, techtreeFixture(t, "techtree_job_second.html"))

	jobs, err := NewTechTree(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (non-job sitemap entry filtered)", len(jobs))
	}

	var telepatia, second Job
	for _, j := range jobs {
		switch j.ExternalID {
		case "8773660e-c2c3-4b49-9d03-6f9f76859bc8":
			telepatia = j
		case "0515f6f5-8f30-4cb6-abd2-16bf87137a4d":
			second = j
		}
	}

	if telepatia.URL != telepatiaURL {
		t.Errorf("telepatia.URL = %q, want %q (canonical loc, no tp param)", telepatia.URL, telepatiaURL)
	}
	if telepatia.Company != "Telepatia" {
		t.Errorf("telepatia.Company = %q, want Telepatia", telepatia.Company)
	}
	if telepatia.Title != "Mid/Senior AI Engineer, New Products (0→1)" {
		t.Errorf("telepatia.Title = %q", telepatia.Title)
	}
	if !strings.Contains(telepatia.Location, "Latin America") {
		t.Errorf("telepatia.Location = %q", telepatia.Location)
	}
	if !telepatia.Remote {
		t.Errorf("telepatia.Remote = false, want true (location states remote)")
	}
	if telepatia.EmploymentType != "full_time" {
		t.Errorf("telepatia.EmploymentType = %q, want full_time", telepatia.EmploymentType)
	}
	if telepatia.PostedAt == nil {
		t.Error("telepatia.PostedAt is nil")
	}
	if !strings.Contains(telepatia.Description, "Why Telepatia") ||
		!strings.Contains(telepatia.Description, "Build MVPs fast") {
		t.Errorf("telepatia.Description missing full body sections: %q", telepatia.Description)
	}
	shortSummary := "Mid/Senior AI Engineer, New Products at Telepatia - turn vague product bets into validated 0→1 products, fast"
	if telepatia.Description == shortSummary {
		t.Error("telepatia.Description equals the ld+json short summary, want the full body")
	}

	if second.Company != "European defence software startup" {
		t.Errorf("second.Company = %q", second.Company)
	}
	if second.Remote {
		t.Errorf("second.Remote = true, want false (Paris, France, on-site)")
	}
}

func TestTechTreeFetchFailsOnUnreadableSitemap(t *testing.T) {
	fake := &routedHTTP{} // no route for sitemap.xml -> GetXML fails to decode/find it
	if _, err := NewTechTree(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("Fetch with unreadable sitemap should return an error")
	}
}

func TestTechTreeFetchFailsWhenEveryPostingUnreadable(t *testing.T) {
	job := "https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", techtreeSitemapXML(job)).
		route(job, techtreeNoLDJSON) // no ld+json at all -> detail parse fails

	if _, err := NewTechTree(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("Fetch should fail when every listed posting is unreadable")
	}
}

func TestTechTreeDetailDropsPostingWithoutHiringOrganization(t *testing.T) {
	good := "https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8"
	bad := "https://jobs.techtree.dev/job/00000000-0000-0000-0000-000000000000"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", techtreeSitemapXML(good, bad)).
		route(good, techtreeFixture(t, "techtree_job_telepatia.html")).
		route(bad, techtreeNoHiringOrg)

	jobs, err := NewTechTree(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != good {
		t.Errorf("posting without a hiring company should be dropped, the other kept: jobs=%v", jobs)
	}
}

func TestTechTreeDetailDropsPostingWithoutProseBody(t *testing.T) {
	good := "https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8"
	bad := "https://jobs.techtree.dev/job/00000000-0000-0000-0000-000000000001"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", techtreeSitemapXML(good, bad)).
		route(good, techtreeFixture(t, "techtree_job_telepatia.html")).
		route(bad, techtreeNoProseBody)

	jobs, err := NewTechTree(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != good {
		t.Errorf("posting without a locatable rich-text body should be dropped, the other kept: jobs=%v", jobs)
	}
}

func TestTechTreeJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8":                                         "8773660e-c2c3-4b49-9d03-6f9f76859bc8",
		"https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8?tp=274cfae4-c111-4f70-9de1-01121f16a127": "8773660e-c2c3-4b49-9d03-6f9f76859bc8",
		"https://jobs.techtree.dev/":                 "",
		"https://jobs.techtree.dev/talent-scout":     "",
		"https://jobs.techtree.dev/terms-of-service": "",
	}
	for u, want := range cases {
		if got := techtreeJobID(u); got != want {
			t.Errorf("techtreeJobID(%q) = %q, want %q", u, got, want)
		}
	}
}
