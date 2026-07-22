package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sgListingHTML builds a softgarden root listing page linking each given job href (the live
// markup emits relative "../job/…" hrefs).
func sgListingHTML(jobHrefs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobList">`)
	for _, h := range jobHrefs {
		b.WriteString(`<div class="matchValue title"><a href="` + h + `" target="_blank">A job</a></div>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// sgDetailHTML builds a softgarden job page carrying a schema.org JobPosting ld+json block.
// jobLocation is a single Place object and there is no remote flag.
func sgDetailHTML(title, encodedDescription, datePosted, locality, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":["FULL_TIME"],` +
		`"hiringOrganization":{"@type":"Organization","name":"Bundesdruckerei"},` +
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

func TestSoftgardenProvider(t *testing.T) {
	if got := NewSoftgarden(nil).Provider(); got != "softgarden" {
		t.Errorf("Provider() = %q, want %q", got, "softgarden")
	}
}

func TestSGJobID(t *testing.T) {
	cases := map[string]string{
		"https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843&l=en": "66221618",
		"../job/66221618/creative-producer?jobDbPVId=279489843":                                         "66221618",
		"https://b.softgarden.io/job/12345":                                                             "12345", // bare id, no slug/query
		"https://b.softgarden.io/":                                                                      "",      // listing root, no id
		"https://b.softgarden.io/imprint":                                                               "",
		"https://b.softgarden.io/job/abc/x":                                                             "", // non-numeric id
	}
	for u, want := range cases {
		if got := sgJobID(u); got != want {
			t.Errorf("sgJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestSGJobLinksResolvesRelativeHrefs(t *testing.T) {
	// The live listing emits relative "../job/…" hrefs; each must resolve to an absolute,
	// fetchable URL against the root, and a repeated job de-dups to one.
	h := sgListingHTML(
		"../job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"../job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"../job/70000001/data-engineer?jobDbPVId=280000000",
	) + `<a href="../imprint">Imprint</a>`
	base := mustURL(t, "https://bundesdruckerei.softgarden.io/")
	got := jobLinks(base, parseHTML(t, h), func(href string) bool { return sgJobID(href) != "" })
	want := []string{
		"https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843&l=en",
		"https://bundesdruckerei.softgarden.io/job/70000001/data-engineer?jobDbPVId=280000000",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobLinks() = %v, want %v", got, want)
	}
}

func TestSoftgardenFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://bundesdruckerei.softgarden.io/job/66221618/creative-producer?jobDbPVId=279489843"
	detail := sgDetailHTML(
		"Creative Producer",
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-07-21T12:43:08.524+02:00", "Berlin", "Deutschland")
	fake := (&routedHTTP{}).
		route("/job/66221618", detail). // detail route first: the listing root has no /job/
		route("bundesdruckerei.softgarden.io/", sgListingHTML(
			"../job/66221618/creative-producer?jobDbPVId=279489843"))

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bundesdruckerei", Provider: "softgarden", Board: "bundesdruckerei",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "66221618" {
		t.Errorf("ExternalID = %q, want 66221618", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Creative Producer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Bundesdruckerei" {
		t.Errorf("Company = %q, want Bundesdruckerei", j.Company)
	}
	if j.Location != "Berlin, Deutschland" {
		t.Errorf("Location = %q, want 'Berlin, Deutschland'", j.Location)
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 21, 10, 43, 8, 524_000_000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-21T12:43:08.524+02:00", j.PostedAt)
	}
}

func TestSoftgardenDropsDetailWithoutJobPosting(t *testing.T) {
	// A job page carrying no schema.org JobPosting drops just that posting; the rest of the
	// board still ingests.
	d := sgDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-07-21T12:43:08.524+02:00", "Oslo", "Norway")
	fake := (&routedHTTP{}).
		route("/job/111", d).                                           // first posting resolves
		route("/job/222", `<html><body>no ld+json here</body></html>`). // second has no JobPosting → drops
		route("bundesdruckerei.softgarden.io/", sgListingHTML(
			"../job/111/role-a",
			"../job/222/role-b"))

	jobs, err := NewSoftgarden(fake).Fetch(context.Background(), CompanyEntry{
		Company: "B", Board: "bundesdruckerei",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (one detail dropped)", len(jobs))
	}
}
