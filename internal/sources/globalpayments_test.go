package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// gpListingHTML builds a Global Payments listing page linking each given job URL.
func gpListingHTML(jobURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul class="jobs">`)
	for _, u := range jobURLs {
		b.WriteString(`<li><a href="` + u + `">A job</a></li>`)
	}
	// A non-job link (pagination/listing root) must be ignored.
	b.WriteString(`<li><a href="/en/jobs/?page=2">Next</a></li>`)
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// gpDetailHTML builds a job page carrying a JobPosting ld+json block whose jobLocation
// nests address as an array (the Global Payments shape) plus a pre-formatted Place name.
func gpDetailHTML(title, encodedDescription, datePosted, locality, region, country, placeName string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"jobLocation":[{"@type":"Place","name":"` + placeName + `","address":[{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressRegion":"` + region + `","addressCountry":"` + country + `"}]}]}` +
		`</script></head><body></body></html>`
}

func TestGPJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.globalpayments.com/en/jobs/r0072212/devops-engineer": "r0072212",
		"/en/jobs/r0071138/software-engineer-iii":                          "r0071138",
		"https://jobs.globalpayments.com/en/jobs/":                         "", // listing root
		"https://jobs.globalpayments.com/en/jobs/?page=2":                  "", // pagination
		"https://jobs.globalpayments.com/en/about":                         "",
	}
	for u, want := range cases {
		if got := gpJobID(u); got != want {
			t.Errorf("gpJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestGPJobLinksResolvesRelativeAndFiltersNonJobs(t *testing.T) {
	h := `<html><body>
		<a href="/en/jobs/r0071138/software-engineer-iii">Engineer</a>
		<a href="/en/jobs/r0071138/software-engineer-iii">Apply</a>
		<a href="https://jobs.globalpayments.com/en/jobs/r0072212/devops-engineer">DevOps</a>
		<a href="/en/jobs/?page=2">Next</a>
		<a href="/en/about">About</a>
	</body></html>`
	base := mustURL(t, "https://jobs.globalpayments.com/en/jobs/?page=1")
	got := gpJobLinks(base, parseHTML(t, h))
	want := []string{
		"https://jobs.globalpayments.com/en/jobs/r0071138/software-engineer-iii",
		"https://jobs.globalpayments.com/en/jobs/r0072212/devops-engineer",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gpJobLinks() = %v, want %v", got, want)
	}
}

func TestGPPostingLocation(t *testing.T) {
	// Structured city/region/country wins.
	p := gpPosting{JobLocation: []gpPlace{{
		Name:    "XIAN, SHAANXI, CHINA",
		Address: []gpAddress{{AddressLocality: "Xian", AddressRegion: "Shaanxi", AddressCountry: "China"}},
	}}}
	if got, want := p.location(), "Xian, Shaanxi, China"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	// Falls back to the Place name when the structured parts are empty.
	p = gpPosting{JobLocation: []gpPlace{{Name: "Remote, USA", Address: []gpAddress{{}}}}}
	if got, want := p.location(), "Remote, USA"; got != want {
		t.Errorf("location() fallback = %q, want %q", got, want)
	}
	// No location at all.
	if got := (gpPosting{}).location(); got != "" {
		t.Errorf("location() = %q, want empty", got)
	}
}

func TestGlobalPaymentsFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://jobs.globalpayments.com/en/jobs/r0072212/devops-engineer"
	detail := gpDetailHTML(
		"DevOps Engineer",
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-06-14T19:00:00-05:00", "Xian", "Shaanxi", "China", "XIAN, SHAANXI, CHINA")
	fake := (&routedHTTP{}).
		route("page=1", gpListingHTML(jobURL)).
		route("page=2", gpListingHTML()).
		route("/en/jobs/r0072212", detail)

	jobs, err := NewGlobalPayments(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Global Payments", Provider: "globalpayments", Board: "jobs.globalpayments.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "r0072212" {
		t.Errorf("ExternalID = %q, want r0072212", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "DevOps Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Global Payments" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Xian, Shaanxi, China" {
		t.Errorf("Location = %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-14T19:00:00-05:00 (= 2026-06-15T00:00Z)", j.PostedAt)
	}
}

func TestGlobalPaymentsStopsWhenPageYieldsNoNewLinks(t *testing.T) {
	// The site clamps ?page=N past its last page, serving the same links; a page with no
	// *new* links must terminate enumeration so Fetch does not loop to gpMaxPages.
	d := gpDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-14T19:00:00-05:00", "Atlanta", "GA", "USA", "ATLANTA, GA, USA")
	fake := (&routedHTTP{}).
		route("/en/jobs/?page", gpListingHTML("https://b/en/jobs/r1/a")). // every page returns the same link
		route("/en/jobs/r1", d)

	jobs, err := NewGlobalPayments(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (de-duplicated, no runaway loop)", len(jobs))
	}
}

func TestGlobalPaymentsFailedDetailDropsOnlyThatPosting(t *testing.T) {
	d := gpDetailHTML("Kept", "&lt;p&gt;x&lt;/p&gt;", "2026-06-14T19:00:00-05:00", "Atlanta", "GA", "USA", "ATLANTA, GA, USA")
	// No route for /en/jobs/r2 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("page=1", gpListingHTML("https://b/en/jobs/r1/kept", "https://b/en/jobs/r2/dropped")).
		route("page=2", gpListingHTML()).
		route("/en/jobs/r1", d)

	jobs, err := NewGlobalPayments(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "r1" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestGlobalPaymentsProvider(t *testing.T) {
	if got := NewGlobalPayments(nil).Provider(); got != "globalpayments" {
		t.Errorf("Provider() = %q, want %q", got, "globalpayments")
	}
}

func TestGlobalPaymentsRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["globalpayments"]
	if !ok {
		t.Fatal("All() missing provider globalpayments")
	}
	if s.Provider() != "globalpayments" {
		t.Errorf("All()[globalpayments].Provider() = %q", s.Provider())
	}
}
