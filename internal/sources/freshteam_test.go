package sources

import (
	"context"
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

func TestFreshteamDropsUnfetchableDetail(t *testing.T) {
	// A detail page that 404s drops just that posting; the rest of the board still ingests.
	d := ftDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-17 20:44:40 UTC", "Oslo", "Norway", false)
	fake := (&routedHTTP{}).
		route("page=1", ftListingHTML(
			"https://b.freshteam.com/jobs/aaaaaaaaaaaa",
			"https://b.freshteam.com/jobs/bbbbbbbbbbbb")).
		route("page=2", ftListingHTML()).
		route("/jobs/aaaaaaaaaaaa", d) // no route for bbbb → GetHTML errors → drops

	jobs, err := NewFreshteam(fake).Fetch(context.Background(), CompanyEntry{Company: "B", Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (one detail dropped)", len(jobs))
	}
}
