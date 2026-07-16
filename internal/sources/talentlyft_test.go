package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// talentlyftSitemapXML is a TalentLyft tenant sitemap: <loc> entries for postings
// (/jobs/<slug>-<code>) plus the listing root, which carries no code and must be skipped.
func talentlyftSitemapXML(host string, jobPaths ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, p := range jobPaths {
		b.WriteString(`<url><loc>https://` + host + p + `</loc></url>`)
	}
	b.WriteString(`<url><loc>https://` + host + `</loc></url></urlset>`)
	return b.String()
}

// talentlyftDetailHTML is a TalentLyft posting page carrying a schema.org JobPosting ld+json.
// The description embeds a <script> (escaped) that sanitizeHTML must strip.
func talentlyftDetailHTML(title string) string {
	return `<html><body>
<script type="application/ld+json">
{"context":"https://schema.org/","@type":"JobPosting","title":"` + title + `",
"description":"<p>Build eddy current instruments.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-23","employmentType":"FULL_TIME",
"hiringOrganization":{"@type":"Organization","name":"INETEC"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"Donji Stupnik","addressCountry":"Croatia"}}}
</script></body></html>`
}

func TestTalentLyftProvider(t *testing.T) {
	if got := NewTalentLyft(nil).Provider(); got != "talentlyft" {
		t.Errorf("Provider() = %q, want talentlyft", got)
	}
}

func TestTalentLyftJobID(t *testing.T) {
	cases := map[string]string{
		"https://inetec.talentlyft.com/jobs/senior-electronics-engineer-cjVz":                 "cjVz",
		"https://flyer.talentlyft.com/matchmatch/jobs/creative-marketing-manager-attain-chMH": "chMH",
		"https://inetec.talentlyft.com/jobs/role-cj4t?utm=x":                                  "cj4t",
		"https://inetec.talentlyft.com":                                                       "",
		"https://flyer.talentlyft.com/matchmatch":                                             "",
	}
	for loc, want := range cases {
		if got := talentlyftJobID(loc); got != want {
			t.Errorf("talentlyftJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestTalentLyftFetchSitemapThenDetailAndMaps(t *testing.T) {
	host := "inetec.talentlyft.com"
	fake := (&routedHTTP{}).
		route("sitemap.xml", talentlyftSitemapXML(host, "/jobs/senior-electronics-engineer-cjVz")).
		route("/jobs/senior-electronics-engineer-cjVz", talentlyftDetailHTML("Senior Electronics Engineer"))

	jobs, err := NewTalentLyft(fake).Fetch(context.Background(), CompanyEntry{
		Company: "INETEC d.o.o.", Provider: "talentlyft", Board: "inetec",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (root skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "cjVz" {
		t.Errorf("ExternalID = %q, want cjVz", j.ExternalID)
	}
	if j.URL != "https://inetec.talentlyft.com/jobs/senior-electronics-engineer-cjVz" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Electronics Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "INETEC d.o.o." {
		t.Errorf("Company = %q, want configured company", j.Company)
	}
	if j.Location != "Donji Stupnik, Croatia" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-23", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "eddy current") {
		t.Errorf("Description lost body: %q", j.Description)
	}
}

func TestTalentLyftSitemapErrorIsBoardError(t *testing.T) {
	if _, err := NewTalentLyft(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "inetec"}); err == nil {
		t.Fatal("want a board-level error when the sitemap fails")
	}
}

func TestTalentLyftRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["talentlyft"]
	if !ok {
		t.Fatal("All() missing provider talentlyft")
	}
	if s.Provider() != "talentlyft" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "talentlyft") {
		t.Error("FilterableProviders() should include talentlyft")
	}
}
