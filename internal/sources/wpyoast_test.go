package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// wpyoastIndexXML is the Yoast sitemap index: per-post-type sub-sitemaps. The adapter must
// pick the "job_listing" one and ignore page-sitemap.
const wpyoastIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://careers.acme.com/page-sitemap.xml</loc></sitemap>
<sitemap><loc>https://careers.acme.com/job_listing-sitemap.xml</loc></sitemap>
</sitemapindex>`

// wpyoastJobSitemapXML is the job_listing sub-sitemap: a urlset of job-page locs.
func wpyoastJobSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

const wpyoastDetailHTML = `<html><head></head><body>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
"title":"Electrical Engineer III &#8211; Design",
"description":"<p>Design things.</p><script>alert(1)<\/script>",
"datePosted":"2026-05-19T00:00:00Z",
"identifier":{"@type":"PropertyValue","name":"Acme","value":"640127"},
"hiringOrganization":{"@type":"Organization","name":"Acme Group"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Corona","addressRegion":"CA","addressCountry":"US"}}}
</script>
</body></html>`

func TestWPYoastProvider(t *testing.T) {
	if got := NewWPYoast(nil).Provider(); got != "wpyoast" {
		t.Errorf("Provider() = %q, want %q", got, "wpyoast")
	}
}

func TestWPYoastJobID(t *testing.T) {
	cases := map[string]string{
		"https://careers.theplanetgroup.com/job/640127-electrical-engineer-corona-california/": "640127",
		"https://careers.theplanetgroup.com/page/about/":                                       "",
	}
	for u, want := range cases {
		if got := wpyoastJobID(u); got != want {
			t.Errorf("wpyoastJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestWPYoastFetchResolvesJobSitemapThenMaps(t *testing.T) {
	job := "https://careers.acme.com/job/640127-electrical-engineer-corona-california/"
	// The job sitemap route is added before the index so the more specific match wins; the
	// adapter must follow the index → job_listing sub-sitemap → detail chain.
	fake := (&routedHTTP{}).
		route("/job/640127-electrical-engineer-corona-california/", wpyoastDetailHTML).
		route("/job_listing-sitemap.xml", wpyoastJobSitemapXML(job)).
		route("/sitemap.xml", wpyoastIndexXML)

	jobs, err := NewWPYoast(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme Group", Provider: "wpyoast", Board: "careers.acme.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "640127" {
		t.Errorf("ExternalID = %q, want 640127", j.ExternalID)
	}
	if j.URL != job {
		t.Errorf("URL = %q, want %q", j.URL, job)
	}
	if j.Title != "Electrical Engineer III – Design" {
		t.Errorf("Title = %q, want entity-decoded", j.Title)
	}
	if j.Company != "Acme Group" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Corona, CA, US" {
		t.Errorf("Location = %q, want %q", j.Location, "Corona, CA, US")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Design things") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-19", j.PostedAt)
	}
}

func TestWPYoastNoJobSitemapYieldsNoJobsNoError(t *testing.T) {
	indexNoJobs := `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://careers.acme.com/page-sitemap.xml</loc></sitemap></sitemapindex>`
	fake := (&routedHTTP{}).route("/sitemap.xml", indexNoJobs)
	jobs, err := NewWPYoast(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.acme.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestWPYoastFiltersNonJobSitemapEntries(t *testing.T) {
	job := "https://careers.acme.com/job/640127-role-corona-california/"
	fake := (&routedHTTP{}).
		route("/job/640127-role-corona-california/", wpyoastDetailHTML).
		route("/job_listing-sitemap.xml", wpyoastJobSitemapXML(
			"https://careers.acme.com/jobs/", // listing index, no numeric id
			job,
		)).
		route("/sitemap.xml", wpyoastIndexXML)

	jobs, err := NewWPYoast(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.acme.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "640127" {
		t.Fatalf("got %v, want only the real job", jobs)
	}
}
