package sources

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestJobStreetIsRegisteredAsItsOwnAggregatorProvider(t *testing.T) {
	src := NewJobStreet(&seekFake{}, newSeekDetailNetwork(&seekFake{}, nil))
	if src.Provider() != "jobstreet" {
		t.Fatalf("Provider() = %q, want jobstreet", src.Provider())
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Error("jobstreet must share SEEK's HydratingSource behaviour")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("jobstreet must be an aggregator so first-party ATS copies win dedup")
	}
	if _, ok := src.(boardless); ok {
		t.Error("jobstreet must not be boardless: the ICT subclassification is the crawl slice")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "jobstreet") {
		t.Error("jobstreet missing from AggregatorProviders")
	}
	// Its listing reaches a natural end and states an exact total, so it closes per board on the
	// default window — a declared grace (even one equal to the default) would bar the board scope.
	if got, ok := SweepGraceWindows(Taxonomy())["jobstreet"]; ok {
		t.Errorf("jobstreet declares a sweep grace of %v; it must sweep on the default window", got)
	}
	if !FullBoardListingProviders(Taxonomy())["jobstreet"] {
		t.Error("jobstreet must carry fullBoardListing")
	}
	if FullBoardListingProviders(Taxonomy())["seek"] {
		t.Error("seek must not carry fullBoardListing: its result window hides each busy slice's tail")
	}
}

// A JobStreet board id repeats in every market and external_id carries no market, so the
// board-scoped close narrows each board to its market by the host its postings live on. The
// prefix must be exactly the one toJob builds the posting URL from, and distinct per market.
func TestJobStreetScopesABoardToItsMarketByURL(t *testing.T) {
	src := NewJobStreet(&seekFake{}, newSeekDetailNetwork(&seekFake{}, nil))
	if !ScopesBoardsByRegion(src) {
		t.Fatal("jobstreet must scope its boards by region")
	}
	prefixes := map[string]string{}
	for region, m := range jobStreetMarkets {
		prefix, ok := BoardURLPrefix(src, region)
		if !ok {
			t.Fatalf("no URL prefix for market %q", region)
		}
		job, ok := seekPost("123", "Engineer", "Co").toJob(m)
		if !ok || !strings.HasPrefix(job.URL, prefix) {
			t.Errorf("market %q: posting URL %q does not start with the prefix %q", region, job.URL, prefix)
		}
		if other, dup := prefixes[prefix]; dup {
			t.Errorf("markets %q and %q share the prefix %q", other, region, prefix)
		}
		prefixes[prefix] = region
	}
	if _, ok := BoardURLPrefix(src, "au"); ok {
		t.Error("a market jobstreet does not serve must have no prefix")
	}
}

// fullBoardListing's bar for JobStreet: a walk that does not read the whole board fails the
// crawl instead of handing the board scope a partial listing it would read as removals.
func TestJobStreetFailsAWalkThatDidNotReadTheWholeBoard(t *testing.T) {
	stated := 5
	cases := map[string]*seekFake{
		"a later page errors": {
			searchByPage: map[int][]seekPosting{1: {seekPost("1", "A", "Co")}, 2: {seekPost("2", "B", "Co")}},
			searchErr:    map[int]bool{2: true},
		},
		"the walk ends short of the stated total": {
			searchByPage: map[int][]seekPosting{1: {seekPost("1", "A", "Co"), seekPost("2", "B", "Co")}},
			total:        &stated,
		},
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			jobs, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).Fetch(context.Background(), CompanyEntry{Region: "sg", Board: "6290"})
			if err == nil {
				t.Errorf("Fetch = %d jobs and no error, want a failed crawl", len(jobs))
			}
		})
	}
}

// A walk that reads exactly what the search states is complete, including an emptied board.
func TestJobStreetAcceptsAWalkThatMatchesTheStatedTotal(t *testing.T) {
	cases := map[string]map[int][]seekPosting{
		"two pages": {1: {seekPost("1", "A", "Co"), seekPost("2", "B", "Co")}, 2: {seekPost("3", "C", "Co")}},
		"empty":     {},
	}
	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &seekFake{searchByPage: pages}
			jobs, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).Fetch(context.Background(), CompanyEntry{Region: "sg", Board: "6290"})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if want := fake.statedTotal(); len(jobs) != want {
				t.Errorf("len(jobs) = %d, want %d", len(jobs), want)
			}
		})
	}
}

func TestJobStreetMarketsUseTheirOwnHostSiteKeyAndWhere(t *testing.T) {
	cases := []struct {
		region, host, siteKey, where string
	}{
		{"sg", "sg.jobstreet.com", "SG-Main", "Singapore"},
		{"my", "my.jobstreet.com", "MY-Main", "Malaysia"},
		{"id", "id.jobstreet.com", "ID-Main", "Indonesia"},
		{"ph", "ph.jobstreet.com", "PH-Main", "Philippines"},
		{"hk", "hk.jobsdb.com", "HK-Main", "Hong Kong"},
		{"th", "th.jobsdb.com", "TH-Main", "Thailand"},
	}
	if len(cases) != len(jobStreetMarkets) {
		t.Fatalf("market test cases = %d, registered markets = %d", len(cases), len(jobStreetMarkets))
	}
	for _, tc := range cases {
		t.Run(tc.region, func(t *testing.T) {
			fake := &seekFake{searchByPage: map[int][]seekPosting{1: {seekPost("1", "Dev", "Co")}}}
			if _, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).Fetch(context.Background(), CompanyEntry{Region: tc.region, Board: "6290"}); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			pu, err := url.Parse(fake.searchURLs[0])
			if err != nil {
				t.Fatalf("parse search URL: %v", err)
			}
			if pu.Host != tc.host {
				t.Errorf("host = %q, want %q", pu.Host, tc.host)
			}
			if got := pu.Query().Get("siteKey"); got != tc.siteKey {
				t.Errorf("siteKey = %q, want %q", got, tc.siteKey)
			}
			if got := pu.Query().Get("where"); got != tc.where {
				t.Errorf("where = %q, want %q", got, tc.where)
			}
			if got := pu.Query().Get("subclassification"); got != "6290" {
				t.Errorf("subclassification = %q, want 6290", got)
			}
		})
	}
}

func TestJobStreetSingaporeMapsHumanAndDetailURLsToJobStreet(t *testing.T) {
	p := seekPost("94590000", "Backend Engineer", "Example Pte Ltd")
	p.Locations = []seekLocation{{Label: "Singapore", CountryCode: "SG"}}
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{1: {p}},
		detailByID:   map[string]string{"94590000": "<p>Build APIs</p>"},
	}
	jobs, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Region: "sg", Board: "6287"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].URL != "https://sg.jobstreet.com/job/94590000" {
		t.Errorf("URL = %q", jobs[0].URL)
	}
	if fake.detailURL != "https://sg.jobstreet.com/graphql" {
		t.Errorf("detail endpoint = %q", fake.detailURL)
	}
	if !slices.Equal(jobs[0].Countries, []string{"sg"}) {
		t.Errorf("Countries = %v, want [sg]", jobs[0].Countries)
	}
}

func TestJobStreetPhilippinesAndThailandMapHumanAndDetailURLs(t *testing.T) {
	cases := []struct {
		region, host, country string
	}{
		{"ph", "ph.jobstreet.com", "ph"},
		{"th", "th.jobsdb.com", "th"},
	}
	for _, tc := range cases {
		t.Run(tc.region, func(t *testing.T) {
			p := seekPost("94590001", "Backend Engineer", "Example")
			p.Locations = []seekLocation{{Label: "Market", CountryCode: strings.ToUpper(tc.country)}}
			fake := &seekFake{
				searchByPage: map[int][]seekPosting{1: {p}},
				detailByID:   map[string]string{"94590001": "<p>Build APIs</p>"},
			}
			jobs, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).(HydratingSource).FetchNew(context.Background(),
				CompanyEntry{Region: tc.region, Board: "6287"}, func(string) bool { return false })
			if err != nil {
				t.Fatalf("FetchNew: %v", err)
			}
			if len(jobs) != 1 {
				t.Fatalf("len(jobs) = %d, want 1", len(jobs))
			}
			wantURL := "https://" + tc.host + "/job/94590001"
			if jobs[0].URL != wantURL {
				t.Errorf("URL = %q, want %q", jobs[0].URL, wantURL)
			}
			wantDetail := "https://" + tc.host + "/graphql"
			if fake.detailURL != wantDetail {
				t.Errorf("detail endpoint = %q, want %q", fake.detailURL, wantDetail)
			}
			if !slices.Equal(jobs[0].Countries, []string{tc.country}) {
				t.Errorf("Countries = %v, want [%s]", jobs[0].Countries, tc.country)
			}
		})
	}
}

func TestJobStreetUnknownMarketFailsWithoutRequest(t *testing.T) {
	fake := &seekFake{}
	_, err := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).Fetch(context.Background(), CompanyEntry{Company: "JobStreet Nowhere", Region: "xx", Board: "6287"})
	if err == nil || !strings.Contains(err.Error(), "jobstreet") || !strings.Contains(err.Error(), "xx") {
		t.Fatalf("unknown market error = %v, want provider and region", err)
	}
	if len(fake.searchURLs) != 0 {
		t.Errorf("unknown market issued requests: %v", fake.searchURLs)
	}
}

func TestJobStreetPageCeilingFailsInsteadOfSilentlyTruncating(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{
		1: {seekPost("1", "A", "Co")},
		2: {seekPost("2", "B", "Co")},
		3: {seekPost("3", "Would prove there is more", "Co")},
	}}
	src := NewJobStreet(fake, newSeekDetailNetwork(fake, nil)).(jobStreet)
	src.maxPages = 2 // shrink the production safety ceiling to make the invariant cheap to test
	jobs, err := src.Fetch(context.Background(), CompanyEntry{Region: "sg", Board: "6290"})
	if err == nil {
		t.Fatalf("capped JobStreet walk returned %d jobs without an error; silent truncation must fail", len(jobs))
	}
	if !strings.Contains(err.Error(), "exceeded 2 pages") {
		t.Errorf("error = %q, want page-ceiling diagnosis", err)
	}
}

func TestJobStreetFetchNewGatedSkipsCoveredEmployerDetail(t *testing.T) {
	fake := &seekFake{
		searchByPage: map[int][]seekPosting{
			1: {seekPost("1", "Covered role", "Covered Co"), seekPost("2", "New role", "New Co")},
			2: {},
		},
		detailByID: map[string]string{"1": "covered body", "2": "new body"},
	}
	src := NewJobStreet(fake, newSeekDetailNetwork(fake, nil))
	gated, ok := src.(CoverageGated)
	if !ok {
		t.Fatal("jobstreet should implement CoverageGated")
	}
	jobs, err := gated.FetchNewGated(context.Background(), CompanyEntry{Provider: "jobstreet", Board: "6287", Region: "ph"},
		func(string) bool { return false },
		DetailGate{Covered: func(companies []string) map[string]bool {
			if !slices.Contains(companies, "Covered Co") || !slices.Contains(companies, "New Co") {
				t.Fatalf("coverage companies = %v", companies)
			}
			return map[string]bool{"Covered Co": true}
		}})
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	fake.mu.Lock()
	hits := append([]string(nil), fake.detailHits...)
	fake.mu.Unlock()
	if slices.Contains(hits, "1") || !slices.Contains(hits, "2") {
		t.Fatalf("detail hits = %v, want only uncovered posting 2", hits)
	}
	for _, j := range jobs {
		if j.ExternalID == "1" && j.Description != "" {
			t.Errorf("covered posting description = %q, want empty list-only result", j.Description)
		}
		if j.ExternalID == "2" && !strings.Contains(j.Description, "new body") {
			t.Errorf("uncovered posting description = %q, want hydrated body", j.Description)
		}
	}
}

// A count one off is what a posting listed mid-walk looks like, so the board is walked once more
// before the crawl fails; the second walk agreeing is a complete listing.
func TestJobStreetRewalksABoardWhoseCountMovedMidWalk(t *testing.T) {
	fake := &shiftedTotalFake{seekFake: &seekFake{searchByPage: map[int][]seekPosting{
		1: {seekPost("1", "A", "Co"), seekPost("2", "B", "Co")},
	}}}
	jobs, err := NewJobStreet(fake, newSeekDetailNetwork(fake.seekFake, nil)).Fetch(context.Background(), CompanyEntry{Region: "sg", Board: "6290"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || fake.walks != 2 {
		t.Errorf("got %d jobs over %d walks, want 2 jobs over 2 walks", len(jobs), fake.walks)
	}
}
