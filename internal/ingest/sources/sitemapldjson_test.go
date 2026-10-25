package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"
)

func sitemapLDJSONURLSet(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, loc := range locs {
		b.WriteString("<url><loc>" + loc + "</loc></url>")
	}
	b.WriteString("</urlset>")
	return b.String()
}

func sitemapLDJSONIndex(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, loc := range locs {
		b.WriteString("<sitemap><loc>" + loc + "</loc></sitemap>")
	}
	b.WriteString("</sitemapindex>")
	return b.String()
}

func sitemapLDJSONPage(title, company, description, employment string) string {
	return `<!doctype html><html><head>` +
		`<script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",` +
		`"title":"` + title + `","description":"` + description + `",` +
		`"datePosted":"2026-10-01","employmentType":` + employment + `,` +
		`"jobLocationType":"TELECOMMUTE",` +
		`"hiringOrganization":{"@type":"Organization","name":"` + company + `"},` +
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"Austin","addressRegion":"TX","addressCountry":"US"}}` +
		`}</script></head><body></body></html>`
}

func gzipString(t *testing.T, raw string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestSitemapLDJSONHydratesMatchingJobPages(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/engineer"
	productURL := "https://jobs.example.com/products/widget"
	emptyURL := "https://jobs.example.com/jobs/about"
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet(jobURL, productURL, emptyURL)).
		route(jobURL, sitemapLDJSONPage("Engineer", "Example Agency", "&lt;p&gt;Build it.&lt;/p&gt;", `"FULL_TIME"`)).
		route(emptyURL, "<html><body>about</body></html>").
		route(productURL, sitemapLDJSONPage("Widget", "Shop", "A product", `"FULL_TIME"`))

	jobs, err := (sitemapldjson{http: fake, maxNew: 20}).FetchNew(
		context.Background(),
		CompanyEntry{
			Company:  "Gov Portal",
			Provider: "sitemapldjson",
			Board:    "https://jobs.example.com/sitemap.xml",
			Region:   "/jobs/",
		},
		func(string) bool { return false },
	)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (product filtered, about page skipped)", len(jobs))
	}
	j := jobs[0]
	if j.Title != "Engineer" || j.Company != "Example Agency" {
		t.Errorf("title/company = %q/%q", j.Title, j.Company)
	}
	if j.ExternalID != jobURL || j.URL != jobURL {
		t.Errorf("id/url = %q/%q", j.ExternalID, j.URL)
	}
	if !strings.Contains(j.Description, "Build it.") {
		t.Errorf("description = %q", j.Description)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("work mode = %q remote=%v", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment = %q", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2026 {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if !strings.Contains(j.Location, "Austin") {
		t.Errorf("location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("countries = %v", j.Countries)
	}
	if fake.calls != 3 {
		t.Errorf("calls = %d, want 3 (sitemap + job + about; product not fetched)", fake.calls)
	}
}

func TestSitemapLDJSONCountriesStayWithJobLocation(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/1"
	page := strings.ReplaceAll(
		sitemapLDJSONPage("Engineer", "Example", "desc", `"FULL_TIME"`),
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"Austin","addressRegion":"TX","addressCountry":"US"}}`,
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"Austin","addressRegion":"TX","addressCountry":"US"}},`+
			`"applicantLocationRequirements":{"@type":"Country","name":"Canada"}`,
	)
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet(jobURL)).
		route(jobURL, page)
	jobs, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Example", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	if len(jobs[0].Countries) != 1 || jobs[0].Countries[0] != "us" {
		t.Errorf("countries = %v, want [us]", jobs[0].Countries)
	}
}

func TestSitemapLDJSONCompanyFallsBackToBoard(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/1"
	page := strings.ReplaceAll(
		sitemapLDJSONPage("Role", "SHOULD NOT APPEAR", "desc", `"FULL_TIME"`),
		`"hiringOrganization":{"@type":"Organization","name":"SHOULD NOT APPEAR"}`,
		`"hiringOrganization":{"@type":"Organization","name":""}`,
	)
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet(jobURL)).
		route(jobURL, page)

	jobs, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Gov Portal", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Gov Portal" {
		t.Fatalf("jobs = %+v, want board company", jobs)
	}
}

func TestSitemapLDJSONSeenSkipsDetail(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/engineer"
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet(jobURL)).
		routeErr(jobURL, errSentinel("detail must not be fetched"))

	jobs, err := (sitemapldjson{http: fake, maxNew: 5}).FetchNew(
		context.Background(),
		CompanyEntry{Company: "Example", Board: "https://jobs.example.com/sitemap.xml"},
		func(id string) bool { return id == jobURL },
	)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh || jobs[0].ExternalID != jobURL {
		t.Fatalf("jobs = %+v, want one SeenRefresh", jobs)
	}
}

func TestSitemapLDJSONBoundsNewDetails(t *testing.T) {
	locs := make([]string, 5)
	for i := range locs {
		locs[i] = "https://jobs.example.com/jobs/" + string(rune('a'+i))
	}
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet(locs...)).
		route("/jobs/", sitemapLDJSONPage("Role", "Co", "desc", `"FULL_TIME"`))

	jobs, err := (sitemapldjson{http: fake, maxNew: 2}).FetchNew(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "https://jobs.example.com/sitemap.xml"},
		func(string) bool { return false },
	)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	if fake.calls != 3 {
		t.Errorf("calls = %d, want 3 (sitemap + 2 details)", fake.calls)
	}
}

func TestSitemapLDJSONReadsGzipAndIndex(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/engineer"
	child := "https://jobs.example.com/jobs.xml.gz"
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONIndex(child)).
		route("jobs.xml.gz", gzipString(t, sitemapLDJSONURLSet(jobURL))).
		route(jobURL, sitemapLDJSONPage("Engineer", "Co", "desc", `["FULL_TIME","OTHER"]`))

	jobs, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Engineer" || jobs[0].EmploymentType != "full_time" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestSitemapLDJSONChildSitemapFailureFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONIndex("https://jobs.example.com/jobs.xml")).
		routeErr("jobs.xml", errSentinel("origin down"))

	_, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err == nil {
		t.Fatal("Fetch error = nil, want child sitemap failure")
	}
}

func TestSitemapLDJSONRejectsANonSitemapAndANonURL(t *testing.T) {
	fake := (&routedHTTP{}).route("sitemap.xml", "<html><body>nope</body></html>")
	_, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err == nil {
		t.Fatal("Fetch error = nil, want non-sitemap body")
	}
	_, err = (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "not-a-url"},
	)
	if err == nil {
		t.Fatal("Fetch error = nil, want rejected board")
	}
}

func TestSitemapLDJSONResolvesRelativeLocs(t *testing.T) {
	jobURL := "https://jobs.example.com/jobs/engineer"
	fake := (&routedHTTP{}).
		route("sitemap.xml", sitemapLDJSONURLSet("/jobs/engineer?utm=1")).
		route(jobURL, sitemapLDJSONPage("Engineer", "Co", "desc", `"FULL_TIME"`))

	jobs, err := (sitemapldjson{http: fake, maxNew: 5}).Fetch(
		context.Background(),
		CompanyEntry{Company: "Co", Board: "https://jobs.example.com/sitemap.xml"},
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	if jobs[0].ExternalID != jobURL {
		t.Errorf("id = %q, want query stripped", jobs[0].ExternalID)
	}
	if jobs[0].URL != "https://jobs.example.com/jobs/engineer?utm=1" {
		t.Errorf("url = %q, want the sitemap loc with its query", jobs[0].URL)
	}
}

func TestSitemapLDJSONRegisteredAsHydratingBoard(t *testing.T) {
	src, ok := All(nil)["sitemapldjson"]
	if !ok {
		t.Fatal("All() missing sitemapldjson")
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Fatal("sitemapldjson is not a HydratingSource")
	}
	if _, ok := src.(fullBoardListing); ok {
		t.Fatal("sitemapldjson claims fullBoardListing")
	}
	if kind := ProviderKind(All(nil), "sitemapldjson"); kind != KindATS {
		t.Fatalf("ProviderKind = %q, want %q", kind, KindATS)
	}
}

func TestURLMatchesPattern(t *testing.T) {
	const raw = "https://jobs.example.com/jobs/engineer"
	cases := []struct {
		pattern string
		want    bool
	}{
		{"", true},
		{"/jobs/", true},
		{"/products/", false},
		{"*/jobs/*", true},
		{"https://jobs.example.com/jobs/*", true},
		{"https://other.example.com/*", false},
		{"*/engineer", true},
		{"*/missing", false},
	}
	if !urlMatchesPattern("https://engineering.example.com/jobs/senior-engineer", "*engineer") {
		t.Error("urlMatchesPattern(*engineer) missed a later engineer after an earlier prefix")
	}
	for _, tc := range cases {
		if got := urlMatchesPattern(raw, tc.pattern); got != tc.want {
			t.Errorf("urlMatchesPattern(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
