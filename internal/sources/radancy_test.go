package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// radancyDetailHTML is a Radancy (TalentBrew) job page: server-rendered HTML whose only
// payload we read is the schema.org JobPosting ld+json. datePosted is non-zero-padded
// ("2026-6-17", as Radancy emits) and the description embeds a <script> (written as
// <\/script> so the JSON string carries it) that sanitizeHTML must strip.
const radancyDetailHTML = `<html><head></head><body>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"JobPosting",
"title":"Regional Account Manager",
"description":"<p>Sell things.</p><script>alert(1)<\/script>",
"datePosted":"2026-6-17",
"identifier":"R-254716",
"hiringOrganization":{"@type":"Organization","name":"AstraZeneca"},
"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"Lafayette","addressRegion":"Louisiana","addressCountry":"United States"}}]}
</script>
</body></html>`

// radancySitemapXML builds a Radancy sitemap urlset from the given locs.
func radancySitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestRadancyProvider(t *testing.T) {
	if got := NewRadancy(nil).Provider(); got != "radancy" {
		t.Errorf("Provider() = %q, want %q", got, "radancy")
	}
}

func TestRadancyJobID(t *testing.T) {
	cases := map[string]string{
		"https://careers.astrazeneca.com/job/lafayette/regional-account-manager/43991/96556869600": "96556869600",
		"https://careers.ing.com/en/job/bucharest/chapter-lead-engineer/3121/39724266176":          "39724266176",
		"https://careers.ing.com/en/category/it-engineering-jobs/2618/32177152/1":                  "", // category, not a job
		"https://careers.ing.com": "",
	}
	for loc, want := range cases {
		if got := radancyJobID(loc); got != want {
			t.Errorf("radancyJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestRadancyFetchSitemapThenDetailAndMaps(t *testing.T) {
	loc := "https://careers.astrazeneca.com/job/lafayette/regional-account-manager/43991/96556869600"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", radancySitemapXML(loc)).
		route("/job/lafayette/regional-account-manager/43991/96556869600", radancyDetailHTML)

	jobs, err := NewRadancy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "AstraZeneca", Provider: "radancy", Board: "careers.astrazeneca.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "96556869600" {
		t.Errorf("ExternalID = %q, want 96556869600", j.ExternalID)
	}
	if j.URL != loc {
		t.Errorf("URL = %q, want %q", j.URL, loc)
	}
	if j.Title != "Regional Account Manager" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "AstraZeneca" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Lafayette, Louisiana, United States" {
		t.Errorf("Location = %q, want %q", j.Location, "Lafayette, Louisiana, United States")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Sell things") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-17 (non-zero-padded source)", j.PostedAt)
	}
}

func TestRadancyDropsNumericRegion(t *testing.T) {
	// ING emits addressRegion "10" (a numeric code) — it must not render "Bucharest, 10, RO".
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-6-17",
"jobLocation":[{"address":{"addressLocality":"Bucharest","addressRegion":"10","addressCountry":"RO"}}]}
</script></body></html>`
	loc := "https://careers.ing.com/en/job/bucharest/role/3121/39724266176"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", radancySitemapXML(loc)).
		route("/en/job/bucharest/role/3121/39724266176", detail)

	jobs, err := NewRadancy(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.ing.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "Bucharest, RO" {
		t.Fatalf("Location = %q, want %q", jobs[0].Location, "Bucharest, RO")
	}
}

func TestRadancyCompanyFallsBackToEntry(t *testing.T) {
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-6-17",
"hiringOrganization":{"name":""},
"jobLocation":[{"address":{"addressLocality":"Austin","addressCountry":"US"}}]}
</script></body></html>`
	loc := "https://careers.acme.com/job/austin/role/1/9"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", radancySitemapXML(loc)).
		route("/job/austin/role/1/9", detail)

	jobs, err := NewRadancy(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "careers.acme.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme Corp" {
		t.Fatalf("Company = %q, want fallback %q", jobs[0].Company, "Acme Corp")
	}
}

func TestRadancyFiltersNonJobSitemapEntries(t *testing.T) {
	job := "https://careers.ing.com/en/job/bucharest/role/3121/39724266176"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", radancySitemapXML(
			"https://careers.ing.com",
			"https://careers.ing.com/en/category/it-engineering-jobs/2618/32177152/1",
			job,
		)).
		route("/en/job/bucharest/role/3121/39724266176", radancyDetailHTML)

	jobs, err := NewRadancy(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.ing.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "39724266176" {
		t.Fatalf("got %v, want only the real job (landing/category filtered)", jobs)
	}
}

func TestRadancyEmptySitemapYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/sitemap.xml", radancySitemapXML())
	jobs, err := NewRadancy(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.acme.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
