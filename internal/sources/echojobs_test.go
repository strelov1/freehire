package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// echojobsFakeHTTP is a routing test double for echojobs' two data sources: XML (sitemap index +
// job shards) and HTML (a posting's detail page, carrying its JobPosting ld+json). Each is
// keyed by exact URL, so a test wires only the routes its scenario needs. gotXML/gotHTML record
// every URL requested, in order, so a test can assert what was (and was not) fetched — in
// particular that a seen posting never costs an HTML request. mu guards both slices: FetchNew
// runs detail() concurrently across a bounded worker pool (see fetchDetails), so a scenario with
// more than one unseen posting calls GetHTML from several goroutines at once.
type echojobsFakeHTTP struct {
	xmlRoutes  map[string]string
	htmlRoutes map[string]string
	xmlErr     map[string]bool
	htmlErr    map[string]bool

	mu      sync.Mutex
	gotXML  []string
	gotHTML []string
}

func (f *echojobsFakeHTTP) GetXML(_ context.Context, url string, v any) error {
	f.mu.Lock()
	f.gotXML = append(f.gotXML, url)
	f.mu.Unlock()
	if f.xmlErr[url] {
		return errors.New("echojobsFakeHTTP: xml boom")
	}
	body, ok := f.xmlRoutes[url]
	if !ok {
		return fmt.Errorf("echojobsFakeHTTP: no xml route for %s", url)
	}
	return xml.Unmarshal([]byte(body), v)
}

func (f *echojobsFakeHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.gotHTML = append(f.gotHTML, url)
	f.mu.Unlock()
	if f.htmlErr[url] {
		return nil, errors.New("echojobsFakeHTTP: html boom")
	}
	body, ok := f.htmlRoutes[url]
	if !ok {
		return nil, fmt.Errorf("echojobsFakeHTTP: no html route for %s", url)
	}
	return html.Parse(strings.NewReader(body))
}

// echojobsSitemapIndexXML builds a sitemap index carrying the given job-shard URLs among the
// site's other (non-job) shards, which a correct crawl must ignore.
func echojobsSitemapIndexXML(jobShardURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	b.WriteString(`<sitemap><loc>https://echojobs.io/sitemap-nav.xml</loc></sitemap>`)
	b.WriteString(`<sitemap><loc>https://echojobs.io/sitemap-companies.xml</loc></sitemap>`)
	for _, u := range jobShardURLs {
		b.WriteString(`<sitemap><loc>` + u + `</loc></sitemap>`)
	}
	b.WriteString(`</sitemapindex>`)
	return b.String()
}

// echojobsShardEntry is one <url> in a job-shard fixture: a slug and its ISO8601 lastmod.
type echojobsShardEntry struct {
	slug, lastMod string
}

func echojobsShardXML(entries ...echojobsShardEntry) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, e := range entries {
		b.WriteString(`<url><loc>https://echojobs.io/job/` + e.slug + `</loc><lastmod>` + e.lastMod + `</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// echojobsJobHTML builds a detail page carrying a schema.org JobPosting ld+json block, as
// echojobs.io's own /job/<slug> pages do (verified live).
func echojobsJobHTML(fields string) string {
	return `<html><body><script type="application/ld+json">{"@context":"https://schema.org","@type":"JobPosting",` +
		fields + `}</script></body></html>`
}

// echojobsShard1URL is the single shard most tests below need — a scenario needing more than
// one shard (freshness-window cutoff, shard-failure partial results) wires its own.
const echojobsShard1URL = "https://echojobs.io/sitemap-jobs/1.xml"

// echojobsSingleShardRoutes builds the xmlRoutes for a fake carrying one shard with one
// freshly-dated posting — the common setup most detail-mapping tests need, so they need not
// each repeat the sitemap index/shard plumbing.
func echojobsSingleShardRoutes(slug string) map[string]string {
	return map[string]string{
		echojobsSitemapURL: echojobsSitemapIndexXML(echojobsShard1URL),
		echojobsShard1URL:  echojobsShardXML(echojobsShardEntry{slug, time.Now().UTC().Format(time.RFC3339)}),
	}
}

func TestEchojobsFetchNewHydratesUnseenPosting(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("acme-swe-abc12"),
		htmlRoutes: map[string]string{
			echojobsJobURL("acme-swe-abc12"): echojobsJobHTML(`"title":"Backend Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Great role.</p>",` +
				`"jobLocationType":"TELECOMMUTE",` +
				`"datePosted":"2026-08-01T00:00:00Z",` +
				`"skills":"Go, Kafka"`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	if job.ExternalID != "acme-swe-abc12" {
		t.Errorf("ExternalID = %q", job.ExternalID)
	}
	if job.URL != "https://echojobs.io/job/acme-swe-abc12" {
		t.Errorf("URL = %q", job.URL)
	}
	if job.Title != "Backend Engineer" || job.Company != "Acme" {
		t.Errorf("identity mismatch: %+v", job)
	}
	if job.Description != "<p>Great role.</p>" {
		t.Errorf("Description = %q", job.Description)
	}
	if !job.Remote || job.WorkMode != "remote" {
		t.Errorf("TELECOMMUTE should map to Remote=true, WorkMode=remote: %+v", job)
	}
	if job.PostedAt == nil {
		t.Errorf("PostedAt not parsed")
	}
	if !slices.Contains(job.Skills, "go") {
		t.Errorf("Skills = %v, want it to contain \"go\"", job.Skills)
	}
}

// A posting the catalogue already has must not cost a detail request: it is refreshed
// (SeenRefresh) with just its identity, preserving whatever description was hydrated when it
// was new — a content-less re-upsert would wipe it. Unlike the old list+detail split, the
// sitemap alone carries no title/company, so a seen posting's refresh Job is intentionally bare.
func TestEchojobsFetchNewSkipsDetailForSeenPosting(t *testing.T) {
	http := &echojobsFakeHTTP{xmlRoutes: echojobsSingleShardRoutes("acme-swe")}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(externalID string) bool {
		return externalID == "acme-swe"
	})
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Fatalf("want 1 job with SeenRefresh set, got %+v", jobs)
	}
	if jobs[0].ExternalID != "acme-swe" || jobs[0].URL != "https://echojobs.io/job/acme-swe" {
		t.Errorf("seen refresh should still carry identity fields: %+v", jobs[0])
	}
	if len(http.gotHTML) != 0 {
		t.Fatalf("a seen posting must not cost a detail request, got %v", http.gotHTML)
	}
}

// A failed detail request drops just that posting: unlike the old list+detail split, the
// sitemap carries no fields a list-only Job could fall back to, so there is nothing left to
// ingest for it this run.
func TestEchojobsFetchNewDetailFailureDropsPosting(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("acme-swe"),
		htmlErr:   map[string]bool{echojobsJobURL("acme-swe"): true},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("want 0 jobs (detail failed, no list-only fallback), got %+v", jobs)
	}
}

// A detail page carrying no JobPosting ld+json block (or one with an empty title) drops the
// posting the same way a fetch failure does — there is no usable content to ingest.
func TestEchojobsFetchNewMissingJSONLDDropsPosting(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("acme-swe"),
		htmlRoutes: map[string]string{
			echojobsJobURL("acme-swe"): `<html><body>not a job page</body></html>`,
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("want 0 jobs, got %+v", jobs)
	}
}

// The crawl keeps walking shards while their postings are within the freshness window, and
// stops as soon as a shard's last (oldest, since shards are newest-first) item falls outside
// it — the earlier, still-fresh items on that same shard are kept, and a shard past that point
// is never fetched.
func TestEchojobsFetchStopsAtFreshnessWindow(t *testing.T) {
	now := time.Now().UTC()
	fresh := now.Format(time.RFC3339)
	stillFresh := now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	stale := now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)

	shard1 := echojobsShard1URL
	shard2 := "https://echojobs.io/sitemap-jobs/2.xml"
	shard3 := "https://echojobs.io/sitemap-jobs/3.xml"

	http := &echojobsFakeHTTP{
		xmlRoutes: map[string]string{
			echojobsSitemapURL: echojobsSitemapIndexXML(shard1, shard2, shard3),
			shard1: echojobsShardXML(
				echojobsShardEntry{"job-1", fresh}, echojobsShardEntry{"job-2", fresh}),
			shard2: echojobsShardXML(
				echojobsShardEntry{"job-3", stillFresh}, echojobsShardEntry{"job-4", stale}),
			shard3: echojobsShardXML(echojobsShardEntry{"job-5", fresh}),
		},
	}

	postings, err := echojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	var slugs []string
	for _, p := range postings {
		slugs = append(slugs, p.Slug)
	}
	if !slices.Equal(slugs, []string{"job-1", "job-2", "job-3"}) {
		t.Fatalf("want job-1,job-2,job-3 (job-4 stale, job-5 never reached), got %v", slugs)
	}
	if !slices.Equal(http.gotXML, []string{echojobsSitemapURL, shard1, shard2}) {
		t.Fatalf("shard 3 should never be requested once shard 2's last item is stale, got %v", http.gotXML)
	}
}

// A failed first shard is a board-level error, per the house pagination rule.
func TestEchojobsCrawlFirstShardFailure(t *testing.T) {
	shard1 := echojobsShard1URL
	http := &echojobsFakeHTTP{
		xmlRoutes: map[string]string{echojobsSitemapURL: echojobsSitemapIndexXML(shard1)},
		xmlErr:    map[string]bool{shard1: true},
	}
	if _, err := (echojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want error on first-shard failure")
	}
}

// A later shard failing ends the walk with what was gathered so far.
func TestEchojobsCrawlLaterShardFailureReturnsPartial(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	shard1 := echojobsShard1URL
	shard2 := "https://echojobs.io/sitemap-jobs/2.xml"
	http := &echojobsFakeHTTP{
		xmlRoutes: map[string]string{
			echojobsSitemapURL: echojobsSitemapIndexXML(shard1, shard2),
			shard1:             echojobsShardXML(echojobsShardEntry{"job-1", now}),
		},
		xmlErr: map[string]bool{shard2: true},
	}

	postings, err := echojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(postings) != 1 || postings[0].Slug != "job-1" {
		t.Fatalf("want partial result [job-1], got %+v", postings)
	}
}

// A posting with no jobLocation falls back to applicantLocationRequirements for its Location —
// the shape a fully-remote posting emits (verified live: echojobs' own Doowii listing).
func TestEchojobsFetchNewFallsBackToApplicantLocationRequirements(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("remote-role"),
		htmlRoutes: map[string]string{
			echojobsJobURL("remote-role"): echojobsJobHTML(`"title":"Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Role.</p>",` +
				`"jobLocationType":"TELECOMMUTE",` +
				`"applicantLocationRequirements":{"@type":"Country","name":"US"}`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "US" {
		t.Fatalf("want Location=US from applicantLocationRequirements, got %+v", jobs)
	}
}

// schema.org allows applicantLocationRequirements as EITHER a single object OR an array (a
// remote posting open to several countries). Go's json.Unmarshal errors on a bare-object field
// asked to decode an array — decoding it as a plain struct would fail the WHOLE ld+json block
// (see schemaNamedAreas' doc), dropping the posting entirely rather than just its location. This
// pins that the array form decodes cleanly and every field survives, not just the location.
func TestEchojobsFetchNewHandlesArrayApplicantLocationRequirements(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("multi-country-role"),
		htmlRoutes: map[string]string{
			echojobsJobURL("multi-country-role"): echojobsJobHTML(`"title":"Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Role.</p>",` +
				`"jobLocationType":"TELECOMMUTE",` +
				`"applicantLocationRequirements":[{"@type":"Country","name":"US"},{"@type":"Country","name":"Canada"}]`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job (array-shaped applicantLocationRequirements must not drop the posting), got %+v", jobs)
	}
	job := jobs[0]
	if job.Title != "Engineer" {
		t.Errorf("Title = %q, want it to survive the array field", job.Title)
	}
	if job.Location != "US, Canada" {
		t.Errorf("Location = %q, want both countries joined", job.Location)
	}
}

// schema.org allows jobLocation as EITHER a single Place OR an array of them, the same way
// applicantLocationRequirements does. This pins the array form via the shared schemaPlaces type.
func TestEchojobsFetchNewHandlesArrayJobLocation(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("multi-location-role"),
		htmlRoutes: map[string]string{
			echojobsJobURL("multi-location-role"): echojobsJobHTML(`"title":"Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Role.</p>",` +
				`"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Austin, TX","addressCountry":"US"}},` +
				`{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Denver, CO","addressCountry":"US"}}]`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Location != "Austin, TX, US" {
		t.Fatalf("want the first location (array-shaped jobLocation must not drop the posting), got %+v", jobs)
	}
}

// datePosted may be a bare "2006-01-02" date rather than a full RFC3339 timestamp (the same
// tenant-dependent variance northstone/briefhq already handle via parseRFC3339OrDate).
func TestEchojobsFetchNewParsesDateOnlyPostedAt(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("date-only-role"),
		htmlRoutes: map[string]string{
			echojobsJobURL("date-only-role"): echojobsJobHTML(`"title":"Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Role.</p>",` +
				`"datePosted":"2026-08-01"`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].PostedAt == nil {
		t.Fatalf("want PostedAt parsed from a bare date, got %+v", jobs)
	}
}

// A posting WITH a jobLocation address uses it, and carries no jobLocationType (on-site),
// so WorkMode stays empty rather than guessed.
func TestEchojobsFetchNewMapsOnsiteLocation(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("onsite-role"),
		htmlRoutes: map[string]string{
			echojobsJobURL("onsite-role"): echojobsJobHTML(`"title":"Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Role.</p>",` +
				`"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Apopka, FL","addressCountry":"US"}}`),
		},
	}

	jobs, err := echojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %+v", jobs)
	}
	job := jobs[0]
	if job.Location != "Apopka, FL, US" {
		t.Errorf("Location = %q", job.Location)
	}
	if job.Remote || job.WorkMode != "" {
		t.Errorf("no jobLocationType should leave Remote=false, WorkMode=\"\": %+v", job)
	}
}

// Fetch (the non-hydrating fallback) has no list-only tier to fall back to any more — the
// sitemap alone carries no field a Job needs — so it fully hydrates every posting it finds,
// same as FetchNew treating everything as new.
func TestEchojobsFetchHydratesEveryPosting(t *testing.T) {
	http := &echojobsFakeHTTP{
		xmlRoutes: echojobsSingleShardRoutes("acme-swe"),
		htmlRoutes: map[string]string{
			echojobsJobURL("acme-swe"): echojobsJobHTML(`"title":"Backend Engineer",` +
				`"hiringOrganization":{"@type":"Organization","name":"Acme"},` +
				`"description":"<p>Great role.</p>"`),
		},
	}

	jobs, err := echojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Backend Engineer" {
		t.Fatalf("want 1 fully-hydrated job, got %+v", jobs)
	}
}

func TestEchojobsProvider(t *testing.T) {
	if got := (echojobs{}).Provider(); got != "echojobs" {
		t.Errorf("Provider() = %q, want echojobs", got)
	}
}
