package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

func itechartSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func itechartDetailHTML(title, desc, datePosted, locality string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `","description":"` + desc + `","datePosted":"` + datePosted + `",` +
		`"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` + locality + `"}}}` +
		`</script></head><body></body></html>`
}

func TestITechArtJobID(t *testing.T) {
	cases := map[string]string{
		"https://itechartgroup.by/job-openings/ai-solution-architect-minsk": "ai-solution-architect-minsk",
		"https://itechartgroup.by/job-openings":                             "", // listing root
		"https://itechartgroup.by/about":                                    "",
	}
	for u, want := range cases {
		if got := itechartJobID(u); got != want {
			t.Errorf("itechartJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestITechArtFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://itechartgroup.by/job-openings/ai-solution-architect-minsk"
	fake := (&routedHTTP{}).
		route("sitemap.xml", itechartSitemapXML("https://itechartgroup.by/job-openings", jobURL)).
		route("/job-openings/ai-solution-architect-minsk", itechartDetailHTML(
			"AI Solution Architect", "&lt;ul&gt;&lt;li&gt;build&lt;/li&gt;&lt;/ul&gt;&lt;script&gt;x&lt;/script&gt;",
			"2026-06-14T10:54:08+03:00", "Минск"))

	jobs, err := NewITechArt(fake).Fetch(context.Background(), CompanyEntry{Company: "iTechArt", Board: "itechartgroup.by"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (listing root filtered out)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "ai-solution-architect-minsk" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "AI Solution Architect" || j.Company != "iTechArt" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Минск" {
		t.Errorf("Location = %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<li>build</li>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 14, 7, 54, 8, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestITechArtProvider(t *testing.T) {
	if got := NewITechArt(nil).Provider(); got != "itechart" {
		t.Errorf("Provider() = %q, want itechart", got)
	}
}

func TestITechArtRegisteredInAll(t *testing.T) {
	if s, ok := All(nil)["itechart"]; !ok || s.Provider() != "itechart" {
		t.Fatal("All() missing provider itechart")
	}
}
