package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// errSeekTest is the failure the fake returns for a page the test marked as failing.
var errSeekTest = errors.New("seek: boom")

// seekPost builds a minimal usable listing posting (id, title, employer).
func seekPost(id, title, company string) seekPosting {
	return seekPosting{ID: id, Title: title, CompanyName: company}
}

// seekArrangements builds the nested work-arrangement shape SEEK's listing carries.
func seekArrangements(labels ...string) seekWorkArrangements {
	var wa seekWorkArrangements
	for _, l := range labels {
		var one seekWorkArrangement
		one.Label.Text = l
		wa.Data = append(wa.Data, one)
	}
	return wa
}

// seekFake serves search pages keyed by their page param and GraphQL details keyed by job id, so
// one fake drives both stages of the crawl.
type seekFake struct {
	searchByPage map[int][]seekPosting // page -> postings that page yields
	searchErr    map[int]bool          // page -> GetJSON returns an error
	searchURLs   []string              // search URLs requested, in order
	detailByID   map[string]string     // job id -> the GraphQL content it answers
	detailErr    map[string]bool       // job id -> PostJSON returns an error
	// The adapter fans its detail fetches out across a worker pool, so these recorders are written
	// from several goroutines at once. Assertions read them after FetchNew has joined the pool.
	mu         sync.Mutex
	detailHits []string // job ids whose detail was requested
	detailURL  string   // the endpoint the detail call went to
}

func (f *seekFake) GetJSON(_ context.Context, u string, v any) error {
	f.searchURLs = append(f.searchURLs, u)
	pu, err := url.Parse(u)
	if err != nil {
		return err
	}
	page := 0
	if p := pu.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			page = n
		}
	}
	if f.searchErr[page] {
		return errSeekTest
	}
	out, ok := v.(*seekSearchPage)
	if !ok {
		return errSeekTest
	}
	out.Data = f.searchByPage[page]
	return nil
}

// PostJSON serves the GraphQL detail. It records the job id every detail call asked for, so a test
// can assert that a posting the catalogue already holds costs no request at all.
func (f *seekFake) PostJSON(_ context.Context, u string, body, v any) error {
	id := seekTestJobID(body)
	f.mu.Lock()
	f.detailHits = append(f.detailHits, id)
	f.detailURL = u
	f.mu.Unlock()
	if f.detailErr[id] {
		return errSeekTest
	}
	out, ok := v.(*seekDetailResponse)
	if !ok {
		return errSeekTest
	}
	out.Data.JobDetails.Job.Content = f.detailByID[id]
	return nil
}

// seekTestJobID digs the jobId out of the GraphQL request body the adapter built.
func seekTestJobID(body any) string {
	b, err := json.Marshal(body)
	if err != nil {
		return ""
	}
	var req struct {
		Variables struct {
			JobID string `json:"jobId"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return ""
	}
	return req.Variables.JobID
}

func TestSeekUnknownMarketFailsTheBoard(t *testing.T) {
	fake := &seekFake{}
	_, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{
		Company: "SEEK Nowhere", Region: "xx", Board: "6287",
	})
	if err == nil {
		t.Fatal("Fetch with an unknown market: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "xx") {
		t.Errorf("error should name the unknown market, got %q", err)
	}
	if len(fake.searchURLs) != 0 {
		t.Errorf("an unknown market must not issue a request, got %v", fake.searchURLs)
	}
}

func TestSeekSearchURLCarriesMarketAndSlice(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {seekPost("1", "Dev", "Co")}}}
	if _, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6287"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.searchURLs) == 0 {
		t.Fatal("no search request issued")
	}
	pu, err := url.Parse(fake.searchURLs[0])
	if err != nil {
		t.Fatalf("parse search URL: %v", err)
	}
	if pu.Host != "www.seek.com.au" {
		t.Errorf("host = %q, want www.seek.com.au", pu.Host)
	}
	q := pu.Query()
	for param, want := range map[string]string{
		"siteKey":           "AU-Main",
		"where":             "All Australia",
		"subclassification": "6287",
		"page":              "1",
		"sortmode":          "ListedDate",
	} {
		if got := q.Get(param); got != want {
			t.Errorf("%s = %q, want %q", param, got, want)
		}
	}
	// The result window is reached in as few requests as possible, so the walk asks for full pages.
	if q.Get("pageSize") != "100" {
		t.Errorf("pageSize = %q, want 100", q.Get("pageSize"))
	}
}

func TestSeekWalksPagesUntilOneAddsNothingNew(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{
		1: {seekPost("1", "A", "Co"), seekPost("2", "B", "Co")},
		2: {seekPost("3", "C", "Co")},
		// Page 3 repeats page 2's posting: SEEK clamping past its last page, not new inventory.
		3: {seekPost("3", "C", "Co")},
		4: {seekPost("4", "Never reached", "Co")},
	}}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6287"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (pages 1-2, page 3 adds nothing)", len(jobs))
	}
	if len(fake.searchURLs) != 3 {
		t.Errorf("requested %d pages, want 3 (the walk stops on the page that adds nothing)", len(fake.searchURLs))
	}
}

// SEEK serves at most ~550 results per query, so a well-behaved edge ends the walk on its own. This
// pins the backstop for one that does not: an edge answering every page with fresh postings must
// not loop the crawl forever.
func TestSeekWalkIsBackstoppedByAPageCeiling(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{}}
	for page := 1; page <= 50; page++ {
		fake.searchByPage[page] = []seekPosting{seekPost(strconv.Itoa(page), "Endless", "Co")}
	}
	if _, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6287"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.searchURLs) >= 50 {
		t.Fatalf("walk requested %d pages: it is not backstopped", len(fake.searchURLs))
	}
	// The ceiling must still clear SEEK's own window (~550 results at 100 per page).
	if want := 550 / seekPageSize; len(fake.searchURLs) < want {
		t.Errorf("walk stopped after %d pages, too few to reach SEEK's ~550-result window", len(fake.searchURLs))
	}
}

// The repository-wide rule for a paginated listing walk: the FIRST page failing is a board-level
// error, a LATER page failing ends the walk with what it gathered. seek is not a fullCatalog source,
// so a partial crawl is safe and only a wholly unreachable board should be reported.
func TestSeekFirstPageFailureFailsTheBoard(t *testing.T) {
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {seekPost("1", "A", "Co")}},
		searchErr:    map[int]bool{1: true},
	}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6287"})
	if err == nil {
		t.Fatalf("first page failing: want a board-level error, got %d jobs", len(jobs))
	}
}

func TestSeekLaterPageFailureKeepsWhatItGathered(t *testing.T) {
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {seekPost("1", "A", "Co"), seekPost("2", "B", "Co")}},
		searchErr:    map[int]bool{2: true},
	}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6287"})
	if err != nil {
		t.Fatalf("later page failing must not fail the board: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2 (page 1 survives page 2's failure)", len(jobs))
	}
}

func TestSeekMapsListingFieldsToAJob(t *testing.T) {
	p := seekPost("93497500", "Senior Go Engineer", "Atlassian")
	p.ListingDate = "2026-07-22T11:47:32Z"
	p.Locations = []seekLocation{{Label: "Smithfield, Sydney NSW", CountryCode: "AU"}}

	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {p}}}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6290"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "93497500" || j.Title != "Senior Go Engineer" || j.Company != "Atlassian" {
		t.Errorf("id/title/company wrong: %q / %q / %q", j.ExternalID, j.Title, j.Company)
	}
	// The /job/<id> page is Cloudflare-gated for our crawler but is the URL a human needs.
	if j.URL != "https://www.seek.com.au/job/93497500" {
		t.Errorf("URL = %q, want the market host's /job/<id>", j.URL)
	}
	if j.Location != "Smithfield, Sydney NSW" {
		t.Errorf("Location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "au" {
		t.Errorf("Countries = %v, want [au] from the posting's structured country code", j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("PostedAt = %v, want 2026-07-22", j.PostedAt)
	}
}

// Roughly one posting in thirty has no profiled employer; advertiser.description carries the name
// the employer typed instead — and sometimes SEEK's "Private Advertiser" placeholder, which must
// never become a company.
func TestSeekResolvesEmployerPerPosting(t *testing.T) {
	profiled := seekPost("1", "Profiled", "Atlassian")
	profiled.Advertiser.Description = "Atlassian Pty Ltd"

	advertiserOnly := seekPost("2", "Advertiser only", "")
	advertiserOnly.Advertiser.Description = "Proxmox Server Solutions GmbH"

	placeholder := seekPost("3", "Private", "")
	placeholder.Advertiser.Description = "Private Advertiser"

	nameless := seekPost("4", "Nameless", "")
	noID := seekPost("", "No id", "Co")

	fake := &seekFake{searchByPage: map[int][]seekPosting{
		1: {profiled, advertiserOnly, placeholder, nameless, noID},
	}}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6290"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got := map[string]string{}
	for _, j := range jobs {
		got[j.ExternalID] = j.Company
	}
	if len(got) != 2 {
		t.Fatalf("kept %d postings (%v), want 2 — placeholder, nameless and id-less are dropped", len(got), got)
	}
	if got["1"] != "Atlassian" {
		t.Errorf("profiled employer = %q, want Atlassian (the profile wins over the advertiser)", got["1"])
	}
	if got["2"] != "Proxmox Server Solutions GmbH" {
		t.Errorf("advertiser fallback = %q", got["2"])
	}
}

func TestSeekWorkModePrefersTheMostRemoteArrangement(t *testing.T) {
	cases := []struct {
		arrangements []string
		want         string
	}{
		{[]string{"Remote"}, "remote"},
		{[]string{"Hybrid"}, "hybrid"},
		{[]string{"On-site"}, "onsite"},
		{[]string{"On-site", "Hybrid"}, "hybrid"},
		{[]string{"On-site", "Remote"}, "remote"},
		// Unstated must stay empty so the pipeline's location heuristic decides instead.
		{nil, ""},
		{[]string{"Something SEEK invented"}, ""},
	}
	for _, c := range cases {
		p := seekPost("1", "T", "Co")
		p.WorkArrangements = seekArrangements(c.arrangements...)
		if got := p.workMode(); got != c.want {
			t.Errorf("workMode(%v) = %q, want %q", c.arrangements, got, c.want)
		}
	}
}

func TestSeekRemoteFlagTracksWorkMode(t *testing.T) {
	remote := seekPost("1", "Remote role", "Co")
	remote.WorkArrangements = seekArrangements("Remote")
	onsite := seekPost("2", "Onsite role", "Co")
	onsite.WorkArrangements = seekArrangements("On-site")

	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {remote, onsite}}}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6290"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !jobs[0].Remote || jobs[0].WorkMode != "remote" {
		t.Errorf("remote posting: Remote=%v WorkMode=%q", jobs[0].Remote, jobs[0].WorkMode)
	}
	if jobs[1].Remote {
		t.Errorf("on-site posting must not be marked Remote")
	}
}

func TestSeekEmploymentTypeMapping(t *testing.T) {
	for _, c := range []struct {
		workTypes []string
		want      string
	}{
		{[]string{"Full time"}, "full_time"},
		{[]string{"Part time"}, "part_time"},
		{[]string{"Contract/Temp"}, "contract"},
		{[]string{"Casual/Vacation"}, ""},
		{nil, ""},
	} {
		if got := seekEmploymentType(c.workTypes); got != c.want {
			t.Errorf("seekEmploymentType(%v) = %q, want %q", c.workTypes, got, c.want)
		}
	}
}

// SEEK's salaryLabel is free text — anything from "$75,000 – $85,000 per year" to "160000" to
// "Rates Negotiable" — so it belongs in the description, not in Job's structured salary fields.
func TestSeekSalaryLabelIsFoldedIntoTheDescription(t *testing.T) {
	paid := seekPost("1", "Paid", "Co")
	paid.SalaryLabel = "$75,000 – $85,000 per year"
	silent := seekPost("2", "Silent", "Co")

	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {paid, silent}}}
	jobs, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "au", Board: "6290"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(jobs[0].Description, "$75,000 – $85,000 per year") {
		t.Errorf("salary label missing from description: %q", jobs[0].Description)
	}
	if jobs[0].SalaryMin != nil || jobs[0].SalaryMax != nil || jobs[0].SalaryCurrency != "" {
		t.Error("a free-text label must not populate the structured salary fields")
	}
	if jobs[1].Description != "" {
		t.Errorf("no salary label must yield no paragraph, got %q", jobs[1].Description)
	}
}

// The listing carries only a one-line teaser, so bodies cost a GraphQL call each. Spending one on a
// posting the catalogue already holds would make every crawl cost the whole catalogue.
func TestSeekFetchNewHydratesOnlyPostingsTheCatalogueLacks(t *testing.T) {
	fresh := seekPost("101", "New role", "Co")
	known := seekPost("202", "Known role", "Co")
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {fresh, known}},
		detailByID:   map[string]string{"101": "<p>Full body</p>", "202": "<p>Should never be asked for</p>"},
	}
	src, ok := NewSeek(fake).(HydratingSource)
	if !ok {
		t.Fatal("seek must implement HydratingSource")
	}
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Region: "au", Board: "6290"},
		func(id string) bool { return id == "202" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if !slices.Equal(fake.detailHits, []string{"101"}) {
		t.Errorf("detail requested for %v, want only [101]", fake.detailHits)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !strings.Contains(byID["101"].Description, "Full body") {
		t.Errorf("new posting was not hydrated: %q", byID["101"].Description)
	}
	if byID["101"].SeenRefresh {
		t.Error("a newly hydrated posting must not be marked SeenRefresh")
	}
	if !byID["202"].SeenRefresh {
		t.Error("a posting the catalogue holds must be marked SeenRefresh so the pipeline only refreshes liveness")
	}
	if byID["202"].Description != "" {
		t.Errorf("a SeenRefresh posting must carry no body, got %q", byID["202"].Description)
	}
}

func TestSeekDetailIsAppendedAfterTheSalaryParagraph(t *testing.T) {
	p := seekPost("101", "Paid role", "Co")
	p.SalaryLabel = "$120,000 per year"
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {p}},
		detailByID:   map[string]string{"101": "<p>What you will do</p>"},
	}
	jobs, err := NewSeek(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Region: "au", Board: "6290"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if fake.detailURL != "https://www.seek.com.au/graphql" {
		t.Errorf("detail endpoint = %q, want the market host's /graphql", fake.detailURL)
	}
	salary, body := strings.Index(jobs[0].Description, "$120,000"), strings.Index(jobs[0].Description, "What you will do")
	if salary < 0 || body < 0 || salary > body {
		t.Errorf("want the salary paragraph then the body, got %q", jobs[0].Description)
	}
}

// A missing body is not a reason to lose a posting: the title, employer and facets are still worth
// having, and the next crawl can hydrate it.
func TestSeekFailedDetailKeepsTheListOnlyPosting(t *testing.T) {
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {seekPost("1", "Transport fails", "Co"), seekPost("2", "Empty body", "Co")}},
		detailErr:    map[string]bool{"1": true},
		detailByID:   map[string]string{"2": "   "},
	}
	jobs, err := NewSeek(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Region: "au", Board: "6290"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 — a failed detail never drops a posting", len(jobs))
	}
	for _, j := range jobs {
		if j.Description != "" {
			t.Errorf("posting %s: want an empty body, got %q", j.ExternalID, j.Description)
		}
	}
}

// seek re-lists vacancies employers also post on their own ATS, so the cross-source dedup pass must
// be able to prefer the first-party copy. It still needs a board to bound the crawl, so unlike the
// boardless aggregators it must NOT carry that marker.
func TestSeekIsAnAggregatorButNotBoardless(t *testing.T) {
	src := NewSeek(&seekFake{})
	if _, ok := src.(aggregator); !ok {
		t.Error("seek must be an aggregator so its copies lose to first-party ATS postings")
	}
	if _, ok := src.(boardless); ok {
		t.Error("seek must not be boardless: the board selects the slice to crawl")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "seek") {
		t.Error("seek missing from AggregatorProviders")
	}
}

// SEEK stops serving results past ~550, so the busiest slices have a tail no crawl reaches. On the
// default window that tail would be closed and reopened as it drifts, writing a phantom removal
// each cycle — and SEEK's job pages are interstitial-gated, so liveness cannot settle it instead.
func TestSeekDeclaresAWiderSweepGrace(t *testing.T) {
	got, ok := SweepGraceWindows(Taxonomy())["seek"]
	if !ok {
		t.Fatal("seek must declare a sweep-grace window")
	}
	if got <= DefaultSweepGrace {
		t.Errorf("sweep grace = %v, want wider than the %v default", got, DefaultSweepGrace)
	}
	if want := 14 * 24 * time.Hour; got != want {
		t.Errorf("sweep grace = %v, want %v", got, want)
	}
}

func TestSeekIsRegisteredAndItsBoardsValidate(t *testing.T) {
	src, ok := All(nil)["seek"]
	if !ok {
		t.Fatal(`All(nil)["seek"] missing`)
	}
	if src.Provider() != "seek" {
		t.Errorf("Provider() = %q, want seek", src.Provider())
	}
	cfg := Config{Sources: []CompanyEntry{
		{Company: "SEEK Australia — Engineering - Software", Provider: "seek", Region: "au", Board: "6290"},
		{Company: "SEEK New Zealand — Engineering - Software", Provider: "seek", Region: "nz", Board: "6290"},
	}}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("board entries must validate against the registry: %v", err)
	}
	// Region is part of the board dedupe key, so the same slice in two markets is two crawl targets.
	if dups := DuplicateBoards(cfg.Sources); len(dups) != 0 {
		t.Errorf("same board id across markets must not collide: %v", dups)
	}
}

func TestSeekNewZealandMarketUsesItsOwnHostAndScope(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {seekPost("1", "Dev", "Co")}}}
	if _, err := NewSeek(fake).Fetch(context.Background(), CompanyEntry{Region: "nz", Board: "6287"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	pu, _ := url.Parse(fake.searchURLs[0])
	if pu.Host != "www.seek.co.nz" {
		t.Errorf("host = %q, want www.seek.co.nz", pu.Host)
	}
	if got := pu.Query().Get("siteKey"); got != "NZ-Main" {
		t.Errorf("siteKey = %q, want NZ-Main", got)
	}
	// where is load-bearing: omitting it does not mean "everywhere", it collapses the result set.
	if got := pu.Query().Get("where"); got != "All New Zealand" {
		t.Errorf("where = %q, want All New Zealand", got)
	}
}
