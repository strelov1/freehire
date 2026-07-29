package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// recordingJSON is a JSONGetter that records every URL it is asked for and replies with the first
// canned body whose match substring the URL contains, so a test can assert on the query string the
// adapter builds — the feed's traps are all in the query, not the body.
type recordingJSON struct {
	routes []struct{ match, body string }
	urls   []string
	err    error
}

func (r *recordingJSON) route(match, body string) *recordingJSON {
	r.routes = append(r.routes, struct{ match, body string }{match, body})
	return r
}

func (r *recordingJSON) GetJSON(_ context.Context, u string, v any) error {
	r.urls = append(r.urls, u)
	if r.err != nil {
		return r.err
	}
	for _, rt := range r.routes {
		if strings.Contains(u, rt.match) {
			return json.Unmarshal([]byte(rt.body), v)
		}
	}
	return json.Unmarshal([]byte(`{"data":[]}`), v)
}

// query returns the parsed query of the n-th recorded request.
func (r *recordingJSON) query(t *testing.T, n int) url.Values {
	t.Helper()
	if n >= len(r.urls) {
		t.Fatalf("only %d request(s) recorded, wanted #%d", len(r.urls), n)
	}
	u, err := url.Parse(r.urls[n])
	if err != nil {
		t.Fatalf("parse recorded url: %v", err)
	}
	return u.Query()
}

func TestWhatJobsProvider(t *testing.T) {
	if got := NewWhatJobs(nil, "7065").Provider(); got != "whatjobs" {
		t.Errorf("Provider() = %q, want whatjobs", got)
	}
}

// The feed aggregates many employers under one account, so it is an aggregator (its copy may
// lose to a first-party ATS posting of the same role). It is NOT boardless — the board carries
// the search keyword and an entry without one cannot be crawled — and NOT fullCatalog, because
// one crawl reads a keyword's first pages, never the whole catalogue.
func TestWhatJobsIsKeywordBoardedAggregator(t *testing.T) {
	s := NewWhatJobs(nil, "7065")

	if _, ok := s.(aggregator); !ok {
		t.Error("whatjobs should implement the aggregator marker")
	}
	if _, ok := s.(boardless); ok {
		t.Error("whatjobs must NOT be boardless: the keyword board is mandatory")
	}
	if _, ok := s.(fullCatalog); ok {
		t.Error("whatjobs must NOT be fullCatalog: a crawl sees only a keyword slice")
	}
}

// The board's keyword drives the search, and the account credential rides the query. limit is
// pinned to the feed's maximum: anything larger is silently clamped, and limit=1 combined with a
// keyword returns an empty page with per_page 0 — a pagination bug in the feed.
func TestWhatJobsRequestCarriesPublisherAndKeyword(t *testing.T) {
	fake := (&recordingJSON{}).route("jobs.json", `{"data":[]}`)

	if _, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{
		Company: "WhatJobs — Backend",
		Board:   "backend engineer",
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	q := fake.query(t, 0)
	for _, want := range []struct{ key, val string }{
		{"publisher", "7065"},
		{"keyword", "backend engineer"},
		{"limit", "50"},
		{"page", "1"},
	} {
		if got := q.Get(want.key); got != want.val {
			t.Errorf("query %s = %q, want %q", want.key, got, want.val)
		}
	}
	if q.Get("user_ip") == "" {
		t.Error("user_ip is mandatory and must be sent")
	}
}

// A slash anywhere in the user_agent value makes the edge redirect the request with the value
// corrupted (Mozilla/5.0 arrives as Mozilla%215.0), which is why every code sample in the vendor's
// own documentation fails. The parameter is optional, so the adapter simply never sends it.
func TestWhatJobsRequestSendsNoUserAgent(t *testing.T) {
	fake := (&recordingJSON{}).route("jobs.json", `{"data":[]}`)

	if _, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "devops"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if got, ok := fake.query(t, 0)["user_agent"]; ok {
		t.Errorf("user_agent must not be sent, got %q", got)
	}
	for _, u := range fake.urls {
		if strings.Contains(u, "user_agent") {
			t.Errorf("url mentions user_agent: %s", u)
		}
	}
}

// whatjobsPage is the feed's real envelope, trimmed. snippet carries the FULL description despite
// its name, and the tracked url is what the native id is read from.
const whatjobsPage = `{"data":[
{"title":"Lead Backend Engineer","company":"Duolingo","location":"New York","postcode":"10001",
 "snippet":"<p>Own the scoring platform.</p>\n#J-18808-Ljbffr","age":"July 28th, 2026","age_days":1,
 "salary":"0.000000 - 0.000000","job_type":"","logo":null,
 "url":"https://www.whatjobs.com/pub_api__cpl__2737655843__7065?utm_campaign=publisher&utm_medium=api&utm_source=7065&geoID=25088"},
{"title":"No tracked id","company":"Nowhere","location":"Austin","postcode":"73301",
 "snippet":"<p>Should be skipped.</p>","age_days":3,"url":"https://www.whatjobs.com/some-other-page"}
],"total":350,"per_page":"50","current_page":1,"last_page":7}`

// The native id is the tracked URL's pub_api__cpl__<id>__<publisher> segment; a posting whose URL
// lacks it is dropped rather than stored under a guessed id, which would collide on the dedup key.
// The tracked URL is stored verbatim: it is not IP-bound, so a stored copy stays valid for any
// later visitor and the publisher attribution survives.
func TestWhatJobsFetchMapsPostingsAndSkipsUntrackedOnes(t *testing.T) {
	fake := (&recordingJSON{}).route("page=1", whatjobsPage)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{
		Company: "WhatJobs — Backend",
		Board:   "backend engineer",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the untracked posting dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "2737655843" {
		t.Errorf("ExternalID = %q, want the cpl id 2737655843", j.ExternalID)
	}
	// The employer comes from the posting, never from the entry's display label.
	if j.Company != "Duolingo" {
		t.Errorf("Company = %q, want Duolingo (not the board entry's label)", j.Company)
	}
	if j.Title != "Lead Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if !strings.Contains(j.URL, "pub_api__cpl__2737655843__7065") {
		t.Errorf("URL lost the tracked path: %q", j.URL)
	}
	if !strings.Contains(j.URL, "utm_source=7065") {
		t.Errorf("URL lost its attribution query: %q", j.URL)
	}
	if !strings.Contains(j.Description, "Own the scoring platform") {
		t.Errorf("Description lost the body: %q", j.Description)
	}
}

// 96% of the feed's descriptions end with a reseller's tracking signature. It is not content —
// it identifies the network the posting was resold through — so it must not reach the catalogue.
func TestWhatJobsStripsResellerSignature(t *testing.T) {
	fake := (&recordingJSON{}).route("page=1", whatjobsPage)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "backend engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs")
	}

	got := jobs[0].Description
	for _, marker := range []string{"#J-18808-Ljbffr", "Ljbffr"} {
		if strings.Contains(got, marker) {
			t.Errorf("Description still carries the reseller marker %q: %q", marker, got)
		}
	}
	if !strings.Contains(got, "Own the scoring platform") {
		t.Errorf("stripping ate the real body: %q", got)
	}
}

// The feed names a city but no country, and its cities collide with better-known foreign ones
// (London is Ohio here, Vienna is Virginia). The publisher id is per-country by the vendor's
// design and this account serves US inventory, so the country is stated rather than left to be
// guessed from a city name — without it a third of the postings resolve to no country at all.
func TestWhatJobsStatesTheAccountCountry(t *testing.T) {
	fake := (&recordingJSON{}).route("page=1", whatjobsPage)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "backend engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs")
	}

	got := jobs[0].Location
	if !strings.Contains(got, "New York") {
		t.Errorf("Location lost the city: %q", got)
	}
	if !strings.Contains(got, "United States") {
		t.Errorf("Location = %q, want the account's country stated", got)
	}
}

// A posting with no city still belongs to the account's country, and must not arrive with a
// dangling separator the geography tokenizer would read as an empty token.
func TestWhatJobsCountryOnlyWhenCityMissing(t *testing.T) {
	page := `{"data":[{"title":"Remote SRE","company":"Acme","location":"","postcode":"",
	 "snippet":"<p>Body.</p>","url":"https://www.whatjobs.com/pub_api__cpl__99__7065"}]}`
	fake := (&recordingJSON{}).route("page=1", page)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "sre"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if got := jobs[0].Location; got != "United States" {
		t.Errorf("Location = %q, want a bare %q", got, "United States")
	}
}

// age/age_days report how long the record has sat in the reseller's index, not when the employer
// published the role — postings from unrelated companies routinely share one value — so they must
// never become a posting date. Leaving it unset lets freshness fall back to first-seen, which is
// true if less precise.
func TestWhatJobsNeverDatesAPostingFromAge(t *testing.T) {
	fake := (&recordingJSON{}).route("page=1", whatjobsPage)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "backend engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs")
	}
	if jobs[0].PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil: age_days is the reseller's index age", *jobs[0].PostedAt)
	}
}

// The publisher id is the account credential, so it lives in the environment and the provider is
// absent without it — an unconfigured environment must leave a provider that cannot crawl
// unregistered rather than register one that fails every board.
func TestWhatJobsRegisteredOnlyWhenPublisherSet(t *testing.T) {
	t.Setenv("WHATJOBS_PUBLISHER_ID", "")
	if _, ok := All(nil)["whatjobs"]; ok {
		t.Error("All() should NOT register whatjobs without WHATJOBS_PUBLISHER_ID")
	}

	t.Setenv("WHATJOBS_PUBLISHER_ID", "7065")
	if _, ok := All(nil)["whatjobs"]; !ok {
		t.Error("All() should register whatjobs when WHATJOBS_PUBLISHER_ID is set")
	}
	if !slices.Contains(FilterableProviders(), "whatjobs") {
		t.Error("FilterableProviders() should include whatjobs when configured")
	}
}

// The board file must load and validate against the real registry, so a malformed entry fails the
// build rather than a cron run. Validation needs the provider registered, hence the env.
func TestWhatJobsBoardFileValidates(t *testing.T) {
	t.Setenv("WHATJOBS_PUBLISHER_ID", "7065")

	cfg, err := LoadConfig("../../sources/whatjobs.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/whatjobs.yml fails validation: %v", err)
	}
	if len(cfg.Sources) == 0 {
		t.Fatal("sources/whatjobs.yml lists no keyword slices")
	}
	// Every entry's board is the search keyword — the adapter has nothing to crawl without one.
	for _, e := range cfg.Sources {
		if strings.TrimSpace(e.Board) == "" {
			t.Errorf("entry %q has no keyword board", e.Company)
		}
	}
}

// The feed answers a rejected publisher id with 410 and an "Invalid Publisher" body. That is a
// board-level failure: the run must count it and close nothing, rather than read an empty result as
// a keyword that ran dry.
func TestWhatJobsRejectedPublisherIsABoardFailure(t *testing.T) {
	fake := &recordingJSON{err: &StatusError{Method: "GET", Code: 410, URL: whatjobsFeedURL}}

	jobs, err := NewWhatJobs(fake, "bad-id").Fetch(context.Background(), CompanyEntry{Board: "devops"})
	if err == nil {
		t.Fatal("Fetch should fail when the feed rejects the publisher id")
	}
	if jobs != nil {
		t.Errorf("Fetch returned %d jobs alongside the error, want none", len(jobs))
	}
	if !strings.Contains(err.Error(), "devops") {
		t.Errorf("error should name the keyword that failed: %v", err)
	}
}

// whatjobsRows builds a page of n postings with distinct tracked ids offset by base.
func whatjobsRows(base, n int) string {
	rows := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, fmt.Sprintf(
			`{"title":"Engineer %[1]d","company":"Acme","location":"Austin","snippet":"<p>Body.</p>",`+
				`"url":"https://www.whatjobs.com/pub_api__cpl__%[1]d__7065"}`, base+i))
	}
	return `{"data":[` + strings.Join(rows, ",") + `]}`
}

// The feed post-filters duplicates AFTER selecting the page, so a page requested with limit=50
// routinely comes back with 44 rows while more pages remain. Treating a short page as the last one
// would silently truncate every keyword — the most costly mistake available here.
func TestWhatJobsShortPageDoesNotEndPagination(t *testing.T) {
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(100, 44)). // short, yet not the last page
		route("page=2", whatjobsRows(200, 50)).
		route("page=3", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 94 {
		t.Errorf("got %d jobs, want 94 (44 + 50): a short page must not stop the crawl", len(jobs))
	}
	if len(fake.urls) != 3 {
		t.Errorf("requested %d pages, want 3 (stop on the EMPTY page, not the short one)", len(fake.urls))
	}
}

// An empty page is the real end-of-results signal.
func TestWhatJobsEmptyPageEndsPagination(t *testing.T) {
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(1, 50)).
		route("page=2", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 50 {
		t.Errorf("got %d jobs, want 50", len(jobs))
	}
	if len(fake.urls) != 2 {
		t.Errorf("requested %d pages, want 2", len(fake.urls))
	}
}

// The feed serves nothing past roughly two thousand pages while reporting a far larger total, so a
// crawl must be bounded rather than trusting the reported page count. A keyword that never runs dry
// stops at the budget.
func TestWhatJobsPageBudgetBoundsAKeyword(t *testing.T) {
	// Every page is full, so only the budget can stop this crawl.
	fake := (&recordingJSON{}).route("jobs.json", whatjobsRows(1, 50))

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.urls) != whatjobsMaxPages {
		t.Errorf("requested %d pages, want the budget %d", len(fake.urls), whatjobsMaxPages)
	}
	if len(jobs) == 0 {
		t.Error("the bounded crawl should still return the pages it read")
	}
}

// The feed post-filters duplicates per page, which means the same posting can surface on more than
// one page of the same keyword. Returning it repeatedly would have the pipeline upsert one row
// hundreds of times over a deep crawl, so the adapter collapses a keyword's repeats itself.
func TestWhatJobsCollapsesRepeatsWithinAKeyword(t *testing.T) {
	// Both pages carry the SAME ids, then the feed runs dry.
	fake := (&recordingJSON{}).
		route("page=1", whatjobsRows(1, 10)).
		route("page=2", whatjobsRows(1, 10)).
		route("page=3", `{"data":[]}`)

	jobs, err := NewWhatJobs(fake, "7065").Fetch(context.Background(), CompanyEntry{Board: "engineer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 10 {
		t.Errorf("got %d jobs, want 10 distinct ids (the repeated page collapsed)", len(jobs))
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[j.ExternalID] {
			t.Errorf("external id %q returned twice", j.ExternalID)
		}
		seen[j.ExternalID] = true
	}
}

// The postings cannot be liveness-probed (the url is a billing landing page, not the employer's
// posting) and a crawl reads only the first pages of each keyword, so the sweep must wait out
// page drift instead of closing on the 48-hour default.
func TestWhatJobsDeclaresWideSweepGrace(t *testing.T) {
	s, ok := NewWhatJobs(nil, "7065").(sweepGrace)
	if !ok {
		t.Fatal("whatjobs should declare a sweepGrace window")
	}
	if got := s.sweepGrace(); got != 14*24*time.Hour {
		t.Errorf("sweepGrace() = %v, want 14 days", got)
	}
}
