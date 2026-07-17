package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// baytDetailHTML renders a Bayt job page the way bayt.com does: one application/ld+json
// JobPosting. jobLocation is a single Place object (not an array), and its address.* fields
// ARE reliable on Bayt (unlike Meta), carrying an ISO addressCountry the geo dictionary reads.
// A company of "" omits hiringOrganization, exercising the drop-a-company-less-posting path.
func baytDetailHTML(title, datePosted, company, locality, country string) string {
	org := ""
	if company != "" {
		org = `"hiringOrganization":{"@type":"Organization","name":"` + company + `"},`
	}
	// The closing script tag inside the JSON is escaped as <\/script> exactly as Bayt
	// renders it, so the embedded ld+json block is not terminated early by the HTML parser.
	return `<html><head><script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting",
"title":"` + title + `",
"description":"<p>Do the thing.<\/p><script>alert(1)<\/script>",
"identifier":{"@type":"PropertyValue","name":"` + company + `","value":"2273240"},
"datePosted":"` + datePosted + `",
"employmentType":"FULL_TIME",
` + org + `
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}
</script></head><body>page</body></html>`
}

// baytListingHTML renders a country listing page: a set of anchors to job-detail pages.
func baytListingHTML(hrefs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul>`)
	for _, h := range hrefs {
		b.WriteString(`<li><a href="` + h + `">job</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

func TestBaytProvider(t *testing.T) {
	if got := NewBayt(nil).Provider(); got != "bayt" {
		t.Errorf("Provider() = %q, want %q", got, "bayt")
	}
}

func TestBaytJobID(t *testing.T) {
	cases := map[string]string{
		"/en/saudi-arabia/jobs/quality-control-officer-5466655/": "5466655",
		"https://www.bayt.com/en/uae/jobs/senior-dev-42/":        "42",
		"/en/saudi-arabia/jobs/tracked-role-88/?utm_source=x":    "88", // query stripped
		"/en/saudi-arabia/jobs/no-trailing-id/":                  "",
		"/en/saudi-arabia/companies/some-company-123/":           "",
	}
	for loc, want := range cases {
		if got := baytJobID(loc); got != want {
			t.Errorf("baytJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestBaytAbsURLResolvesHrefForms(t *testing.T) {
	const want = "https://www.bayt.com/en/saudi-arabia/jobs/senior-dev-42/"
	cases := map[string]string{
		"/en/saudi-arabia/jobs/senior-dev-42/":                     want, // root-relative
		"https://www.bayt.com/en/saudi-arabia/jobs/senior-dev-42/": want, // already absolute
		// protocol-relative: keeps its own host; the old "http" prefix guess mis-built
		// "https://www.bayt.com//www.bayt.com/…".
		"//www.bayt.com/en/saudi-arabia/jobs/senior-dev-42/": want,
	}
	for href, w := range cases {
		if got := baytAbsURL(href); got != w {
			t.Errorf("baytAbsURL(%q) = %q, want %q", href, got, w)
		}
	}
}

func TestBaytRegisteredAndFacet(t *testing.T) {
	if _, ok := All(nil)["bayt"]; !ok {
		t.Fatal("bayt not registered in sources.All")
	}
	// bayt is a multi-company aggregator crawl, so it must be offered as a source facet.
	found := false
	for _, p := range FilterableProviders() {
		if p == "bayt" {
			found = true
		}
	}
	if !found {
		t.Error("bayt should appear in the source facet")
	}
}

func TestBaytFetchListingThenDetailAndMaps(t *testing.T) {
	href := "/en/saudi-arabia/jobs/quality-control-officer-5466655/"
	fake := (&routedHTTP{}).
		route("/en/saudi-arabia/jobs/?page=1", baytListingHTML(href)).
		route("/en/saudi-arabia/jobs/?page=2", baytListingHTML()). // empty → stops pagination
		route("quality-control-officer-5466655", baytDetailHTML(
			"Quality Control Officer", "2026-07-03", "SADAFCO", "Dammam", "SA"))

	jobs, err := NewBayt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "saudi-arabia",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "5466655" {
		t.Errorf("ExternalID = %q, want 5466655", j.ExternalID)
	}
	if !strings.HasSuffix(j.URL, href) {
		t.Errorf("URL = %q, want suffix %q", j.URL, href)
	}
	if j.Title != "Quality Control Officer" {
		t.Errorf("Title = %q", j.Title)
	}
	// Company comes from the posting (hiringOrganization), not the configured entry.
	if j.Company != "SADAFCO" {
		t.Errorf("Company = %q, want SADAFCO (from hiringOrganization)", j.Company)
	}
	if j.Location != "Dammam, SA" {
		t.Errorf("Location = %q, want %q", j.Location, "Dammam, SA")
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<p>Do the thing.</p>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-03", j.PostedAt)
	}
}

func TestBaytDropsPostingWithNoCompany(t *testing.T) {
	href := "/en/saudi-arabia/jobs/ghost-role-777/"
	fake := (&routedHTTP{}).
		route("/en/saudi-arabia/jobs/?page=1", baytListingHTML(href)).
		route("/en/saudi-arabia/jobs/?page=2", baytListingHTML()).
		route("ghost-role-777", baytDetailHTML("Ghost", "2026-07-03", "", "Riyadh", "SA"))

	jobs, err := NewBayt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "saudi-arabia",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("company-less posting should be dropped, got %d jobs", len(jobs))
	}
}

func TestBaytSkipsListingLinkWithNoID(t *testing.T) {
	// A non-job or id-less anchor on the listing must not become a Job.
	fake := (&routedHTTP{}).
		route("/en/saudi-arabia/jobs/?page=1", baytListingHTML(
			"/en/saudi-arabia/companies/acme-1/", // not a job link
			"/en/saudi-arabia/jobs/plain-slug/",  // job path but no trailing id
		)).
		route("/en/saudi-arabia/jobs/?page=2", baytListingHTML())

	jobs, err := NewBayt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "saudi-arabia",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("id-less links should yield no jobs, got %d", len(jobs))
	}
}

func TestBaytDropsDetailWithNoJobPosting(t *testing.T) {
	// A detail page whose markup lost its JobPosting block is dropped, not errored: one
	// re-templated posting must not abort an otherwise healthy crawl.
	href := "/en/saudi-arabia/jobs/broken-detail-321/"
	fake := (&routedHTTP{}).
		route("/en/saudi-arabia/jobs/?page=1", baytListingHTML(href)).
		route("/en/saudi-arabia/jobs/?page=2", baytListingHTML()).
		route("broken-detail-321", `<html><body>no ld+json here</body></html>`)

	jobs, err := NewBayt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "saudi-arabia",
	})
	if err != nil {
		t.Fatalf("a JobPosting-less detail must not error the board: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("JobPosting-less detail should be dropped, got %d jobs", len(jobs))
	}
}

func TestBaytFirstListingPageErrorFailsBoard(t *testing.T) {
	// No route for page 1 → the transport errors → the board fails loudly.
	_, err := NewBayt(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "saudi-arabia",
	})
	if err == nil {
		t.Fatal("a broken first listing page must error the board, not yield an empty success")
	}
}

func TestBaytPaginationFollowsMultiplePages(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/en/uae/jobs/?page=1", baytListingHTML("/en/uae/jobs/a-1/")).
		route("/en/uae/jobs/?page=2", baytListingHTML("/en/uae/jobs/b-2/")).
		route("/en/uae/jobs/?page=3", baytListingHTML()). // empty → stop
		route("/en/uae/jobs/a-1/", baytDetailHTML("A", "2026-07-01", "Acme", "Dubai", "AE")).
		route("/en/uae/jobs/b-2/", baytDetailHTML("B", "2026-07-02", "Beta", "Dubai", "AE"))

	jobs, err := NewBayt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Bayt", Provider: "bayt", Board: "uae",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("pagination should collect jobs across pages, got %d", len(jobs))
	}
}
