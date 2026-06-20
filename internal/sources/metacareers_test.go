package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// metaDetailHTML renders a job page the way metacareers.com does: a single
// application/ld+json JobPosting. The jobLocation address.* fields are deliberately
// wrong (Meta renders them as a repeated bogus value), so the adapter must read
// jobLocation[].name and never the address sub-object.
func metaDetailHTML(title, datePosted string, locationNames ...string) string {
	var locs strings.Builder
	for _, n := range locationNames {
		locs.WriteString(`{"@type":"Place","name":"` + n + `","address":{"@type":"PostalAddress","addressLocality":"Aiken","addressRegion":"SC","addressCountry":{"@type":"Country","name":["USA","USA"]}}},`)
	}
	loc := strings.TrimSuffix(locs.String(), ",")
	// The closing script tag inside the JSON is escaped as <\/script> exactly as Meta
	// renders it, so the embedded ld+json block is not terminated early by the HTML parser.
	return `<html><head><script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting",
"title":"` + title + `",
"description":"<h2>Role<\/h2><p>Build it.<\/p><script>alert(1)<\/script>",
"datePosted":"` + datePosted + `",
"employmentType":"Full-time",
"jobLocation":[` + loc + `]}
</script></head><body>page</body></html>`
}

func metaSitemapXML(entries ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, e := range entries {
		b.WriteString(`<url><loc>` + e[0] + `</loc><lastmod>` + e[1] + `</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestMetaCareersProvider(t *testing.T) {
	if got := NewMetaCareers(nil).Provider(); got != "meta" {
		t.Errorf("Provider() = %q, want %q", got, "meta")
	}
}

// TestMetaHTTPBlocksInternalTarget drives the real metaHTTP wrapper (tls-client over the
// SSRF-guarded dialer) at a loopback target and asserts it is refused. This locks the SSRF
// contract through the actual fingerprint transport — the durable protection against a future
// tls-client upgrade silently changing its dial path — and covers the wrapper's error path
// without any real network. No external egress: the guard's Control hook rejects the address
// before a connection is attempted.
func TestMetaHTTPBlocksInternalTarget(t *testing.T) {
	c, err := newMetaHTTP()
	if err != nil {
		t.Fatalf("newMetaHTTP: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var v struct{}
	err = c.GetXML(ctx, "http://127.0.0.1:9/x", &v)
	if err == nil {
		t.Fatal("expected metaHTTP to refuse a loopback target, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error %q does not mention the SSRF block", err)
	}
}

func TestMetaCareersRegisteredAndBoardless(t *testing.T) {
	registry := All(nil)
	src, ok := registry["meta"]
	if !ok {
		t.Fatal("meta not registered in sources.All")
	}
	if _, isBoardless := src.(boardless); !isBoardless {
		t.Error("meta should be a boardless single-company source")
	}
	// As a single-company boardless source it must be excluded from the source facet.
	for _, p := range FilterableProviders() {
		if p == "meta" {
			t.Error("single-company boardless meta should not appear in the source facet")
		}
	}
}

func TestMetaJobID(t *testing.T) {
	cases := map[string]string{
		"https://www.metacareers.com/profile/job_details/1690022942358388/": "1690022942358388",
		"https://www.metacareers.com/profile/job_details/27299132166411637": "27299132166411637",
		"https://www.metacareers.com/profile/no-id/":                        "",
	}
	for loc, want := range cases {
		if got := metaJobID(loc); got != want {
			t.Errorf("metaJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestMetaCareersFetchSitemapThenDetailAndMaps(t *testing.T) {
	loc := "https://www.metacareers.com/profile/job_details/12345/"
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML([2]string{loc, "2026-06-06T19:38:59-07:00"})).
		route("/job_details/12345", metaDetailHTML("Critical Facility Engineer", "2026-04-14T15:49:02-07:00", "Menlo Park, CA"))

	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Meta", Provider: "meta",
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
	if j.Title != "Critical Facility Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Meta" {
		t.Errorf("Company = %q, want Meta", j.Company)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<h2>Role</h2>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	// datePosted wins over the sitemap <lastmod>.
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 4, 14, 22, 49, 2, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-04-14T15:49:02-07:00", j.PostedAt)
	}
}

func TestMetaCareersLocationUsesNameNotBrokenAddress(t *testing.T) {
	// The address.* fields say "Aiken, SC"; the correct value lives in jobLocation[].name.
	loc := "https://www.metacareers.com/profile/job_details/777/"
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML([2]string{loc, "2026-06-06T00:00:00-07:00"})).
		route("/job_details/777", metaDetailHTML("Eng", "2026-04-14T15:49:02-07:00", "Menlo Park, CA", "London, UK"))

	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if strings.Contains(jobs[0].Location, "Aiken") {
		t.Errorf("Location used the broken address field: %q", jobs[0].Location)
	}
	if !strings.Contains(jobs[0].Location, "Menlo Park, CA") {
		t.Errorf("Location should include the first jobLocation name, got %q", jobs[0].Location)
	}
}

func TestMetaCareersEmptyLocationWhenNoJobLocation(t *testing.T) {
	loc := "https://www.metacareers.com/profile/job_details/888/"
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML([2]string{loc, "2026-06-06T00:00:00-07:00"})).
		route("/job_details/888", metaDetailHTML("Eng", "2026-04-14T15:49:02-07:00"))
	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "" {
		t.Fatalf("want empty location, got %v", jobs)
	}
}

func TestMetaCareersPostedAtFallsBackToLastmod(t *testing.T) {
	loc := "https://www.metacareers.com/profile/job_details/999/"
	// Empty datePosted → fall back to the sitemap <lastmod>.
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML([2]string{loc, "2026-05-05T12:00:00Z"})).
		route("/job_details/999", metaDetailHTML("Eng", "", "Remote, US"))
	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].PostedAt == nil || !jobs[0].PostedAt.Equal(time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want lastmod fallback 2026-05-05T12:00:00Z", jobs[0].PostedAt)
	}
}

func TestMetaCareersFailedDetailDropsOnlyThatPosting(t *testing.T) {
	ok := "https://www.metacareers.com/profile/job_details/111/"
	bad := "https://www.metacareers.com/profile/job_details/222/"
	// No route for /job_details/222 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML(
			[2]string{ok, "2026-06-06T00:00:00Z"}, [2]string{bad, "2026-06-06T00:00:00Z"})).
		route("/job_details/111", metaDetailHTML("Kept", "2026-04-14T15:49:02-07:00", "Menlo Park, CA"))

	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestMetaCareersDropsJobWithNoParseableID(t *testing.T) {
	loc := "https://www.metacareers.com/profile/teams/engineering/"
	fake := (&routedHTTP{}).
		route("/jobsearch/sitemap.xml", metaSitemapXML([2]string{loc, "2026-06-06T00:00:00Z"})).
		route("/profile/teams/engineering", metaDetailHTML("X", "2026-04-14T15:49:02-07:00", "Menlo Park, CA"))
	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (unparseable id dropped)", len(jobs))
	}
}

func TestMetaCareersEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/jobsearch/sitemap.xml", metaSitemapXML())
	jobs, err := NewMetaCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Meta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
