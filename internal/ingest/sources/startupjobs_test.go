package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// startupjobsNow is the fake clock: the evening of the day the real sitemap index fixture was
// generated (its posting files carry lastmod 2026-10-14T03:00:23Z..03:00:51Z).
var startupjobsNow = time.Date(2026, 10, 14, 22, 0, 0, 0, time.UTC)

// startupjobsSince is where the window starts at that hour: the sitemap is older than the
// window span, so the window reaches back the span and no further.
var startupjobsSince = startupjobsNow.Add(-startupjobsWindowSpan).Unix()

// startupjobsFakeIndex stands in for the Algolia index. It evaluates the adapter's created_at_i
// numeric filters and objectID filters over its records and enforces the limits measured live
// on 2026-10-14: a page is at most 100 hits, and nothing past the 500th hit of a query is
// served — asking for it is a 400, exactly as Algolia answers it.
type startupjobsFakeIndex struct {
	mu            sync.Mutex
	records       []map[string]any // the real hit shape, objectID/path/created_at_i varied
	queries       int
	failOnQuery   int  // 1-based query number to fail with a 503; 0 never
	notExhaustive bool // report every count as an estimate
	gotHeaders    map[string]string
	gotURL        string
	lowestBound   int64 // the smallest created_at_i>= bound any query asked for
}

func (f *startupjobsFakeIndex) PostJSONWithHeaders(_ context.Context, url string, headers map[string]string, body, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries++
	f.gotHeaders, f.gotURL = headers, url
	if f.queries == f.failOnQuery {
		return &StatusError{Method: http.MethodPost, Code: http.StatusServiceUnavailable, URL: url}
	}
	q := body.(startupjobsQuery)
	for _, nf := range q.NumericFilters {
		if b, ok := strings.CutPrefix(nf, "created_at_i>="); ok {
			if n, _ := strconv.ParseInt(b, 10, 64); f.lowestBound == 0 || n < f.lowestBound {
				f.lowestBound = n
			}
		}
	}
	if q.HitsPerPage > startupjobsPageSize {
		return fmt.Errorf("fake: hitsPerPage %d is clamped by the real index", q.HitsPerPage)
	}
	var ids map[string]bool
	if q.Filters != "" {
		ids = map[string]bool{}
		for _, clause := range strings.Split(q.Filters, " OR ") {
			id, ok := strings.CutPrefix(clause, "objectID:")
			if !ok {
				return fmt.Errorf("fake: unexpected filter clause %q", clause)
			}
			ids[id] = true
		}
	}
	var matched []map[string]any
	for _, r := range f.records {
		if ids != nil && !ids[r["objectID"].(string)] {
			continue
		}
		if startupjobsFakeMatches(r["created_at_i"].(float64), q.NumericFilters) {
			matched = append(matched, r)
		}
	}
	page := []map[string]any{}
	if q.HitsPerPage > 0 {
		start := q.Page * q.HitsPerPage
		if start >= startupjobsWindow {
			return &StatusError{Method: http.MethodPost, Code: http.StatusBadRequest, URL: url}
		}
		if end := min(start+q.HitsPerPage, len(matched), startupjobsWindow); start < end {
			page = matched[start:end]
		}
	}
	raw, err := json.Marshal(map[string]any{
		"hits": page, "nbHits": len(matched), "page": q.Page, "hitsPerPage": q.HitsPerPage,
		"exhaustiveNbHits": !f.notExhaustive,
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func startupjobsFakeMatches(created float64, filters []string) bool {
	for _, f := range filters {
		switch {
		case strings.HasPrefix(f, "created_at_i>="):
			n, _ := strconv.ParseFloat(strings.TrimPrefix(f, "created_at_i>="), 64)
			if created < n {
				return false
			}
		case strings.HasPrefix(f, "created_at_i<"):
			n, _ := strconv.ParseFloat(strings.TrimPrefix(f, "created_at_i<"), 64)
			if created >= n {
				return false
			}
		default:
			panic("fake: unexpected filter " + f)
		}
	}
	return true
}

// startupjobsRealHits returns the two hits of a real index page (testdata, captured live).
func startupjobsRealHits(t *testing.T) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile("testdata/startupjobs_algolia_page.json")
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Hits []map[string]any `json:"hits"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	return page.Hits
}

// startupjobsRecord clones the real Fonzi hit under a new id, creation time, title and employer.
func startupjobsRecord(t *testing.T, id int, created int64, title, company string) map[string]any {
	t.Helper()
	r := map[string]any{}
	for k, v := range startupjobsRealHits(t)[0] {
		r[k] = v
	}
	r["objectID"] = strconv.Itoa(id)
	r["path"] = fmt.Sprintf("/posting-%d", id)
	r["created_at_i"] = float64(created)
	r["title"] = title
	r["company_name"] = company
	return r
}

// startupjobsFakeFeed serves the real sitemap index fixture, the first posting file built from
// the given postings in the real file's shape, and the other five posting files empty.
type startupjobsFakeFeed struct {
	posts   []string // posting URLs listed in posts.xml.gz
	fail    string   // a URL to fail
	lastMod string   // overrides every posting file's lastmod when set
}

func (f startupjobsFakeFeed) GetXML(_ context.Context, url string, v any) error {
	if url == f.fail {
		return &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway, URL: url}
	}
	var raw []byte
	switch {
	case url == startupjobsSitemapIndex:
		b, err := os.ReadFile("testdata/startupjobs_sitemap_index.xml")
		if err != nil {
			return err
		}
		if f.lastMod != "" {
			b = []byte(strings.ReplaceAll(string(b), "2026-10-14T03:00", f.lastMod))
		}
		raw = b
	case strings.HasSuffix(url, "/posts.xml.gz"):
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
		for _, u := range f.posts {
			fmt.Fprintf(&b, `<url><loc>%s</loc><lastmod>2026-10-14T03:00:26+00:00</lastmod><changefreq>weekly</changefreq><priority>0.5</priority></url>`, u)
		}
		b.WriteString(`</urlset>`)
		raw = []byte(b.String())
	case strings.Contains(url, "/posts"):
		raw = []byte(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></urlset>`)
	default:
		return fmt.Errorf("fake: the adapter must read only posting sitemaps, asked for %s", url)
	}
	return xml.Unmarshal(raw, v)
}

// startupjobsFakeSite serves the homepage and posting pages from testdata, and records every
// URL asked for. status overrides a URL's answer with a bare status code.
type startupjobsFakeSite struct {
	mu      sync.Mutex
	home    []byte
	posting []byte
	status  map[string]int
	asked   []string
}

func newStartupjobsFakeSite(t *testing.T) *startupjobsFakeSite {
	t.Helper()
	home, err := os.ReadFile("testdata/startupjobs_home.html")
	if err != nil {
		t.Fatal(err)
	}
	posting, err := os.ReadFile("testdata/startupjobs_posting.html")
	if err != nil {
		t.Fatal(err)
	}
	return &startupjobsFakeSite{home: home, posting: posting, status: map[string]int{}}
}

func (s *startupjobsFakeSite) GetHTML(_ context.Context, url string) (*html.Node, error) {
	s.mu.Lock()
	s.asked = append(s.asked, url)
	code := s.status[url]
	s.mu.Unlock()
	if code != 0 {
		return nil, &StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	if url == startupjobsBaseURL+"/" {
		return html.Parse(bytes.NewReader(s.home))
	}
	return html.Parse(bytes.NewReader(s.posting))
}

func (s *startupjobsFakeSite) postingsAsked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, u := range s.asked {
		if u != startupjobsBaseURL+"/" {
			out = append(out, u)
		}
	}
	slices.Sort(out)
	return out
}

// newStartupjobsTest wires the adapter over the fakes with the fake clock and a backlog pick
// that always reads the first (newest) chunk.
func newStartupjobsTest(site *startupjobsFakeSite, feed startupjobsFakeFeed, idx *startupjobsFakeIndex, maxNew int) startupjobs {
	s := NewStartupJobs(site, feed, idx, maxNew).(startupjobs)
	s.now = func() time.Time { return startupjobsNow }
	s.pick = func(int) int { return 0 }
	return s
}

func TestStartupJobsMarkers(t *testing.T) {
	src := NewStartupJobs(nil, nil, nil, 0)
	if _, ok := src.(boardless); !ok {
		t.Error("startupjobs is one public catalogue and must be boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("startupjobs re-lists many employers' postings and must be an aggregator, or the dedup pass never suppresses its copies")
	}
	if _, ok := src.(CoverageGated); !ok {
		t.Error("startupjobs must be CoverageGated: its bodies cost Cloudflare-fronted requests")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("startupjobs lists the whole catalogue or fails, so it is swept by source")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "startupjobs") {
		t.Error("the taxonomy registry must know startupjobs as an aggregator")
	}
}

// The search key is a secured key with a validUntil a few hours out, so it is read from the
// homepage every run; a markup change must fail loudly rather than fall back to anything.
func TestStartupJobsReadsTheAlgoliaIndexFromTheHomepage(t *testing.T) {
	site := newStartupjobsFakeSite(t)
	root, err := html.Parse(bytes.NewReader(site.home))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := startupjobsIndexFromHome(root)
	if err != nil {
		t.Fatalf("startupjobsIndexFromHome: %v", err)
	}
	want := startupjobsIndex{AppID: "4CQMTMMK73", APIKey: "test-secured-search-key", Index: "Post_production"}
	if idx != want {
		t.Errorf("index = %+v, want %+v", idx, want)
	}

	for name, page := range map[string]string{
		"no tags":         `<html><head><title>x</title></head></html>`,
		"hostile app id":  strings.Replace(string(site.home), `content="4CQMTMMK73"`, `content="evil.example/x"`, 1),
		"hostile index":   strings.Replace(string(site.home), `content="Post_production"`, `content="../keys"`, 1),
		"missing api key": strings.Replace(string(site.home), `name="current-algolia-api-key-search"`, `name="other"`, 1),
	} {
		root, err := html.Parse(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := startupjobsIndexFromHome(root); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

func TestStartupJobsMapsARealHit(t *testing.T) {
	raw, err := os.ReadFile("testdata/startupjobs_algolia_page.json")
	if err != nil {
		t.Fatal(err)
	}
	var res startupjobsResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 || !res.ExhaustiveNbHits {
		t.Fatalf("fixture decoded to %d hits (exhaustive=%v), want 2 exhaustive", len(res.Hits), res.ExhaustiveNbHits)
	}

	job, ok := res.Hits[0].toJob()
	if !ok {
		t.Fatal("the Fonzi hit must map")
	}
	if job.ExternalID != "8724966" || job.URL != "https://startup.jobs/senior-software-engineer-fonzi-8724966" {
		t.Errorf("identity = %q %q", job.ExternalID, job.URL)
	}
	if job.Title != "Senior Software Engineer" || job.Company != "Fonzi" || job.Location != "San Francisco, United States" {
		t.Errorf("title/company/location = %q / %q / %q", job.Title, job.Company, job.Location)
	}
	if job.WorkMode != "onsite" || job.Remote || job.EmploymentType != "full_time" {
		t.Errorf("work mode %q remote %v employment %q, want onsite/false/full_time", job.WorkMode, job.Remote, job.EmploymentType)
	}
	if !slices.Equal(job.Countries, []string{"us"}) {
		t.Errorf("countries = %v, want [us]", job.Countries)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 150000 || job.SalaryMax == nil || *job.SalaryMax != 400000 ||
		job.SalaryCurrency != "USD" || job.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s, want 150000-400000 USD/year", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.PostedAt != nil {
		t.Errorf("PostedAt = %v; the index states no posting date worth trusting", job.PostedAt)
	}

	// McGill states a currency and an interval but no amount: no salary at all.
	mcgill, ok := res.Hits[1].toJob()
	if !ok {
		t.Fatal("the McGill hit must map")
	}
	if mcgill.SalaryMin != nil || mcgill.SalaryMax != nil || mcgill.SalaryCurrency != "" {
		t.Errorf("McGill salary = %v-%v %q, want none", mcgill.SalaryMin, mcgill.SalaryMax, mcgill.SalaryCurrency)
	}
	if !slices.Equal(mcgill.Countries, []string{"ca"}) {
		t.Errorf("McGill countries = %v, want [ca]", mcgill.Countries)
	}
}

// The real sitemap: the index names posting files beside companies, roles, locations...; a
// posting URL ends in the posting id the index keys it on; and the sitemap's generation is the
// OLDEST posting file's lastmod.
func TestStartupJobsReadsTheRealSitemap(t *testing.T) {
	raw, err := os.ReadFile("testdata/startupjobs_sitemap_posts.xml")
	if err != nil {
		t.Fatal(err)
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, u := range doc.URLs {
		ids = append(ids, startupjobsPostingID(u.Loc))
	}
	if want := []string{"10323575", "10323576", "10323577", "10442138", "10442139"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	for _, bad := range []string{"https://startup.jobs/companies/fonzi", "https://evil.example/x-123", "https://startup.jobs/a/b-12"} {
		if id := startupjobsPostingID(bad); id != "" {
			t.Errorf("startupjobsPostingID(%q) = %q, want none", bad, id)
		}
	}

	feed := startupjobsFakeFeed{posts: []string{"https://startup.jobs/senior-software-engineer-fonzi-8724966"}}
	s := newStartupjobsTest(newStartupjobsFakeSite(t), feed, &startupjobsFakeIndex{}, 0)
	listed, generated, err := s.sitemap(context.Background())
	if err != nil {
		t.Fatalf("sitemap: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "8724966" {
		t.Errorf("listed = %+v", listed)
	}
	if want := time.Date(2026, 10, 14, 3, 0, 23, 0, time.UTC); !generated.Equal(want) {
		t.Errorf("generated = %s, want the oldest posting file's %s", generated, want)
	}

	// A posting file that fails is a sixth of the catalogue, not an empty one.
	feed.fail = "https://cdn.startup.jobs/sitemaps/startupjobs/posts3.xml.gz"
	s = newStartupjobsTest(newStartupjobsFakeSite(t), feed, &startupjobsFakeIndex{}, 0)
	if _, _, err := s.sitemap(context.Background()); err == nil {
		t.Error("a failed posting sitemap must fail the run")
	}
}

// The window walk must read everything since the sitemap through a window of 500, narrowing
// whatever overflows it. The records are bunched the way the real index is — a dense recent
// hour, a sparser rest of the window — so slices are both narrowed and widened.
func TestStartupJobsWalkReadsEveryPostingThroughTheWindow(t *testing.T) {
	now := startupjobsNow.Unix()
	var records []map[string]any
	for i := range 2600 {
		created := now - int64(i%3600) // 1,500 inside the last hour
		if i >= 1500 {
			created = startupjobsSince + int64(i-1500)*15 // the rest of the window
		}
		records = append(records, startupjobsRecord(t, 9000000+i, created, "Backend Engineer", "Acme"))
	}
	records = append(records, startupjobsRecord(t, 1, startupjobsSince-1, "Older", "Acme")) // before the window
	idx := &startupjobsFakeIndex{records: records}
	s := newStartupjobsTest(newStartupjobsFakeSite(t), startupjobsFakeFeed{}, idx, 0)

	budget := &startupjobsBudget{left: 1000}
	hits, err := s.walk(context.Background(), startupjobsIndex{AppID: "4CQMTMMK73", APIKey: "k", Index: "Post_production"},
		startupjobsSince, now, budget)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(hits) != 2600 {
		t.Fatalf("walk read %d postings, want the 2600 in the window", len(hits))
	}
	ids := map[string]bool{}
	for _, h := range hits {
		if ids[h.ObjectID] || h.ObjectID == "1" {
			t.Fatalf("posting %s read twice or from before the window", h.ObjectID)
		}
		ids[h.ObjectID] = true
	}
	if spent := 1000 - budget.left; spent > startupjobsQueryBudget {
		t.Errorf("walking 2600 postings spent %d queries, more than a run's %d", spent, startupjobsQueryBudget)
	}
}

// More than 500 postings created in one second cannot be read through the window, and no
// narrower filter exists. Reading the first 500 and calling the slice complete is the silent
// truncation a source-scoped sweep turns into a mass close, so the crawl must fail instead.
func TestStartupJobsWalkFailsOnASecondItCannotNarrow(t *testing.T) {
	var records []map[string]any
	for i := range 600 {
		records = append(records, startupjobsRecord(t, 9000000+i, 1791936000, "Backend Engineer", "Acme"))
	}
	s := newStartupjobsTest(newStartupjobsFakeSite(t), startupjobsFakeFeed{}, &startupjobsFakeIndex{records: records}, 0)
	hits, err := s.walk(context.Background(), startupjobsIndex{AppID: "a", APIKey: "k", Index: "i"},
		startupjobsSince-86400, startupjobsNow.Unix(), &startupjobsBudget{left: 1000})
	if err == nil {
		t.Fatalf("walk returned %d postings from a slice it could not read whole; want an error", len(hits))
	}
	if !strings.Contains(err.Error(), "cannot be narrowed") {
		t.Errorf("error = %v, want the narrowing diagnosis", err)
	}
}

// An estimated count proves nothing about whether a slice was read whole.
func TestStartupJobsWalkRefusesAnEstimatedCount(t *testing.T) {
	idx := &startupjobsFakeIndex{
		records:       []map[string]any{startupjobsRecord(t, 9000000, startupjobsSince+10, "Backend Engineer", "Acme")},
		notExhaustive: true,
	}
	s := newStartupjobsTest(newStartupjobsFakeSite(t), startupjobsFakeFeed{}, idx, 0)
	_, err := s.walk(context.Background(), startupjobsIndex{AppID: "a", APIKey: "k", Index: "i"},
		startupjobsSince, startupjobsNow.Unix(), &startupjobsBudget{left: 10})
	if err == nil || !strings.Contains(err.Error(), "estimated") {
		t.Fatalf("err = %v, want the estimated-count refusal", err)
	}
}

// startupjobsCatalogue is a small but complete site: postings in the sitemap (old) and in the
// window (new), some stored already, some not technical, one from a covered employer.
func startupjobsCatalogue(t *testing.T) (startupjobsFakeFeed, *startupjobsFakeIndex) {
	old := startupjobsSince - 7*86400
	recent := startupjobsSince + 3600
	records := []map[string]any{
		startupjobsRecord(t, 100, old, "Backend Engineer", "Acme"),     // stored
		startupjobsRecord(t, 101, old, "Data Engineer", "Acme"),        // backlog, technical
		startupjobsRecord(t, 102, old, "Registered Nurse", "Acme"),     // backlog
		startupjobsRecord(t, 200, recent, "Platform Engineer", "Acme"), // window, stored
		startupjobsRecord(t, 201, recent, "Frontend Engineer", "Acme"), // window, new
		startupjobsRecord(t, 202, recent, "SRE", "Covered Co"),         // window, covered employer
	}
	feed := startupjobsFakeFeed{posts: []string{
		"https://startup.jobs/posting-100", "https://startup.jobs/posting-101",
		"https://startup.jobs/posting-102",
		"https://startup.jobs/posting-99", // removed since the sitemap: the index no longer knows it
	}}
	return feed, &startupjobsFakeIndex{records: records}
}

// The whole run: the sitemap refreshes what is stored, the window and the backlog bring the
// rest with their metadata, and a body is bought only for a posting the pipeline will keep. A
// covered or rejected posting is yielded body-less so the pipeline still decides and counts it.
func TestStartupJobsFetchNewGatedBuysOnlyBodiesThePipelineKeeps(t *testing.T) {
	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	s := newStartupjobsTest(site, feed, idx, 0)

	var askedAbout []string
	gate := DetailGate{
		Covered: func(names []string) map[string]bool {
			askedAbout = names
			return map[string]bool{"Covered Co": true}
		},
	}
	seen := func(id string) bool { return id == "100" || id == "200" }
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "startupjobs"}, seen, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(askedAbout) != 4 {
		t.Errorf("coverage asked about %v, want the four candidates' employers", askedAbout)
	}
	want := []string{"https://startup.jobs/posting-101", "https://startup.jobs/posting-102", "https://startup.jobs/posting-201"}
	if got := site.postingsAsked(); !slices.Equal(got, want) {
		t.Errorf("posting pages fetched = %v, want only %v", got, want)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs (%v), want 6", len(jobs), byID)
	}
	if j := byID["100"]; !j.SeenRefresh || j.URL != "https://startup.jobs/posting-100" {
		t.Errorf("stored sitemap posting = %+v, want a refresh by identity", j)
	}
	if j := byID["200"]; !j.SeenRefresh || j.Title != "Platform Engineer" {
		t.Errorf("stored window posting = %+v, want a refresh carrying its title", j)
	}
	for _, id := range []string{"101", "102", "201"} {
		if j := byID[id]; !strings.Contains(j.Description, "Fonzi Match Day") {
			t.Errorf("new posting %s description = %q, want the posting page's JobPosting body", id, j.Description)
		}
	}
	for _, id := range []string{"202"} {
		if j := byID[id]; j.Description != "" || j.SeenRefresh {
			t.Errorf("posting %s: description %q seenRefresh %v, want body-less and new", id, j.Description, j.SeenRefresh)
		}
	}
	if _, ok := byID["99"]; ok {
		t.Error("a posting the index no longer knows must not be yielded")
	}
	if idx.gotHeaders["X-Algolia-Application-Id"] != "4CQMTMMK73" || idx.gotHeaders["X-Algolia-API-Key"] != "test-secured-search-key" {
		t.Errorf("credentials sent = %v, want the homepage's", idx.gotHeaders)
	}
	if idx.gotURL != "https://4cqmtmmk73-dsn.algolia.net/1/indexes/Post_production/query" {
		t.Errorf("algolia url = %q", idx.gotURL)
	}
}

// fullCatalog lets the sweep close by source whatever a run did not list, so every way the
// listing can come back incomplete must fail the run instead.
func TestStartupJobsRefusesAnIncompleteListing(t *testing.T) {
	never := func(string) bool { return false }

	feed, idx := startupjobsCatalogue(t)
	idx.failOnQuery = 2
	if jobs, err := newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never); err == nil || jobs != nil {
		t.Errorf("a failed Algolia query: jobs=%d err=%v, want no jobs and an error", len(jobs), err)
	}

	feed, idx = startupjobsCatalogue(t)
	for i := range 40 { // the index holds far more pre-window postings than the sitemap lists
		idx.records = append(idx.records, startupjobsRecord(t, 300+i, startupjobsSince-86400, "Backend Engineer", "Acme"))
	}
	_, err := newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "sitemap lists") {
		t.Errorf("a short sitemap: err = %v, want the refusal", err)
	}

	feed, idx = startupjobsCatalogue(t)
	feed.lastMod = "2026-10-12T03:00"
	_, err = newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "sitemap generated") {
		t.Errorf("a stale sitemap: err = %v, want the refusal", err)
	}
}

// The budget bounds one run's posting-page requests; what it leaves out is not yielded, so it
// stays new and is offered again by a later run.
func TestStartupJobsHydrationBudgetDefersTheRest(t *testing.T) {
	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	jobs, err := newStartupjobsTest(site, feed, idx, 1).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(site.postingsAsked()) != 1 || len(jobs) != 1 {
		t.Errorf("fetched %d pages and yielded %d jobs, want 1 and 1", len(site.postingsAsked()), len(jobs))
	}
}

// A 404 is the platform saying the posting is gone: dropped, and not a reason to fail the run.
// A refusal is about the IP: the rest of the run's hydration stops, and a run that read nothing
// it needed fails, because "listed postings, read none" is how the Cloudflare wall looks.
func TestStartupJobsPostingPageFailures(t *testing.T) {
	never := func(string) bool { return false }

	feed, idx := startupjobsCatalogue(t)
	site := newStartupjobsFakeSite(t)
	site.status["https://startup.jobs/posting-101"] = http.StatusNotFound
	jobs, err := newStartupjobsTest(site, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err != nil {
		t.Fatalf("a gone posting failed the run: %v", err)
	}
	if len(jobs) != 5 {
		t.Errorf("got %d jobs, want the 5 readable ones", len(jobs))
	}

	// Every page gone is an answer about every posting, not the wall.
	feed, idx = startupjobsCatalogue(t)
	gone := newStartupjobsFakeSite(t)
	for _, id := range []int{100, 101, 102, 200, 201, 202} {
		gone.status[fmt.Sprintf("https://startup.jobs/posting-%d", id)] = http.StatusGone
	}
	if _, err := newStartupjobsTest(gone, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never); err != nil {
		t.Errorf("every posting gone failed the run: %v", err)
	}

	feed, idx = startupjobsCatalogue(t)
	refused := newStartupjobsFakeSite(t)
	for _, id := range []int{100, 101, 102, 200, 201, 202} {
		refused.status[fmt.Sprintf("https://startup.jobs/posting-%d", id)] = http.StatusForbidden
	}
	_, err = newStartupjobsTest(refused, feed, idx, 0).FetchNew(context.Background(), CompanyEntry{}, never)
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want the read-none failure", err)
	}
	if n := len(refused.postingsAsked()); n > startupjobsDetailWorkers {
		t.Errorf("asked for %d posting pages after the edge refused; want at most the %d already in flight", n, startupjobsDetailWorkers)
	}
}

// The window reaches back its span — unless the sitemap is fresher than that, when it reaches
// back only to an hour before the sitemap was written: the sitemap holds the rest.
func TestStartupJobsWindowReachesBackToTheSitemapOrTheSpan(t *testing.T) {
	for _, tc := range []struct {
		now  time.Time
		want time.Time
	}{
		{startupjobsNow, startupjobsNow.Add(-startupjobsWindowSpan)},
		{time.Date(2026, 10, 14, 4, 0, 0, 0, time.UTC), time.Date(2026, 10, 14, 2, 0, 23, 0, time.UTC)},
	} {
		feed, idx := startupjobsCatalogue(t)
		s := newStartupjobsTest(newStartupjobsFakeSite(t), feed, idx, 0)
		s.now = func() time.Time { return tc.now }
		if _, err := s.crawl(context.Background(), func(string) bool { return false }); err != nil {
			t.Fatalf("crawl at %s: %v", tc.now, err)
		}
		if idx.lowestBound != tc.want.Unix() {
			t.Errorf("at %s the window reached back to %d, want %d (%s)", tc.now, idx.lowestBound, tc.want.Unix(), tc.want)
		}
	}
}
