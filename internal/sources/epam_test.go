package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// epamNextDataScript builds the __NEXT_DATA__ script EPAM's vacancy pages embed, carrying the
// structured description slices (intro HTML, category bullet arrays, benefits HTML) that the
// flattened ld+json description drops. Marshaled via encoding/json so the fixture cannot drift
// from valid JSON.
func epamNextDataScript(intro string, resp, req, nice []string, benefitsHTML string) string {
	payload := map[string]any{
		"props": map[string]any{
			"pageProps": map[string]any{
				"job": map[string]any{
					"description": intro,
					"category": map[string]any{
						"responsibilities": resp,
						"requirements":     req,
						"nice_to_have":     nice,
					},
					"benefits": []map[string]any{{"content": benefitsHTML}},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return `<script id="__NEXT_DATA__" type="application/json">` + string(b) + `</script>`
}

// epamSitemapXML builds the (gzip-decoded) sitemap urlset linking the given <loc> URLs.
func epamSitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset>`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// epamDetailHTML builds an EPAM vacancy page carrying a JobPosting ld+json block. EPAM
// emits no jobLocation; location comes from applicantLocationRequirements (an array of
// Country) and the remote flag from jobLocationType ("TELECOMMUTE").
func epamDetailHTML(title, description, datePosted, locType string, countries ...string) string {
	var alr strings.Builder
	for i, c := range countries {
		if i > 0 {
			alr.WriteString(",")
		}
		alr.WriteString(`{"@type":"Country","name":"` + c + `"}`)
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"jobLocationType":"` + locType + `",` +
		`"applicantLocationRequirements":[` + alr.String() + `],` +
		`"identifier":{"@type":"PropertyValue","name":"EPAM Systems","value":"ignored"}}` +
		`</script></head><body></body></html>`
}

func TestEPAMJobID(t *testing.T) {
	cases := map[string]string{
		"https://careers.epam.com/en/vacancy/abap-software-engineer-bltmoen02larol38uw0_en": "bltmoen02larol38uw0",
		"https://careers.epam.com/en/vacancy/abap-tech-lead-blt17cb77be1b13b884_en":         "blt17cb77be1b13b884",
		"https://careers.epam.com/uk/vacancy/x-bltabc123_uk":                                "", // non-English → filtered out (no id)
		"https://careers.epam.com/en/jobs":                                                  "", // listing
		"https://careers.epam.com/en":                                                       "",
	}
	for u, want := range cases {
		if got := epamJobID(u); got != want {
			t.Errorf("epamJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestEPAMPostingLocation(t *testing.T) {
	p := epamPosting{ApplicantLocationRequirements: []epamCountry{{Name: "Colombia"}, {Name: "Mexico"}}}
	if got, want := p.location(), "Colombia, Mexico"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	if got := (epamPosting{}).location(); got != "" {
		t.Errorf("location() = %q, want empty", got)
	}
}

func TestEPAMFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://careers.epam.com/en/vacancy/data-technology-consultant-blt01b3u51rnautbmxq_en"
	detail := epamDetailHTML(
		"Data Technology Consultant",
		"&lt;p&gt;Lead &lt;b&gt;data&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;x&lt;/script&gt;",
		"2026-06-18", "TELECOMMUTE", "Colombia", "Mexico")
	fake := (&routedHTTP{}).
		route("sitemap.xml.gz", epamSitemapXML(jobURL)).
		route("/en/vacancy/data-technology-consultant-blt01b3u51rnautbmxq_en", detail)

	jobs, err := NewEPAM(fake).Fetch(context.Background(), CompanyEntry{
		Company: "EPAM Systems", Provider: "epam", Board: "careers.epam.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "blt01b3u51rnautbmxq" {
		t.Errorf("ExternalID = %q, want blt01b3u51rnautbmxq", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Data Technology Consultant" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "EPAM Systems" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Colombia, Mexico" {
		t.Errorf("Location = %q, want %q", j.Location, "Colombia, Mexico")
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote (jobLocationType TELECOMMUTE)", j.Remote, j.WorkMode)
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>data</b>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-18", j.PostedAt)
	}
}

func TestEPAMDescriptionFromNextDataStructured(t *testing.T) {
	jobURL := "https://careers.epam.com/en/vacancy/senior-full-stack-developer-bltx6xf2xw5owhbw1rh_en"
	nd := epamNextDataScript(
		"<p>We are seeking a talented <strong>Senior Full Stack Developer</strong>.</p>",
		[]string{"Design and maintain backend services", "Build user interfaces with React"},
		[]string{"At least 3 years of experience"},
		[]string{"Experience with Docker and Kubernetes"},
		"<ul><li>International projects with top brands</li><li>Healthcare benefits</li></ul>",
	)
	// The ld+json still carries the flat, structure-less blob EPAM emits; the adapter must
	// prefer the structured __NEXT_DATA__ payload over it.
	detail := strings.Replace(
		epamDetailHTML("Senior Full Stack Developer",
			"Flat blob Responsibilities Design and maintain Requirements At least 3",
			"2026-07-16", "TELECOMMUTE", "Argentina"),
		"</body>", nd+"</body>", 1)
	fake := (&routedHTTP{}).
		route("sitemap.xml.gz", epamSitemapXML(jobURL)).
		route("/en/vacancy/senior-full-stack-developer-bltx6xf2xw5owhbw1rh_en", detail)

	jobs, err := NewEPAM(fake).Fetch(context.Background(), CompanyEntry{Company: "EPAM Systems", Board: "careers.epam.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	d := jobs[0].Description
	for _, want := range []string{
		"<strong>Senior Full Stack Developer</strong>", // intro HTML preserved
		"<h3>Responsibilities</h3>",
		"<li>Build user interfaces with React</li>",
		"<h3>Requirements</h3>",
		"<h3>Nice to have</h3>",
		"<li>Experience with Docker and Kubernetes</li>",
		"Healthcare benefits", // benefits section recovered (absent from ld+json)
	} {
		if !strings.Contains(d, want) {
			t.Errorf("Description missing %q\n got: %s", want, d)
		}
	}
	if strings.Contains(d, "Flat blob") {
		t.Errorf("Description used flat ld+json blob instead of structured __NEXT_DATA__: %s", d)
	}
}

func TestEPAMDescriptionDropsLeakedSectionLabels(t *testing.T) {
	// EPAM's own data leaks the next section's heading as the last bullet of the previous
	// list (the artifact that also duplicates "Requirements Requirements" in the ld+json).
	j := epamJob{}
	j.Category.Responsibilities = []string{"Build services", "Requirements"}
	j.Category.Requirements = []string{"3 years experience", "Nice to have"}
	d := j.descriptionHTML()
	if strings.Contains(d, "<li>Requirements</li>") || strings.Contains(d, "<li>Nice to have</li>") {
		t.Errorf("leaked section label rendered as a bullet: %s", d)
	}
	if !strings.Contains(d, "<li>Build services</li>") || !strings.Contains(d, "<li>3 years experience</li>") {
		t.Errorf("dropped a real bullet: %s", d)
	}
}

func TestEPAMFiltersNonEnglishAndNonVacancyURLs(t *testing.T) {
	en := "https://careers.epam.com/en/vacancy/role-blt111aaa_en"
	detail := epamDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-18", "TELECOMMUTE", "Poland")
	fake := (&routedHTTP{}).
		route("sitemap.xml.gz", epamSitemapXML(
			"https://careers.epam.com/en",
			"https://careers.epam.com/en/jobs",
			"https://careers.epam.com/uk/vacancy/role-blt222bbb_uk", // non-English vacancy → skip
			en,
		)).
		route("/en/vacancy/role-blt111aaa_en", detail)

	jobs, err := NewEPAM(fake).Fetch(context.Background(), CompanyEntry{Company: "EPAM Systems", Board: "careers.epam.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "blt111aaa" {
		t.Fatalf("got %v, want only the single English vacancy", jobs)
	}
}

func TestEPAMFailedDetailDropsOnlyThatPosting(t *testing.T) {
	kept := "https://careers.epam.com/en/vacancy/kept-blt1keep_en"
	dropped := "https://careers.epam.com/en/vacancy/dropped-blt2drop_en"
	detail := epamDetailHTML("Kept", "&lt;p&gt;x&lt;/p&gt;", "2026-06-18", "TELECOMMUTE", "Poland")
	// No route for the dropped vacancy → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("sitemap.xml.gz", epamSitemapXML(kept, dropped)).
		route("/en/vacancy/kept-blt1keep_en", detail)

	jobs, err := NewEPAM(fake).Fetch(context.Background(), CompanyEntry{Company: "EPAM Systems", Board: "careers.epam.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "blt1keep" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestEPAMProvider(t *testing.T) {
	if got := NewEPAM(nil).Provider(); got != "epam" {
		t.Errorf("Provider() = %q, want %q", got, "epam")
	}
}

func TestEPAMRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["epam"]
	if !ok {
		t.Fatal("All() missing provider epam")
	}
	if s.Provider() != "epam" {
		t.Errorf("All()[epam].Provider() = %q", s.Provider())
	}
}
