package sources

import (
	"context"
	"testing"
)

func TestDataartVacancyCode(t *testing.T) {
	cases := map[string]string{
		"https://www.dataart.team/vacancies/sla116":       "sla116",
		"https://www.dataart.team/vacancies/jav1526":      "jav1526",
		"https://www.dataart.team/vacancies/ml00057":      "ml00057",
		"https://www.dataart.team/vacancies/go00040/":     "go00040",
		"https://www.dataart.team/es/vacancies/sla116":    "", // localisation → skipped
		"https://www.dataart.team/ua/vacancies/sla116":    "",
		"https://www.dataart.team/vacancies":              "", // listing root
		"https://www.dataart.team/events/archive":         "",
		"https://www.dataart.team/vacancies/sla116/apply": "", // deeper path, not a vacancy
	}
	for u, want := range cases {
		if got := dataartVacancyCode(u); got != want {
			t.Errorf("dataartVacancyCode(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestDataartPostingLocation(t *testing.T) {
	p := dataartPosting{JobLocation: []dataartPlace{
		{Address: dataartAddress{Locality: "Almaty", Country: "Kazakhstan"}},
		{Address: dataartAddress{Locality: "Warsaw", Country: "Poland"}},
	}}
	if got, want := p.location(), "Almaty, Kazakhstan; Warsaw, Poland"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	// Country-only place falls back to the country.
	if got, want := (dataartPosting{JobLocation: []dataartPlace{
		{Address: dataartAddress{Country: "Germany"}},
	}}).location(), "Germany"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	// Duplicate places collapse.
	if got, want := (dataartPosting{JobLocation: []dataartPlace{
		{Address: dataartAddress{Locality: "Kyiv", Country: "Ukraine"}},
		{Address: dataartAddress{Locality: "Kyiv", Country: "Ukraine"}},
	}}).location(), "Kyiv, Ukraine"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	if got := (dataartPosting{}).location(); got != "" {
		t.Errorf("location() = %q, want empty", got)
	}
}

func TestDataartPostingStripsRemoteCodes(t *testing.T) {
	// DataArt leaks internal "Remote.*" region codes into the address; location() drops them
	// (keeping a real city), skips a fully-coded place, and remote() flags the posting remote.
	p := dataartPosting{JobLocation: []dataartPlace{
		{Address: dataartAddress{Locality: "Buenos Aires", Country: "Remote.LATAM-country"}},
		{Address: dataartAddress{Locality: "Remote.AR", Country: "Remote.LATAM-country"}},
		{Address: dataartAddress{Locality: "Kyiv", Country: "Ukraine"}},
	}}
	if got, want := p.location(), "Buenos Aires; Kyiv, Ukraine"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	if !p.remote() {
		t.Error("remote() = false, want true (a Remote.* tag is present)")
	}
	if (dataartPosting{JobLocation: []dataartPlace{
		{Address: dataartAddress{Locality: "Kyiv", Country: "Ukraine"}},
	}}).remote() {
		t.Error("remote() = true, want false (no Remote.* tag)")
	}
}

// dataartDetailHTML builds a DataArt vacancy page carrying a JobPosting ld+json block with a
// jobLocation array of Place (address country + locality).
func dataartDetailHTML(title, description, datePosted string, places ...[2]string) string {
	var jl string
	for i, p := range places {
		if i > 0 {
			jl += ","
		}
		jl += `{"@type":"Place","address":{"@type":"PostalAddress","addressCountry":"` +
			p[1] + `","addressLocality":"` + p[0] + `"}}`
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":"FULL_TIME",` +
		`"jobLocation":[` + jl + `]}` +
		`</script></head><body></body></html>`
}

func dataartSitemapXML(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func TestDataartFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://www.dataart.team/vacancies/sla116"
	detail := dataartDetailHTML(
		"Shopify Solutions Architect",
		"&lt;p&gt;Lead &lt;b&gt;Shopify&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;x&lt;/script&gt;",
		"2025-05-07",
		[2]string{"Almaty", "Kazakhstan"}, [2]string{"Warsaw", "Poland"})

	fake := (&routedHTTP{}).
		route("sitemap.xml", dataartSitemapXML(
			jobURL,
			"https://www.dataart.team/es/vacancies/sla116", // localisation, skipped
			"https://www.dataart.team/vacancies",           // listing root, skipped
			"https://www.dataart.team/events/archive",      // unrelated, skipped
		)).
		route("/vacancies/sla116", detail)

	jobs, err := NewDataArt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "DataArt", Provider: "dataart",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (localised + listing URLs must be excluded)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "sla116" {
		t.Errorf("ExternalID = %q, want sla116", j.ExternalID)
	}
	if j.Title != "Shopify Solutions Architect" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "DataArt" {
		t.Errorf("Company = %q, want DataArt", j.Company)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Location != "Almaty, Kazakhstan; Warsaw, Poland" {
		t.Errorf("Location = %q", j.Location)
	}
	// sanitizeHTML keeps safe formatting tags and strips only dangerous ones (e.g. <script>).
	if want := "<p>Lead <b>Shopify</b>.</p>"; j.Description != want {
		t.Errorf("Description = %q, want %q (HTML unescaped + sanitized)", j.Description, want)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2025-05-07" {
		t.Errorf("PostedAt = %v, want 2025-05-07", j.PostedAt)
	}
	// No Remote.* tag among these places → no structured work-mode.
	if j.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (no structured remote signal)", j.WorkMode)
	}
}

func TestDataartDetailRemoteWorkMode(t *testing.T) {
	jobURL := "https://www.dataart.team/vacancies/rem001"
	detail := dataartDetailHTML("Remote Engineer", "d", "2026-01-02",
		[2]string{"Buenos Aires", "Remote.LATAM-country"})
	fake := (&routedHTTP{}).route("/vacancies/rem001", detail)

	j, ok := dataart{http: fake}.detail(context.Background(),
		CompanyEntry{Company: "DataArt", Provider: "dataart"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (structured Remote.* tag)", j.WorkMode)
	}
	if !j.Remote {
		t.Error("Remote = false, want true")
	}
	if j.Location != "Buenos Aires" {
		t.Errorf("Location = %q, want %q (Remote.* code stripped)", j.Location, "Buenos Aires")
	}
}

func TestDataartProviderAndBoardless(t *testing.T) {
	var s Source = NewDataArt(nil)
	if s.Provider() != "dataart" {
		t.Errorf("Provider() = %q, want dataart", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("dataart must be boardless (single-company, no board id)")
	}
}
