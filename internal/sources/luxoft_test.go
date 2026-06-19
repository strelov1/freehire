package sources

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// lxListingHTML builds a Luxoft listing page linking each given job URL, plus the
// pagination/page-size controls that must be ignored (they share the /jobs path).
func lxListingHTML(jobURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs__list">`)
	for _, u := range jobURLs {
		b.WriteString(`<a href="` + u + `" class="jobs__list__job"><h2>A job</h2></a>`)
	}
	b.WriteString(`<a href="/jobs?page=2">Next</a>`)
	b.WriteString(`<a href="/jobs?perPage=15">15</a>`)
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// lxDetailHTML builds a Luxoft job page carrying a JobPosting ld+json block whose
// jobLocation is a single Place object (address a single PostalAddress object) and whose
// datePosted is the space-separated, zoneless timestamp Luxoft emits.
func lxDetailHTML(title, description, datePosted, locality, region, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"identifier":{"@type":"PropertyValue","name":"Luxoft","value":"VR-123564"},` +
		`"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressRegion":"` + region + `","addressCountry":"` + country + `"}}}` +
		`</script></head><body></body></html>`
}

func TestLuxoftJobID(t *testing.T) {
	cases := map[string]string{
		"https://career.luxoft.com/jobs/front-end-react-developer-25262": "25262",
		"/jobs/senior-axiom-developer-25277":                             "25277",
		"/jobs/react-18-developer-25001":                                 "25001", // a digit mid-slug; the trailing id wins
		"https://career.luxoft.com/jobs":                                 "",      // listing root
		"https://career.luxoft.com/jobs?page=2":                          "",      // pagination
		"/jobs?perPage=15":                                               "",      // page-size control
		"/jobs/no-trailing-id":                                           "",      // not a posting
		"/about":                                                         "",
	}
	for u, want := range cases {
		if got := lxJobID(u); got != want {
			t.Errorf("lxJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestLuxoftJobLinksResolvesRelativeAndFiltersNonJobs(t *testing.T) {
	h := `<html><body>
		<a href="/jobs/front-end-react-developer-25262">Engineer</a>
		<a href="/jobs/front-end-react-developer-25262">Apply</a>
		<a href="https://career.luxoft.com/jobs/senior-axiom-developer-25277">Axiom</a>
		<a href="/jobs?page=2">Next</a>
		<a href="/about">About</a>
	</body></html>`
	base := mustURL(t, "https://career.luxoft.com/jobs?page=1")
	got := lxJobLinks(base, parseHTML(t, h))
	want := []string{
		"https://career.luxoft.com/jobs/front-end-react-developer-25262",
		"https://career.luxoft.com/jobs/senior-axiom-developer-25277",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lxJobLinks() = %v, want %v", got, want)
	}
}

func TestLuxoftPostingLocation(t *testing.T) {
	// Structured city/region/country wins.
	p := lxPosting{JobLocation: lxPlace{
		Name:    "PUNE, INDIA",
		Address: lxAddress{AddressLocality: "Pune", AddressRegion: "Maharashtra", AddressCountry: "IN"},
	}}
	if got, want := p.location(), "Pune, Maharashtra, IN"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	// Falls back to the Place name when the structured parts are empty.
	p = lxPosting{JobLocation: lxPlace{Name: "Remote"}}
	if got, want := p.location(), "Remote"; got != want {
		t.Errorf("location() fallback = %q, want %q", got, want)
	}
	// No location at all.
	if got := (lxPosting{}).location(); got != "" {
		t.Errorf("location() = %q, want empty", got)
	}
}

func TestLuxoftFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://career.luxoft.com/jobs/front-end-react-developer-25262"
	detail := lxDetailHTML(
		"Front-end React Developer",
		// Entity-encoded in the ld+json (so a literal </script> can't truncate the block),
		// matching the html.UnescapeString the adapter applies before sanitizing.
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-06-14 19:00:00", "Pune", "", "IN")
	fake := (&routedHTTP{}).
		route("&page=2", lxListingHTML()).
		route("perPage=60", lxListingHTML(jobURL)).
		route("/jobs/front-end-react-developer-25262", detail)

	jobs, err := NewLuxoft(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Luxoft", Provider: "luxoft", Board: "career.luxoft.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "25262" {
		t.Errorf("ExternalID = %q, want 25262", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Front-end React Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Luxoft" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Pune, IN" {
		t.Errorf("Location = %q, want %q", j.Location, "Pune, IN")
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 14, 19, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-14 19:00:00 UTC", j.PostedAt)
	}
}

func TestLuxoftFutureDatePostedDropped(t *testing.T) {
	// Luxoft stamps some postings with a future datePosted; the parser must drop it to nil
	// (the pipeline then falls back to ingest time) rather than persist a future posted_at.
	jobURL := "https://career.luxoft.com/jobs/senior-axiom-developer-25277"
	detail := lxDetailHTML("Senior Axiom Developer", "<p>x</p>", "2099-08-03 00:00:00", "Pune", "", "IN")
	fake := (&routedHTTP{}).
		route("&page=2", lxListingHTML()).
		route("perPage=60", lxListingHTML(jobURL)).
		route("/jobs/senior-axiom-developer-25277", detail)

	jobs, err := NewLuxoft(fake).Fetch(context.Background(), CompanyEntry{Company: "Luxoft", Board: "career.luxoft.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (future date dropped)", jobs[0].PostedAt)
	}
}

func TestLuxoftStopsWhenPageYieldsNoNewLinks(t *testing.T) {
	// Luxoft clamps ?page=N past its last page, re-serving the same links; a page with no
	// *new* links must terminate enumeration so Fetch does not loop to lxMaxPages.
	d := lxDetailHTML("Role", "<p>x</p>", "2026-06-14 19:00:00", "Pune", "", "IN")
	fake := (&routedHTTP{}).
		route("perPage=60", lxListingHTML("https://b/jobs/role-1")). // every page returns the same link
		route("/jobs/role-1", d)

	jobs, err := NewLuxoft(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (de-duplicated, no runaway loop)", len(jobs))
	}
}

func TestLuxoftFailedDetailDropsOnlyThatPosting(t *testing.T) {
	d := lxDetailHTML("Kept", "<p>x</p>", "2026-06-14 19:00:00", "Pune", "", "IN")
	// No route for /jobs/dropped-2 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("&page=2", lxListingHTML()).
		route("perPage=60", lxListingHTML("https://b/jobs/kept-1", "https://b/jobs/dropped-2")).
		route("/jobs/kept-1", d)

	jobs, err := NewLuxoft(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

// recordingHTML captures the URLs requested, serving canned bodies by substring while
// recording every call, so a test can assert the exact listing URLs the adapter builds.
type recordingHTML struct {
	urls   []string
	routes *routedHTTP
}

func (r *recordingHTML) GetHTML(ctx context.Context, u string) (*html.Node, error) {
	r.urls = append(r.urls, u)
	return r.routes.GetHTML(ctx, u)
}

func TestLuxoftListingPageOneOmitsPageParam(t *testing.T) {
	// Luxoft 301-redirects /jobs?page=1 (the default page) to a broken relative Location,
	// which the HTTP client resolves to a 404. The adapter must request page 1 WITHOUT a
	// page param, adding &page=N only from page 2 on.
	rec := &recordingHTML{routes: (&routedHTTP{}).
		route("&page=2", lxListingHTML()).
		route("perPage=60", lxListingHTML("https://career.luxoft.com/jobs/role-1")).
		route("/jobs/role-1", lxDetailHTML("Role", "<p>x</p>", "2026-06-14 19:00:00", "Pune", "", "IN"))}

	if _, err := NewLuxoft(rec).Fetch(context.Background(), CompanyEntry{Company: "Luxoft", Board: "career.luxoft.com"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(rec.urls) == 0 {
		t.Fatal("no listing URL requested")
	}
	page1 := rec.urls[0]
	if strings.Contains(page1, "page=1") {
		t.Errorf("page-1 listing URL %q must not carry page=1 (it 301-redirects to a 404)", page1)
	}
	if !strings.Contains(page1, "perPage=60") {
		t.Errorf("page-1 listing URL %q should request perPage=60", page1)
	}
}

func TestLuxoftProvider(t *testing.T) {
	if got := NewLuxoft(nil).Provider(); got != "luxoft" {
		t.Errorf("Provider() = %q, want %q", got, "luxoft")
	}
}

func TestLuxoftRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["luxoft"]
	if !ok {
		t.Fatal("All() missing provider luxoft")
	}
	if s.Provider() != "luxoft" {
		t.Errorf("All()[luxoft].Provider() = %q", s.Provider())
	}
}
