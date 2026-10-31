package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// gtSitemapIndexXML renders the sitemap index: a <sitemapindex> of shard <loc>s.
func gtSitemapIndexXML(shardLocs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><sitemapindex>`)
	for _, l := range shardLocs {
		b.WriteString(`<sitemap><loc>` + l + `</loc></sitemap>`)
	}
	b.WriteString(`</sitemapindex>`)
	return b.String()
}

// gtShardXML renders a shard: a <urlset> of page <loc>s.
func gtShardXML(urls ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, u := range urls {
		b.WriteString(`<url><loc>` + u + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// gtDetailHTML renders a GulfTalent job page: one schema.org JobPosting. datePosted is RFC3339,
// jobLocation is a single Place with a reliable address, and company of "" omits the org.
func gtDetailHTML(title, datePosted, company, locality, country string) string {
	org := ""
	if company != "" {
		org = `"hiringOrganization":{"@type":"Organization","name":"` + company + `"},`
	}
	return `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"description":"<p>Do the thing.<\/p><script>alert(1)<\/script>",
"identifier":{"@type":"PropertyValue","name":"` + company + `","value":"604168"},
"datePosted":"` + datePosted + `",
"employmentType":["FULL_TIME"],
` + org + `
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}
</script></head><body>page</body></html>`
}

func TestGulfTalentProvider(t *testing.T) {
	if got := NewGulfTalent(nil).Provider(); got != "gulftalent" {
		t.Errorf("Provider() = %q, want %q", got, "gulftalent")
	}
}

// gulftalent is blocked by both its TLS fingerprint AND the direct datacenter IP, so it must
// route through the proxied fingerprint transport — not the standard proxied client (which
// keeps Go's default fingerprint and still 403s) and not the direct fingerprint (blocked by IP).
func TestGulfTalentIsFingerprintProxied(t *testing.T) {
	if _, ok := proxiedFingerprintProviders["gulftalent"]; !ok {
		t.Error("gulftalent must be in proxiedFingerprintProviders (needs Chrome fingerprint AND the proxy IP)")
	}
	if _, ok := proxiedProviders["gulftalent"]; ok {
		t.Error("gulftalent must NOT be in proxiedProviders — the standard proxied client keeps Go's fingerprint, which its edge 403s")
	}
	// bayt shares the fingerprint transport but is behind a Cloudflare JS challenge no
	// fingerprint/IP passes, so it must not be routed here (proxying it would not help).
	if _, ok := proxiedFingerprintProviders["bayt"]; ok {
		t.Error("bayt must NOT be in proxiedFingerprintProviders — its Cloudflare JS challenge is unaffected by the proxy")
	}
}

func TestGulfTalentJobID(t *testing.T) {
	cases := map[string]string{
		"https://www.gulftalent.com/uae/jobs/maintenance-specialist-604168": "604168",
		"https://www.gulftalent.com/saudi-arabia/jobs/dev-42":               "42",
		"https://www.gulftalent.com/uae/jobs/tracked-99?ref=feed":           "99", // query stripped
		"https://www.gulftalent.com/jobs/category/accounting":               "",
		"https://www.gulftalent.com/companies/some-group-careers":           "",
	}
	for loc, want := range cases {
		if got := gulftalentJobID(loc); got != want {
			t.Errorf("gulftalentJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestGulfTalentRegisteredBoardlessAggregatorFacet(t *testing.T) {
	src, ok := All(nil)["gulftalent"]
	if !ok {
		t.Fatal("gulftalent not registered in sources.All")
	}
	if _, isBoardless := src.(boardless); !isBoardless {
		t.Error("gulftalent should be boardless (one global sitemap crawl, no board id)")
	}
	if _, isAggregator := src.(aggregator); !isAggregator {
		t.Error("gulftalent should be an aggregator (many companies per crawl)")
	}
	found := false
	for _, p := range FilterableProviders() {
		if p == "gulftalent" {
			found = true
		}
	}
	if !found {
		t.Error("gulftalent should appear in the source facet")
	}
}

func TestGulfTalentFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://www.gulftalent.com/uae/jobs/maintenance-specialist-604168"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
		route("sitemap_jx000.xml", gtShardXML(jobURL)).
		route("maintenance-specialist-604168", gtDetailHTML(
			"Maintenance Specialist", "2026-07-03T00:00:00+00:00", "RTC-1 Employment Services", "Dubai", "UAE"))

	jobs, err := NewGulfTalent(fake).Fetch(context.Background(), CompanyEntry{
		Company: "GulfTalent", Provider: "gulftalent",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "604168" {
		t.Errorf("ExternalID = %q, want 604168", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Maintenance Specialist" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "RTC-1 Employment Services" {
		t.Errorf("Company = %q, want the posting's hiringOrganization", j.Company)
	}
	if j.Location != "Dubai, UAE" {
		t.Errorf("Location = %q, want %q", j.Location, "Dubai, UAE")
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<p>Do the thing.</p>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-03", j.PostedAt)
	}
}

func TestGulfTalentFollowsOnlyJobPostingShards(t *testing.T) {
	// The index lists a job-posting shard (jx) and a category-listing shard (jl). Only jx pages
	// are real postings; a jl page must never be fetched or mapped even though its URL looks
	// job-shaped.
	jxJob := "https://www.gulftalent.com/uae/jobs/real-role-1000"
	jlJob := "https://www.gulftalent.com/uae/jobs/should-not-appear-2000"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML(
			"https://www.gulftalent.com/sitemaps/sitemap_jx000.xml",
			"https://www.gulftalent.com/sitemaps/sitemap_jl000.xml",
		)).
		route("sitemap_jx000.xml", gtShardXML(jxJob)).
		route("sitemap_jl000.xml", gtShardXML(jlJob)).
		route("real-role-1000", gtDetailHTML("Real", "2026-07-01T00:00:00+00:00", "Acme", "Dubai", "UAE")).
		route("should-not-appear-2000", gtDetailHTML("Nope", "2026-07-01T00:00:00+00:00", "Ghost", "Dubai", "UAE"))

	jobs, err := NewGulfTalent(fake).Fetch(context.Background(), CompanyEntry{Company: "GulfTalent"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1000" {
		t.Fatalf("only the jx-shard posting should be crawled, got %+v", jobs)
	}
}

func TestGulfTalentDropsPostingWithNoCompany(t *testing.T) {
	jobURL := "https://www.gulftalent.com/uae/jobs/ghost-777"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
		route("sitemap_jx000.xml", gtShardXML(jobURL)).
		route("ghost-777", gtDetailHTML("Ghost", "2026-07-01T00:00:00+00:00", "", "Dubai", "UAE"))

	jobs, err := NewGulfTalent(fake).Fetch(context.Background(), CompanyEntry{Company: "GulfTalent"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("company-less posting should be dropped, got %d", len(jobs))
	}
}

func TestGulfTalentDropsDetailWithNoJobPosting(t *testing.T) {
	// A detail page without a JobPosting block is dropped, not errored — one bad posting must
	// not abort the crawl (only an unparseable index does).
	jobURL := "https://www.gulftalent.com/uae/jobs/broken-555"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
		route("sitemap_jx000.xml", gtShardXML(jobURL)).
		route("broken-555", `<html><body>no ld+json</body></html>`)

	jobs, err := NewGulfTalent(fake).Fetch(context.Background(), CompanyEntry{Company: "GulfTalent"})
	if err != nil {
		t.Fatalf("a JobPosting-less detail must not error the crawl: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("JobPosting-less detail should be dropped, got %d", len(jobs))
	}
}

func TestGulfTalentUnparseableIndexErrors(t *testing.T) {
	fake := (&routedHTTP{}).route("/sitemap.xml", "not xml at all <<<")
	_, err := NewGulfTalent(fake).Fetch(context.Background(), CompanyEntry{Company: "GulfTalent"})
	if err == nil {
		t.Fatal("an unparseable sitemap index must error, not silently yield no jobs")
	}
}

// FetchNew hydrates only what the catalogue lacks: a seen posting is a liveness refresh with no
// detail request of its own, and new ones are hydrated up to the per-run budget — the rest are
// deferred (not yielded, so not stored) to a later run. The sitemap lists ~20k postings and each
// detail page is a browser navigation of seconds, so re-reading every one each run took a run
// past its four-hour timeout (2026-10-21).
func TestGulfTalentFetchNewRefreshesSeenAndBudgetsNew(t *testing.T) {
	seenURL := "https://www.gulftalent.com/uae/jobs/old-role-100"
	newA := "https://www.gulftalent.com/uae/jobs/role-a-201"
	newB := "https://www.gulftalent.com/uae/jobs/role-b-202"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
		route("sitemap_jx000.xml", gtShardXML(seenURL, newA, newB)).
		route("role-a-201", gtDetailHTML("A", "2026-07-03T00:00:00+00:00", "Acme", "Dubai", "UAE")).
		route("role-b-202", gtDetailHTML("B", "2026-07-03T00:00:00+00:00", "Beta", "Dubai", "UAE"))
	src := gulftalent{http: fake, newestPerRun: 1}
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "100" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := map[string]Job{}
	for _, j := range jobs {
		got[j.ExternalID] = j
	}
	if len(got) != 2 || !got["100"].SeenRefresh || got["201"].Title != "A" {
		t.Fatalf("jobs = %+v, want 100 refreshed and 201 hydrated (202 deferred)", jobs)
	}
}

// A job shard that cannot be read fails the crawl rather than dropping its slice: gulftalent is
// swept, so a listing missing a shard would close every posting in it.
func TestGulfTalentUnreadableJobShardFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML(
			"https://www.gulftalent.com/sitemaps/sitemap_jx000.xml",
			"https://www.gulftalent.com/sitemaps/sitemap_jx001.xml")).
		route("sitemap_jx000.xml", gtShardXML("https://www.gulftalent.com/uae/jobs/role-a-201")).
		route("role-a-201", gtDetailHTML("A", "2026-07-03T00:00:00+00:00", "Acme", "Dubai", "UAE"))
	if _, err := (gulftalent{http: fake}).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when a job shard cannot be read")
	}
}

// gulftalent lists its whole catalogue every run (the sitemap and every job shard, or an error),
// so it is swept by source: a posting gone from the sitemap closes whichever company it belonged
// to. Under the company scope it closed only for companies a run happened to write; on
// 2026-10-30, 12,906 of ~15k open postings had last been seen on 2026-07-07, and every one
// sampled answered "Job closed".
func TestGulfTalentIsFullCatalog(t *testing.T) {
	if !isFullCatalog(NewGulfTalent(nil)) {
		t.Fatal("gulftalent must be fullCatalog")
	}
}

// A sitemap that names no job shard, or job shards that name no posting, is not an empty
// catalogue but a page other than the one we read (a challenge, a moved layout). It must fail
// the crawl: a source-scoped sweep reads an empty listing as every posting gone.
func TestGulfTalentEmptyListingFailsTheCrawl(t *testing.T) {
	cases := map[string]*routedHTTP{
		"no job shard": (&routedHTTP{}).
			route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jl000.xml")),
		"no posting in the shards": (&routedHTTP{}).
			route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
			route("sitemap_jx000.xml", gtShardXML("https://www.gulftalent.com/jobs/category/accounting")),
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (gulftalent{http: fake}).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
				t.Fatal("want an error for a listing with no postings")
			}
		})
	}
}

// The new postings past the newest share are taken from a window that moves every day. A fixed
// window starved the backlog for good: a posting whose company an ATS already covers is skipped
// after hydration, so it is never stored and stays "new" — measured 2026-10-30, 383 of each
// run's 500 hydrations were such postings, the same ones every day, while ~33.5k live postings
// further down the sitemap were never read.
func TestGulfTalentFetchNewRotatesTheBacklogWindow(t *testing.T) {
	var urls []string
	for i := 0; i < 10; i++ {
		urls = append(urls, fmt.Sprintf("https://www.gulftalent.com/uae/jobs/role-%d", 100+i))
	}
	fake := (&routedHTTP{}).
		route("/sitemap.xml", gtSitemapIndexXML("https://www.gulftalent.com/sitemaps/sitemap_jx000.xml")).
		route("sitemap_jx000.xml", gtShardXML(urls...)).
		route("/jobs/role-", gtDetailHTML("Role", "2026-07-03T00:00:00+00:00", "Acme", "Dubai", "UAE"))
	day0 := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	reached := map[string]bool{}
	for d := 0; d < 3; d++ {
		now := day0.AddDate(0, 0, d)
		src := gulftalent{http: fake, newestPerRun: 2, backlogPerRun: 3, now: func() time.Time { return now }}
		jobs, err := src.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
		if err != nil {
			t.Fatalf("day %d: FetchNew: %v", d, err)
		}
		got := map[string]bool{}
		for _, j := range jobs {
			got[j.ExternalID] = true
			reached[j.ExternalID] = true
		}
		if len(got) != 5 || !got["100"] || !got["101"] {
			t.Fatalf("day %d: hydrated %v, want the newest 2 plus a backlog window of 3", d, got)
		}
	}
	if len(reached) != 10 {
		t.Fatalf("three days of a 3-wide window must reach all 8 backlog postings; reached %d of 10: %v", len(reached), reached)
	}
}

func TestGulfTalentIsAHydratingSource(t *testing.T) {
	if _, ok := NewGulfTalent(nil).(HydratingSource); !ok {
		t.Fatal("gulftalent must implement HydratingSource")
	}
}
