package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

const compleoDetailHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"Java Tech Lead - Lisboa",
"description":"<p>Java &amp; Spring.</p><script>alert(1)<\/script>",
"hiringOrganization":{"@type":"Organization","name":"WA FENIX Serviços e Soluções em TI Ltda"},
"datePosted":"2026-05-25T03:00:00.000Z",
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressCountry":"PT"}},
"url":"https://jobs.compleo.app/wafx/jobdetail/HJ06276A",
"identifier":{"@type":"PropertyValue","name":"Compleo Job ID","value":"HJ06276A"}}
</script></head><body></body></html>`

func compleoSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestCompleoProvider(t *testing.T) {
	if got := NewCompleo(nil).Provider(); got != "compleo" {
		t.Errorf("Provider() = %q, want compleo", got)
	}
}

func TestCompleoRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["compleo"]; !ok {
		t.Error("All() should register provider compleo")
	}
	if !slices.Contains(FilterableProviders(), "compleo") {
		t.Error("FilterableProviders() should include compleo")
	}
}

// Compleo publishes one sitemap for the whole platform, so a board is a slice of it: only the
// postings under this tenant's path segment belong to the configured company.
func TestCompleoFetchTakesOnlyTheBoardsPostings(t *testing.T) {
	mine := "https://jobs.compleo.app/wafx/jobdetail/HJ06276A"
	theirs := "https://jobs.compleo.app/agiliza/jobdetail/BA00898A"
	// A tenant whose name merely starts with the board's must not be swept in with it.
	neighbour := "https://jobs.compleo.app/wafxpro/jobdetail/CC00111B"

	fake := (&routedHTTP{}).
		route("/wafx/jobdetail/HJ06276A", compleoDetailHTML).
		route("/sitemap.xml", compleoSitemapXML(mine, theirs, neighbour, "https://jobs.compleo.app/wafx/home"))

	jobs, err := NewCompleo(fake).Fetch(context.Background(), CompanyEntry{
		Company: "WA FENIX", Provider: "compleo", Board: "wafx",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (other tenants and non-posting pages filtered)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "HJ06276A" || j.URL != mine {
		t.Errorf("id/url wrong: %q %q", j.ExternalID, j.URL)
	}
	if j.Company != "WA FENIX Serviços e Soluções em TI Ltda" {
		t.Errorf("Company = %q, want the employer from the posting", j.Company)
	}
	if j.Title != "Java Tech Lead - Lisboa" {
		t.Errorf("Title = %q", j.Title)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 5, 25, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestCompleoJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.compleo.app/wafx/jobdetail/HJ06276A":        "HJ06276A",
		"https://jobs.compleo.app/huntrh.huntit/jobdetail/CI102I": "CI102I",
		"https://jobs.compleo.app/wafx/home":                      "",
	}
	for u, want := range cases {
		if got := compleoJobID(u); got != want {
			t.Errorf("compleoJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

// A board whose listed postings all fail to read must fail the crawl, not look empty; a partial
// read stays a success.
func TestCompleoFetchFailsWhenNoListedPostingReadable(t *testing.T) {
	a := "https://jobs.compleo.app/wafx/jobdetail/HJ06276A"
	b := "https://jobs.compleo.app/wafx/jobdetail/HJ06277B"
	down := &StatusError{Method: "GET", Code: 503, URL: b}
	entry := CompanyEntry{Company: "WA FENIX", Provider: "compleo", Board: "wafx"}
	sitemap := compleoSitemapXML(a, b)

	all := (&routedHTTP{}).routeErr("/jobdetail/", down).route("/sitemap.xml", sitemap)
	if _, err := NewCompleo(all).Fetch(context.Background(), entry); err == nil {
		t.Fatal("all detail reads 503: want error, got nil")
	}

	junk := (&routedHTTP{}).route("/jobdetail/", "<html><body>Oops</body></html>").route("/sitemap.xml", sitemap)
	if _, err := NewCompleo(junk).Fetch(context.Background(), entry); err == nil {
		t.Fatal("all detail pages 200 without JobPosting: want error, got nil")
	}

	some := (&routedHTTP{}).routeErr("HJ06277B", down).route("HJ06276A", compleoDetailHTML).route("/sitemap.xml", sitemap)
	jobs, err := NewCompleo(some).Fetch(context.Background(), entry)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("partial read: jobs=%d err=%v, want 1 job and nil", len(jobs), err)
	}

	empty := (&routedHTTP{}).route("/sitemap.xml", compleoSitemapXML())
	if jobs, err := NewCompleo(empty).Fetch(context.Background(), entry); err != nil || len(jobs) != 0 {
		t.Fatalf("empty sitemap: jobs=%d err=%v, want success", len(jobs), err)
	}
}
