package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

const instaffoDetailHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"Senior Software Engineer API (f/m/d)",
"description":"<p>Node.js &amp; SQL.</p><script>alert(1)<\/script>",
"datePosted":"2026-07-20",
"hiringOrganization":{"@type":"Organization","name":"smartclip Europe GmbH"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Berlin","addressCountry":"DE"}}}
</script></head><body></body></html>`

const instaffoIndexXML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://jobs.instaffo.com/sitemap-static.xml</loc></sitemap>
<sitemap><loc>https://jobs.instaffo.com/sitemap-jobs.xml</loc></sitemap>
</sitemapindex>`

func instaffoJobsXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestInstaffoProvider(t *testing.T) {
	if got := NewInstaffo(nil).Provider(); got != "instaffo" {
		t.Errorf("Provider() = %q, want instaffo", got)
	}
}

func TestInstaffoIsBoardlessAggregator(t *testing.T) {
	s := NewInstaffo(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("instaffo should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("instaffo should implement the aggregator marker")
	}
}

func TestInstaffoRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["instaffo"]; !ok {
		t.Error("All() should register provider instaffo")
	}
	if !slices.Contains(FilterableProviders(), "instaffo") {
		t.Error("FilterableProviders() should include instaffo")
	}
}

// Instaffo lists every posting twice — once per locale — under the same native id. Only the
// canonical /de/ URL yields an id, so the /en/ twin is dropped before it is ever fetched.
func TestInstaffoJobIDTakesCanonicalLocaleOnly(t *testing.T) {
	cases := map[string]string{
		"https://jobs.instaffo.com/de/job/java-entwickler-backend-m-w-430f12a0768e": "java-entwickler-backend-m-w-430f12a0768e",
		"https://jobs.instaffo.com/en/job/java-entwickler-backend-m-w-430f12a0768e": "",
		"https://jobs.instaffo.com/de/talent/berlin":                                "",
	}
	for u, want := range cases {
		if got := instaffoJobID(u); got != want {
			t.Errorf("instaffoJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestInstaffoFetchResolvesJobSitemapThenMaps(t *testing.T) {
	job := "https://jobs.instaffo.com/de/job/software-engineer-api-f-m-d-8394973d2c35"
	twin := "https://jobs.instaffo.com/en/job/software-engineer-api-f-m-d-8394973d2c35"
	fake := (&routedHTTP{}).
		route("/de/job/software-engineer-api-f-m-d-8394973d2c35", instaffoDetailHTML).
		route("/sitemap-jobs.xml", instaffoJobsXML(job, twin, "https://jobs.instaffo.com/de/talent/berlin")).
		route("/sitemap.xml", instaffoIndexXML)

	jobs, err := NewInstaffo(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (locale twin and non-job entry filtered)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "software-engineer-api-f-m-d-8394973d2c35" || j.URL != job {
		t.Errorf("id/url wrong: %s %s", j.ExternalID, j.URL)
	}
	if j.Company != "smartclip Europe GmbH" || j.Title != "Senior Software Engineer API (f/m/d)" {
		t.Errorf("bad mapping: company=%q title=%q", j.Company, j.Title)
	}
	if j.Location != "Berlin, DE" {
		t.Errorf("Location = %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

// Instaffo never retires a posting from its sitemap, so the unseen sweep can never close
// one — age is the only staleness signal there is, and it is applied on the way in.
func TestInstaffoDropsPostingsOlderThanMaxAge(t *testing.T) {
	// The date is stamped in UTC because that is how the adapter reads a bare "2006-01-02"
	// datePosted back. Formatting it in the local zone instead makes the test fail west of
	// Greenwich whenever the local date still lags UTC's: the local calendar day is one earlier,
	// the parsed midnight lands a day further back, and the "fresh" posting ages past the window.
	// CI runs in UTC, so this only ever broke on developer machines.
	postingAged := func(d time.Duration) string {
		return `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Entwickler",
"hiringOrganization":{"@type":"Organization","name":"Acme GmbH"},
"datePosted":"` + time.Now().UTC().Add(-d).Format("2006-01-02") + `"}
</script></head><body></body></html>`
	}

	fresh := "https://jobs.instaffo.com/de/job/fresh-aaaaaaaaaaaa"
	stale := "https://jobs.instaffo.com/de/job/stale-bbbbbbbbbbbb"
	undated := "https://jobs.instaffo.com/de/job/undated-cccccccccccc"

	// The fresh fixture sits a week inside the window, not an hour. `datePosted` is
	// date-only, so formatting truncates it back to midnight and the posting reads as up to
	// a day older than asked — and in a negative-offset timezone `Format` picks the local
	// date while `Parse` reads it as UTC, adding the offset on top. At maxAge-24h that put
	// it 16 minutes past a 365-day limit here (UTC-03:00, late evening) while still passing
	// in CI's UTC by a few minutes. The test is about either side of the window, not its
	// exact edge, so it takes a margin wider than any timezone.
	fake := (&routedHTTP{}).
		route("/de/job/fresh-aaaaaaaaaaaa", postingAged(instaffoMaxAge-7*24*time.Hour)).
		route("/de/job/stale-bbbbbbbbbbbb", postingAged(instaffoMaxAge+24*time.Hour)).
		route("/de/job/undated-cccccccccccc", `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Entwickler",
"hiringOrganization":{"@type":"Organization","name":"Acme GmbH"}}
</script></head><body></body></html>`).
		route("/sitemap-jobs.xml", instaffoJobsXML(fresh, stale, undated)).
		route("/sitemap.xml", instaffoIndexXML)

	jobs, err := NewInstaffo(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (only the posting inside the age window)", len(jobs))
	}
	if jobs[0].URL != fresh {
		t.Errorf("kept %q, want the fresh posting", jobs[0].URL)
	}
}

// A posting with no company would slug to nothing downstream, so it is skipped rather than
// carried with an empty employer.
func TestInstaffoSkipsPostingWithoutCompany(t *testing.T) {
	const noCompany = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Ghost","datePosted":"2026-07-20"}
</script></head><body></body></html>`

	job := "https://jobs.instaffo.com/de/job/ghost-000000000000"
	fake := (&routedHTTP{}).
		route("/de/job/ghost-000000000000", noCompany).
		route("/sitemap-jobs.xml", instaffoJobsXML(job)).
		route("/sitemap.xml", instaffoIndexXML)

	jobs, err := NewInstaffo(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (posting without hiringOrganization)", len(jobs))
	}
}
