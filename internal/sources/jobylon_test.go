package sources

import (
	"context"
	"strings"
	"testing"
)

// jobylonIndexXML is the sitemap index: its one child is the jobs sub-sitemap. The adapter must
// resolve the child by its "sitemap-jobs" name, not by a hardcoded URL.
const jobylonIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://emp.jobylon.com/sitemap-jobs.xml</loc><lastmod>2026-07-18</lastmod></sitemap>
</sitemapindex>`

// jobylonJobsXML is the flat jobs urlset: two real job pages plus a company page (not a /jobs/<id>
// posting, so it must be skipped).
const jobylonJobsXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://emp.jobylon.com/jobs/369523-acme-engineer/</loc><lastmod>2026-07-17</lastmod></url>
<url><loc>https://emp.jobylon.com/jobs/111-globex-designer/</loc></url>
<url><loc>https://emp.jobylon.com/companies/2451-acme/</loc></url>
</urlset>`

// jobylonAcmeHTML is a real-shape job page: the title carries an HTML entity, employmentType is an
// ARRAY (which must not fail the whole-posting decode), the description has a stray <script> to
// sanitize, and jobLocation is a multi-place array.
const jobylonAcmeHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Engineer &amp; Lead",
"employmentType":["CONTRACTOR"],
"description":"<p>Build things.</p><script>evil()<\/script>","datePosted":"2026-07-17T13:26:14+00:00",
"hiringOrganization":{"@type":"Organization","name":"Acme &amp; Co"},
"jobLocation":[
 {"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Kista","addressCountry":"Sweden"}},
 {"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Oslo","addressCountry":"Norway"}}]}
</script></head><body></body></html>`

// jobylonEmptyCoHTML has a JobPosting whose hiringOrganization.name is empty — an aggregator
// posting with no company breaks the public slug and must be dropped.
const jobylonEmptyCoHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Designer",
"description":"<p>Design.</p>","datePosted":"2026-07-16T00:00:00+00:00",
"hiringOrganization":{"@type":"Organization","name":""},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Berlin"}}}
</script></head><body></body></html>`

func TestJobylonRegistered(t *testing.T) {
	if _, ok := All(nil)["jobylon"]; !ok {
		t.Fatal("jobylon must be registered in sources.All")
	}
	// A boardless aggregator stays in the source facet (unlike a single-company boardless source).
	found := false
	for _, p := range FilterableProviders() {
		if p == "jobylon" {
			found = true
			break
		}
	}
	if !found {
		t.Error("jobylon must be listed by FilterableProviders (boardless aggregator)")
	}
}

func TestJobylonProviderAndMarkers(t *testing.T) {
	s := NewJobylon(nil)
	if got := s.Provider(); got != "jobylon" {
		t.Errorf("Provider() = %q, want jobylon", got)
	}
	if _, ok := s.(boardless); !ok {
		t.Error("jobylon must be boardless (one global feed, no per-tenant board)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobylon must be an aggregator (company comes from each posting)")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("jobylon must be a HydratingSource (incremental detail hydration)")
	}
}

func TestJobylonFetchMapsJob(t *testing.T) {
	fake := (&routedHTTP{}).
		route("sitemap-jobs", jobylonJobsXML).
		route("sitemap.xml", jobylonIndexXML).
		route("/jobs/369523-", jobylonAcmeHTML).
		route("/jobs/111-", jobylonEmptyCoHTML)

	jobs, err := NewJobylon(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobylon"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 369523 maps; 111 is dropped (empty company); the /companies/ url is skipped (not a posting).
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "369523" {
		t.Errorf("external_id = %q, want 369523", j.ExternalID)
	}
	if j.URL != "https://emp.jobylon.com/jobs/369523-acme-engineer/" {
		t.Errorf("url = %q", j.URL)
	}
	if j.Title != "Engineer & Lead" {
		t.Errorf("title = %q, want HTML-unescaped 'Engineer & Lead'", j.Title)
	}
	if j.Company != "Acme & Co" {
		t.Errorf("company = %q, want HTML-unescaped 'Acme & Co' (from hiringOrganization)", j.Company)
	}
	if j.Location != "Kista, Sweden; Oslo, Norway" {
		t.Errorf("location = %q, want joined multi-place", j.Location)
	}
	if !strings.Contains(j.Description, "Build things") {
		t.Errorf("description missing body: %q", j.Description)
	}
	if strings.Contains(j.Description, "evil") {
		t.Errorf("description not sanitized (script survived): %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 {
		t.Errorf("posted_at = %v, want a 2026 date", j.PostedAt)
	}
}

func TestJobylonDropsUnusablePostings(t *testing.T) {
	// 369523 maps; 111 (empty company) and 222 (no JobPosting ld+json) are dropped.
	fake := (&routedHTTP{}).
		route("sitemap-jobs", jobylonJobsXMLWith222).
		route("sitemap.xml", jobylonIndexXML).
		route("/jobs/369523-", jobylonAcmeHTML).
		route("/jobs/111-", jobylonEmptyCoHTML).
		route("/jobs/222-", `<html><body>no structured data here</body></html>`)

	jobs, err := NewJobylon(fake).Fetch(context.Background(), CompanyEntry{Company: "Jobylon"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "369523" {
		t.Fatalf("jobs = %+v, want only 369523 (111 empty-company and 222 no-ld+json dropped)", jobs)
	}
}

// jobylonJobsXMLWith222 adds a third job whose page carries no JobPosting.
const jobylonJobsXMLWith222 = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://emp.jobylon.com/jobs/369523-acme-engineer/</loc></url>
<url><loc>https://emp.jobylon.com/jobs/111-globex-designer/</loc></url>
<url><loc>https://emp.jobylon.com/jobs/222-empty-void/</loc></url>
</urlset>`

func TestJobylonFetchNewHydratesOnlyUnseen(t *testing.T) {
	// 111 is already ingested (seen) → refreshed by identity, no detail fetch. 369523 is new →
	// hydrated from its detail page. No detail route is registered for 111, so a stray detail
	// fetch would fail and drop it — its presence proves no detail request was made.
	fake := (&routedHTTP{}).
		route("sitemap-jobs", jobylonJobsXML).
		route("sitemap.xml", jobylonIndexXML).
		route("/jobs/369523-", jobylonAcmeHTML)

	seen := func(id string) bool { return id == "111" }
	jobs, err := NewJobylon(fake).(HydratingSource).
		FetchNew(context.Background(), CompanyEntry{Company: "Jobylon"}, seen)
	if err != nil {
		t.Fatalf("fetchNew: %v", err)
	}

	var refreshed, hydrated *Job
	for i := range jobs {
		switch jobs[i].ExternalID {
		case "111":
			refreshed = &jobs[i]
		case "369523":
			hydrated = &jobs[i]
		}
	}
	if refreshed == nil {
		t.Fatal("seen posting 111 missing — it must be emitted as a liveness refresh")
	}
	if !refreshed.SeenRefresh {
		t.Error("seen posting 111 must set SeenRefresh")
	}
	if refreshed.Description != "" {
		t.Errorf("seen posting 111 must carry no content (no detail fetch), got description %q", refreshed.Description)
	}
	if refreshed.URL != "https://emp.jobylon.com/jobs/111-globex-designer/" {
		t.Errorf("seen posting 111 url = %q", refreshed.URL)
	}
	if hydrated == nil {
		t.Fatal("unseen posting 369523 missing — it must be hydrated")
	}
	if hydrated.SeenRefresh {
		t.Error("unseen posting 369523 must NOT set SeenRefresh")
	}
	if !strings.Contains(hydrated.Description, "Build things") {
		t.Errorf("unseen posting 369523 must be hydrated with its body, got %q", hydrated.Description)
	}
}
