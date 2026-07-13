package sources

import (
	"context"
	"strings"
	"testing"
)

// twogisListingHTML is a 2GIS careers listing page (job.2gis.ru/vacancies). It carries the given
// vacancy paths as /vacancies/<category>/<id> anchors, plus the /vacancies listing root and an
// /about nav anchor that must NOT be treated as postings — exercising the job-link predicate.
func twogisListingHTML(paths ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="VacancyList">`)
	for _, p := range paths {
		b.WriteString(`<a class="VacancyItem-module__root" href="/vacancies/` + p + `">A Role</a>`)
	}
	b.WriteString(`<a href="/vacancies">All vacancies</a>`)
	b.WriteString(`<a href="/about">About</a>`)
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// twogisRemoteDetailHTML is a 2GIS vacancy page whose payload is the schema.org JobPosting ld+json:
// jobLocationType TELECOMMUTE (structured remote), NO jobLocation, a nested PropertyValue
// identifier, and a description embedding a <script> that sanitizeHTML must strip.
func twogisRemoteDetailHTML(title, cat, id string) string {
	return `<html><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"description":"<p>Build maps.</p><script>alert(1)<\/script>",
"identifier":{"@type":"PropertyValue","name":"2ГИС","value":"` + id + `"},
"hiringOrganization":{"@type":"Organization","name":"2ГИС"},
"url":"/vacancies/` + cat + `/` + id + `",
"jobLocationType":"TELECOMMUTE"}
</script></body></html>`
}

// twogisOnsiteDetailHTML is a 2GIS vacancy page for an onsite role: no jobLocationType, and a
// SINGLE jobLocation object (not an array) with a city — proving the flexible single-or-array
// decode and that a located role is not flagged remote.
func twogisOnsiteDetailHTML(title, cat, id, city string) string {
	return `<html><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `","description":"<p>On site.</p>",
"identifier":{"@type":"PropertyValue","name":"2ГИС","value":"` + id + `"},
"url":"/vacancies/` + cat + `/` + id + `",
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` + city + `"}}}
</script></body></html>`
}

func TestTwoGISProvider(t *testing.T) {
	if got := NewTwoGIS(nil).Provider(); got != "2gis" {
		t.Errorf("Provider() = %q, want %q", got, "2gis")
	}
}

func TestTwoGISJobID(t *testing.T) {
	cases := map[string]string{
		"https://job.2gis.ru/vacancies/testing/406":       "406",
		"https://job.2gis.ru/vacancies/saless/362?utm=x":  "362",
		"https://job.2gis.ru/vacancies/infra_admin/329#a": "329",
		"https://job.2gis.ru/vacancies/testing":           "", // category only, no id
		"https://job.2gis.ru/vacancies":                   "", // listing root
		"https://job.2gis.ru/about":                       "",
	}
	for in, want := range cases {
		if got := twogisJobID(in); got != want {
			t.Errorf("twogisJobID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTwoGISFetchListingThenDetailAndMaps(t *testing.T) {
	// Detail routes come first: the listing URL (.../vacancies) is a substring of every detail
	// URL (.../vacancies/<cat>/<id>), and routedHTTP matches by first-containing route, so the
	// specific detail paths must precede the generic listing route.
	fake := (&routedHTTP{}).
		route("/vacancies/testing/406", twogisRemoteDetailHTML("QA Engineer", "testing", "406")).
		route("/vacancies/saless/362", twogisOnsiteDetailHTML("Regional Trainer", "saless", "362", "Томск")).
		route("/vacancies", twogisListingHTML("testing/406", "saless/362"))

	jobs, err := NewTwoGIS(fake).Fetch(context.Background(), CompanyEntry{
		Company: "2GIS", Provider: "2gis",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	remote, ok := byID["406"]
	if !ok {
		t.Fatal("missing job 406")
	}
	if remote.URL != "https://job.2gis.ru/vacancies/testing/406" {
		t.Errorf("URL = %q, want canonical detail URL", remote.URL)
	}
	if remote.Title != "QA Engineer" {
		t.Errorf("Title = %q", remote.Title)
	}
	// The configured company is canonical (this is 2GIS's own site); the JSON-LD
	// hiringOrganization is ignored so the board never mislabels its postings.
	if remote.Company != "2GIS" {
		t.Errorf("Company = %q, want configured company", remote.Company)
	}
	if !remote.Remote || remote.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want structured remote from TELECOMMUTE", remote.Remote, remote.WorkMode)
	}
	if remote.Location != "" {
		t.Errorf("Location = %q, want empty (no jobLocation)", remote.Location)
	}
	if strings.Contains(remote.Description, "<script>") || strings.Contains(remote.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", remote.Description)
	}
	if !strings.Contains(remote.Description, "Build maps") {
		t.Errorf("Description lost real content: %q", remote.Description)
	}

	onsite, ok := byID["362"]
	if !ok {
		t.Fatal("missing job 362")
	}
	if onsite.Location != "Томск" {
		t.Errorf("Location = %q, want %q", onsite.Location, "Томск")
	}
	if onsite.Remote || onsite.WorkMode != "" {
		t.Errorf("Remote=%v WorkMode=%q, want not-remote for a located onsite role", onsite.Remote, onsite.WorkMode)
	}
}

func TestTwoGISListingErrorIsBoardLevel(t *testing.T) {
	// An empty router errors on the listing fetch, which must surface as a board-level error
	// (not a silent empty result).
	_, err := NewTwoGIS(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Company: "2GIS", Provider: "2gis",
	})
	if err == nil {
		t.Fatal("Fetch: want board-level error on listing failure, got nil")
	}
}
