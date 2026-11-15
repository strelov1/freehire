package sources

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ftListingHTML builds a Freshteam /jobs listing page linking each given job URL.
func ftListingHTML(jobURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs">`)
	for _, u := range jobURLs {
		b.WriteString(`<a href="` + u + `">A job</a>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// ftDetailHTML builds a Freshteam job page carrying a schema.org JobPosting ld+json block.
// Freshteam emits jobLocation as a single object (not an array) and a top-level remote bool.
func ftDetailHTML(title, encodedDescription, datePosted, region, country string, remote bool) string {
	rem := "false"
	if remote {
		rem = "true"
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":"FULL_TIME",` +
		// Freshteam emits remote as a JSON string ("true"/"false"), not a bare bool.
		`"remote":"` + rem + `",` +
		`"hiringOrganization":{"@type":"Organization","name":"Simera"},` +
		`"jobLocation":{"@type":"Place","address":{"addressRegion":"` + region + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

func TestFreshteamProvider(t *testing.T) {
	if got := NewFreshteam(nil).Provider(); got != "freshteam" {
		t.Errorf("Provider() = %q, want %q", got, "freshteam")
	}
}

func TestFTJobID(t *testing.T) {
	cases := map[string]string{
		// The live listing slugs the permalink as /jobs/<id>/<slug>; the id is the leading
		// 12-char segment regardless of any trailing slug.
		"https://simera-talent.freshteam.com/jobs/8OA37qwQgD2C/back-end-developer-br-3": "8OA37qwQgD2C",
		"/jobs/_-MH_W5oSn7f/seo-manager-bolivia":                                        "_-MH_W5oSn7f",
		"https://simera-talent.freshteam.com/jobs/-9IgOtRyhpgD":                         "-9IgOtRyhpgD", // bare id (apply page passes it as ?jobId)
		"https://simera-talent.freshteam.com/jobs":                                      "",             // listing, no id
		"https://simera-talent.freshteam.com/jobs/search":                               "",             // not a 12-char id
		"https://simera-talent.freshteam.com/about":                                     "",
	}
	for u, want := range cases {
		if got := ftJobID(u); got != want {
			t.Errorf("ftJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestFTJobLinksResolvesRelativeHrefs(t *testing.T) {
	// A relative href must still yield an absolute, fetchable URL — otherwise the detail
	// GET fails on a bare path and the posting silently drops.
	// Each card links the same job twice (heading + apply); de-dup keeps it once.
	h := `<html><body>
		<a href="/jobs/8OA37qwQgD2C/back-end-developer-br-3" class="heading">Engineer</a>
		<a href="/jobs/8OA37qwQgD2C/back-end-developer-br-3" class="apply">Apply</a>
		<a href="https://simera-talent.freshteam.com/jobs/_-MH_W5oSn7f/seo-manager-bolivia">Designer</a>
		<a href="/jobs">All jobs</a>
		<a href="/about">About</a>
	</body></html>`
	base := mustURL(t, "https://simera-talent.freshteam.com/jobs?page=1")
	got := ftJobLinks(base, parseHTML(t, h))
	want := []string{
		"https://simera-talent.freshteam.com/jobs/8OA37qwQgD2C/back-end-developer-br-3",
		"https://simera-talent.freshteam.com/jobs/_-MH_W5oSn7f/seo-manager-bolivia",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ftJobLinks() = %v, want %v", got, want)
	}
}

func TestFreshteamFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://simera-talent.freshteam.com/jobs/8OA37qwQgD2C/back-end-developer-br-3"
	detail := ftDetailHTML(
		"Back End Developer",
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-06-17 20:44:40 UTC", "San Francisco", "United States of America", true)
	fake := (&routedHTTP{}).
		route("page=1", ftListingHTML(jobURL)).
		route("page=2", ftListingHTML()).
		route("/jobs/8OA37qwQgD2C", detail)

	jobs, err := NewFreshteam(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Simera", Provider: "freshteam", Board: "simera-talent",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "8OA37qwQgD2C" {
		t.Errorf("ExternalID = %q, want 8OA37qwQgD2C", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Back End Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Simera" {
		t.Errorf("Company = %q, want Simera", j.Company)
	}
	if j.Location != "San Francisco, United States of America" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true (JSON-LD remote:true)")
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 17, 20, 44, 40, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-17 20:44:40 UTC", j.PostedAt)
	}
}

func TestFreshteamPaginatesUntilNoNewLinks(t *testing.T) {
	d := ftDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-17 20:44:40 UTC", "Berlin", "Germany", false)
	fake := (&routedHTTP{}).
		route("page=1", ftListingHTML(
			"https://b.freshteam.com/jobs/aaaaaaaaaaaa",
			"https://b.freshteam.com/jobs/bbbbbbbbbbbb")).
		route("page=2", ftListingHTML("https://b.freshteam.com/jobs/cccccccccccc")).
		route("page=3", ftListingHTML()).
		route("/jobs/aaaaaaaaaaaa", d).
		route("/jobs/bbbbbbbbbbbb", d).
		route("/jobs/cccccccccccc", d)

	jobs, err := NewFreshteam(fake).Fetch(context.Background(), CompanyEntry{Company: "B", Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (two pages enumerated)", len(jobs))
	}
}

// A detail request the crawl could not READ must not look like a posting that is not there.
// The detail page is freshteam's only source for a posting, and it is re-fetched on every
// run, so a dropped one leaves the posting missing from a crawl that reported no failure at
// all — and the stale-job sweep closes a live vacancy once the grace window elapses.
func TestFreshteamUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	d := ftDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-17 20:44:40 UTC", "Oslo", "Norway", false)
	fake := (&routedHTTP{}).
		route("page=1", ftListingHTML(
			"https://b.freshteam.com/jobs/aaaaaaaaaaaa",
			"https://b.freshteam.com/jobs/bbbbbbbbbbbb")).
		route("page=2", ftListingHTML()).
		route("/jobs/aaaaaaaaaaaa", d).
		routeErr("/jobs/bbbbbbbbbbbb", errors.New("connection reset by peer"))

	jobs, err := NewFreshteam(fake).Fetch(context.Background(), CompanyEntry{Company: "B", Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != "aaaaaaaaaaaa" {
		t.Fatalf("read = %v, want only the posting whose detail answered", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "bbbbbbbbbbbb" {
		t.Fatalf("unreadable markers = %v, want one for the posting whose detail did not", markers)
	}
	if markers[0].Company != "B" {
		t.Errorf("marker Company = %q, want the board's employer", markers[0].Company)
	}
}

// The other half of the distinction: 404 is the platform's own answer that the posting is
// gone, so the crawl drops it and the board's evidence stays complete — otherwise a board
// whose listing has gone stale could never retire anything.
func TestFreshteamGoneDetailDropsThePosting(t *testing.T) {
	d := ftDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-17 20:44:40 UTC", "Oslo", "Norway", false)
	gone := "https://b.freshteam.com/jobs/bbbbbbbbbbbb"
	fake := (&routedHTTP{}).
		route("page=1", ftListingHTML(
			"https://b.freshteam.com/jobs/aaaaaaaaaaaa",
			gone)).
		route("page=2", ftListingHTML()).
		route("/jobs/aaaaaaaaaaaa", d).
		routeErr("/jobs/bbbbbbbbbbbb", &StatusError{Method: "GET", Code: 404, URL: gone})

	jobs, err := NewFreshteam(fake).Fetch(context.Background(), CompanyEntry{Company: "B", Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "aaaaaaaaaaaa" {
		t.Fatalf("got %v, want only the kept posting — a 404 is evidence, not a hole", jobs)
	}
}

func TestFreshteamReadsRenderedDetailWhenTemplateBreaksJSONLD(t *testing.T) {
	page := `<div id="job-details-header"><h1 class="brand-color">Technical Product Manager</h1><div class="stick-hide-in-mobile">Noida, Uttar Pradesh<div>Work Type: Full Time</div></div></div><div class="job-details-content content"><div><p>Build product tools with List&lt;T&gt;.</p></div><p>Requirements: Go.</p><div>Liquid error: undefined method public_fields</div></div>`
	fake := (&routedHTTP{}).route("/jobs/QjrN6azL0OS-", page)
	job, ok := (freshteam{http: fake}).detail(context.Background(), CompanyEntry{Company: "10times"}, "https://10times.freshteam.com/jobs/QjrN6azL0OS-/technical-product-manager")
	if !ok || job.Title != "Technical Product Manager" || !strings.Contains(job.Description, "Build product tools") || strings.Contains(job.Description, "Liquid error") || !strings.Contains(job.Description, "List&lt;T&gt;") || !strings.Contains(job.Description, "Requirements: Go.") || job.Location != "Noida, Uttar Pradesh" || job.PostedAt != nil {
		t.Fatalf("rendered posting = %+v, readable=%v", job, ok)
	}
}

func TestFreshteamRenderedMarkupWithoutTextIsUnreadable(t *testing.T) {
	page := `<div id="job-details-header"><h1 class="brand-color">Engineer</h1></div><div class="job-details-content"><div></div></div>`
	fake := (&routedHTTP{}).route("/jobs/abcdefghijkl/engineer", page)
	job, ok := NewFreshteam(fake).(freshteam).detail(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"}, "https://acme.freshteam.com/jobs/abcdefghijkl/engineer")
	if !ok || !job.Unreadable {
		t.Fatalf("empty rendered posting accepted: ok=%v unreadable=%v", ok, job.Unreadable)
	}
}
