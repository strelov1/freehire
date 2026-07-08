package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// icimsDetailHTML is an iCIMS ?in_iframe=1 job fragment: a server-rendered page whose
// only payload we read is the schema.org JobPosting ld+json. The description embeds a
// <script> written as <\/script> so the JSON string carries it without the HTML parser
// treating it as the end of the ld+json block (the standard escaping); sanitizeHTML must
// then strip it. streetAddress/postalCode are the iCIMS "UNAVAILABLE" placeholder and
// must not leak into the location.
const icimsDetailHTML = `<html><head></head><body>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"JobPosting",
"title":"Mobile Medical Assistant",
"description":"<h2>Overview</h2><p>Assist physicians.</p><script>alert(1)<\/script>",
"datePosted":"2026-04-15T00:00:00.000Z",
"hiringOrganization":{"@type":"Organization","name":"360care Inc"},
"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",
"streetAddress":"UNAVAILABLE","postalCode":"UNAVAILABLE",
"addressLocality":"Kalamazoo","addressRegion":"MI","addressCountry":"US"}}]}
</script>
</body></html>`

// icimsSitemapXML builds an iCIMS sitemap urlset from the given job locs.
func icimsSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc><lastmod>2026-06-16T15:51:06-04:00</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// icimsSitemapIndexXML builds an iCIMS <sitemapindex> pointing at the given sub-sitemap locs.
func icimsSitemapIndexXML(subLocs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range subLocs {
		b.WriteString(`<sitemap><loc>` + l + `</loc></sitemap>`)
	}
	b.WriteString(`</sitemapindex>`)
	return b.String()
}

func TestICIMSHost(t *testing.T) {
	cases := map[string]string{
		"360care":              "careers-360care.icims.com", // classic slug → icims host
		"careers.docusign.com": "careers.docusign.com",      // vanity domain passes through
	}
	for board, want := range cases {
		if got := icimsHost(board); got != want {
			t.Errorf("icimsHost(%q) = %q, want %q", board, got, want)
		}
	}
}

// A vanity-domain board (careers.docusign.com) serves a sitemap INDEX (not a flat urlset)
// and its job detail lives at /careers-home/jobs/<id>?in_iframe=1 (not <loc>?in_iframe=1).
// The adapter must follow the index, match the query-form job loc, and fetch the
// careers-home fragment.
func TestICIMSVanityDomainSitemapIndexAndCareersHomeDetail(t *testing.T) {
	loc := "https://careers.docusign.com/jobs/27897?lang=en-us"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapIndexXML("https://careers.docusign.com/sitemap1.xml")).
		route("/sitemap1.xml", icimsSitemapXML(loc)).
		route("/careers-home/jobs/27897?in_iframe=1", icimsDetailHTML)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Docusign", Provider: "icims", Board: "careers.docusign.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (index followed, query-form loc matched)", len(jobs))
	}
	if jobs[0].ExternalID != "27897" {
		t.Errorf("ExternalID = %q, want 27897", jobs[0].ExternalID)
	}
	if jobs[0].URL != loc {
		t.Errorf("URL = %q, want the canonical sitemap loc %q", jobs[0].URL, loc)
	}
	if jobs[0].Title != "Mobile Medical Assistant" {
		t.Errorf("Title = %q — detail fragment not fetched via careers-home path", jobs[0].Title)
	}
}

func TestICIMSProvider(t *testing.T) {
	if got := NewICIMS(nil).Provider(); got != "icims" {
		t.Errorf("Provider() = %q, want %q", got, "icims")
	}
}

func TestICIMSJobID(t *testing.T) {
	cases := map[string]string{
		"https://careers-360care.icims.com/jobs/3787/mobile-medical-assistant/job": "3787",
		"https://careers-acme.icims.com/jobs/12345/some-role/job?in_iframe=1":      "12345",
		"https://careers-acme.icims.com/jobs/search":                               "",
		"https://careers-acme.icims.com/jobs/intro":                                "",
		// Vanity/careers-home sitemap locs: /jobs/<id> with a query and no trailing slash.
		"https://careers.docusign.com/jobs/27897?lang=en-us": "27897",
		"https://careers.docusign.com/jobs/29339":            "29339",
	}
	for loc, want := range cases {
		if got := icimsJobID(loc); got != want {
			t.Errorf("icimsJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestICIMSFetchSitemapThenDetailAndMaps(t *testing.T) {
	loc := "https://careers-360care.icims.com/jobs/3787/mobile-medical-assistant/job"
	// The detail route matches only the ?in_iframe=1 URL, so the test fails unless the
	// adapter fetches the iframe fragment (not the SPA-wrapper canonical page).
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(loc)).
		route("/jobs/3787/mobile-medical-assistant/job?in_iframe=1", icimsDetailHTML)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{
		Company: "360care", Provider: "icims", Board: "360care",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "3787" {
		t.Errorf("ExternalID = %q, want 3787", j.ExternalID)
	}
	if j.URL != loc {
		t.Errorf("URL = %q, want canonical %q (no ?in_iframe=1)", j.URL, loc)
	}
	if j.Title != "Mobile Medical Assistant" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "360care Inc" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Kalamazoo, MI, US" {
		t.Errorf("Location = %q, want %q", j.Location, "Kalamazoo, MI, US")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "<h2>Overview</h2>") || !strings.Contains(j.Description, "Assist physicians") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-04-15", j.PostedAt)
	}
}

func TestICIMSFiltersNonJobSitemapEntries(t *testing.T) {
	job := "https://careers-360care.icims.com/jobs/3787/mobile-medical-assistant/job"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(
			"https://careers-360care.icims.com/jobs/search",
			"https://careers-360care.icims.com/jobs/intro",
			job,
		)).
		route("/jobs/3787/mobile-medical-assistant/job?in_iframe=1", icimsDetailHTML)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Board: "360care"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "3787" {
		t.Fatalf("got %v, want only the real job (search/intro filtered)", jobs)
	}
}

func TestICIMSDropsUnavailableLocationParts(t *testing.T) {
	// A board with region == UNAVAILABLE must not render "City, UNAVAILABLE, US".
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-01-01T00:00:00.000Z",
"jobLocation":[{"address":{"addressLocality":"Austin","addressRegion":"UNAVAILABLE","addressCountry":"US"}}]}
</script></body></html>`
	loc := "https://careers-acme.icims.com/jobs/9/role/job"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(loc)).
		route("/jobs/9/role/job?in_iframe=1", detail)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "Austin, US" {
		t.Fatalf("Location = %q, want %q", jobs[0].Location, "Austin, US")
	}
}

func TestICIMSCompanyFallsBackToEntry(t *testing.T) {
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-01-01T00:00:00.000Z",
"hiringOrganization":{"name":""},
"jobLocation":[{"address":{"addressLocality":"Austin","addressCountry":"US"}}]}
</script></body></html>`
	loc := "https://careers-acme.icims.com/jobs/9/role/job"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(loc)).
		route("/jobs/9/role/job?in_iframe=1", detail)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme Corp" {
		t.Fatalf("Company = %q, want fallback %q", jobs[0].Company, "Acme Corp")
	}
}

func TestICIMSRemoteFromJobLocationType(t *testing.T) {
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Remote Role","datePosted":"2026-01-01T00:00:00.000Z",
"jobLocationType":"TELECOMMUTE",
"jobLocation":[{"address":{"addressCountry":"US"}}]}
</script></body></html>`
	loc := "https://careers-acme.icims.com/jobs/9/remote-role/job"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(loc)).
		route("/jobs/9/remote-role/job?in_iframe=1", detail)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !jobs[0].Remote {
		t.Errorf("Remote = false, want true (TELECOMMUTE)")
	}
	if jobs[0].WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", jobs[0].WorkMode)
	}
}

func TestICIMSFailedDetailDropsOnlyThatPosting(t *testing.T) {
	ok := "https://careers-acme.icims.com/jobs/111/kept/job"
	bad := "https://careers-acme.icims.com/jobs/222/dropped/job"
	// No route for /jobs/222/...?in_iframe=1 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("/sitemap.xml", icimsSitemapXML(ok, bad)).
		route("/jobs/111/kept/job?in_iframe=1", icimsDetailHTML)

	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestICIMSEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/sitemap.xml", icimsSitemapXML())
	jobs, err := NewICIMS(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
