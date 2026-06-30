package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// rapydListingHTML builds a careers-search listing page linking each position URL, plus
// non-position links (nav/footer) that must be ignored.
func rapydListingHTML(positionURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="careers-grid">`)
	for _, u := range positionURLs {
		b.WriteString(`<a href="` + u + `">A role</a>`)
	}
	b.WriteString(`<a href="/company/careers/">All openings</a>`)
	b.WriteString(`<a href="/company/about/">About</a>`)
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// rapydDetailHTML mirrors the WPBakery career page: an h1 title, a .country-term location,
// the single-career-position__main content column, and a footer link outside that column.
func rapydDetailHTML(title, location, bodyHTML string) string {
	return `<html><head>` +
		`<meta property="og:title" content="` + title + ` - ` + location + ` - Rapyd"/>` +
		`</head><body>` +
		`<div class="country-term"><h4 class="vcex-terms-grid-entry-title entry-title">` + location + `</h4></div>` +
		`<h1 class="vcex-heading"><span class="vcex-heading-inner">` + title + `</span></h1>` +
		`<div class="vc_section single-career-position__content">` +
		`<div class="wpb_column single-career-position__main">` +
		`<div class="job-details">` + bodyHTML + `</div>` +
		`</div></div>` +
		`<div class="footer"><a href="/company/partners">Partners</a></div>` +
		`</body></html>`
}

func TestRapydPositionID(t *testing.T) {
	cases := map[string]string{
		"https://www.rapyd.net/company/careers/positions/finance-data-developer-bogota-colombia/": "finance-data-developer-bogota-colombia",
		"/company/careers/positions/sales-manager-bogota-colombia/":                               "sales-manager-bogota-colombia",
		"https://www.rapyd.net/company/careers-search/":                                           "", // listing
		"https://www.rapyd.net/company/careers/":                                                  "", // nav
		"https://www.rapyd.net/company/about/":                                                    "",
	}
	for u, want := range cases {
		if got := rapydPositionID(u); got != want {
			t.Errorf("rapydPositionID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestRapydPositionLinksResolvesRelativeAndFiltersNonPositions(t *testing.T) {
	h := `<html><body>
		<a href="/company/careers/positions/sales-manager-bogota-colombia/">Sales</a>
		<a href="/company/careers/positions/sales-manager-bogota-colombia/">Apply</a>
		<a href="https://www.rapyd.net/company/careers/positions/noc-manager-tel-aviv-israel/">NOC</a>
		<a href="/company/careers/">All</a>
		<a href="/company/about/">About</a>
	</body></html>`
	base := mustURL(t, "https://www.rapyd.net/company/careers-search/")
	got := rapydPositionLinks(base, parseHTML(t, h))
	want := []string{
		"https://www.rapyd.net/company/careers/positions/sales-manager-bogota-colombia/",
		"https://www.rapyd.net/company/careers/positions/noc-manager-tel-aviv-israel/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rapydPositionLinks() = %v, want %v", got, want)
	}
}

func TestRapydFetchListingThenDetailAndMaps(t *testing.T) {
	posURL := "https://www.rapyd.net/company/careers/positions/finance-data-developer-bogota-colombia/"
	detail := rapydDetailHTML("Finance Data Analyst", "Bogota, Colombia",
		`<h3>Description</h3><p>Build <b>pipelines</b>.</p><h3>Requirements</h3><ul><li>Python</li></ul>`)
	fake := (&routedHTTP{}).
		route("careers-search", rapydListingHTML(posURL)).
		route("finance-data-developer", detail)

	jobs, err := NewRapyd(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Rapyd", Provider: "rapyd", Board: "www.rapyd.net",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "finance-data-developer-bogota-colombia" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != posURL {
		t.Errorf("URL = %q, want %q", j.URL, posURL)
	}
	if j.Title != "Finance Data Analyst" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Rapyd" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Bogota, Colombia" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "<b>pipelines</b>") || !strings.Contains(j.Description, "Python") {
		t.Errorf("Description missing job body: %q", j.Description)
	}
	if strings.Contains(j.Description, "Partners") {
		t.Errorf("Description leaked footer content: %q", j.Description)
	}
}

func TestRapydFailedDetailDropsOnlyThatPosting(t *testing.T) {
	kept := "https://www.rapyd.net/company/careers/positions/kept-role-london/"
	dropped := "https://www.rapyd.net/company/careers/positions/dropped-role-paris/"
	detail := rapydDetailHTML("Kept Role", "London, UK", `<h3>Description</h3><p>x</p>`)
	// No route for the dropped position → GetHTML errors → only that posting drops.
	fake := (&routedHTTP{}).
		route("careers-search", rapydListingHTML(kept, dropped)).
		route("kept-role-london", detail)

	jobs, err := NewRapyd(fake).Fetch(context.Background(), CompanyEntry{Company: "Rapyd", Board: "www.rapyd.net"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "kept-role-london" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestRapydProvider(t *testing.T) {
	if got := NewRapyd(nil).Provider(); got != "rapyd" {
		t.Errorf("Provider() = %q, want %q", got, "rapyd")
	}
}

func TestRapydRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["rapyd"]
	if !ok {
		t.Fatal("All() missing provider rapyd")
	}
	if s.Provider() != "rapyd" {
		t.Errorf("All()[rapyd].Provider() = %q", s.Provider())
	}
}
