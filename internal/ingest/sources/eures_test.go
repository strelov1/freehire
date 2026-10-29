package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

// euresFake routes PostJSON calls (the paginated search) by the request's location code and
// Page, and GetJSON calls (per-posting detail) by the id suffix of the URL, so a single fake
// drives both stages. searchByPage answers every location a test does not name in searchByLoc.
type euresFake struct {
	searchByPage map[int]string            // page -> search response JSON ("" => empty page)
	searchByLoc  map[string]map[int]string // location code -> page -> search response JSON
	detailByID   map[string]string
	detailErr    map[string]bool
	gotBodies    []euresSearchRequest
	searchPages  []int
	detailCalls  []string
	mu           sync.Mutex
}

func (f *euresFake) PostJSON(_ context.Context, _ string, body, v any) error {
	req, ok := body.(euresSearchRequest)
	if !ok {
		return fmt.Errorf("euresFake.PostJSON: unexpected body type %T", body)
	}
	f.gotBodies = append(f.gotBodies, req)
	f.searchPages = append(f.searchPages, req.Page)
	pages := f.searchByPage
	if len(req.LocationCodes) == 1 {
		if byLoc, ok := f.searchByLoc[req.LocationCodes[0]]; ok {
			pages = byLoc
		}
	}
	b := pages[req.Page]
	if b == "" {
		b = `{"numberRecords":0,"jvs":[]}`
	}
	return json.Unmarshal([]byte(b), v)
}

func (f *euresFake) GetJSON(_ context.Context, url string, v any) error {
	id := strings.TrimPrefix(url, euresDetailURL)
	if i := strings.Index(id, "?"); i >= 0 {
		id = id[:i]
	}
	f.mu.Lock() // fetchDetails calls GetJSON from several workers at once
	f.detailCalls = append(f.detailCalls, id)
	f.mu.Unlock()
	if f.detailErr[id] {
		return errors.New("detail boom")
	}
	b := f.detailByID[id]
	if b == "" {
		b = `{}`
	}
	return json.Unmarshal([]byte(b), v)
}

// euresFullPage builds a search response of n postings, all with distinct ids offset by base,
// reporting numberRecords as total.
func euresFullPage(n, base, total int) string {
	return euresFacetedPage(n, base, total, "")
}

// euresFacetedPage is euresFullPage carrying a POSITION_LOCATION facet for country, whose
// children are the given sub-region codes — the shape the API answers a country query with.
func euresFacetedPage(n, base, total int, country string, children ...string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":"ID-%d","title":"T","employer":{"name":"Co"},"description":"<p>d</p>"}`, base+i)
	}
	facets := ""
	if country != "" {
		kids := make([]string, len(children))
		for i, c := range children {
			kids[i] = fmt.Sprintf(`{"code":%q,"count":1,"childrenList":[]}`, c)
		}
		facets = fmt.Sprintf(`,"facets":{"POSITION_LOCATION":{"facetEntriesList":[{"code":%q,"count":%d,"childrenList":[%s]}]}}`,
			country, total, strings.Join(kids, ","))
	}
	return fmt.Sprintf(`{"numberRecords":%d,"jvs":[%s]%s}`, total, strings.Join(items, ","), facets)
}

// newTestEures builds the adapter with a small depth cap, so the partitioning path can be
// driven without fabricating ten thousand postings.
func newTestEures(f *euresFake, depthCap int) eures {
	return eures{http: f, depthCap: depthCap}
}

func TestEuresSearchRequestShape(t *testing.T) {
	fake := &euresFake{}
	if _, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.gotBodies) != 1 {
		t.Fatalf("got %d search requests, want 1", len(fake.gotBodies))
	}
	req := fake.gotBodies[0]
	if !slices.Equal(req.LocationCodes, []string{"de"}) {
		t.Errorf("LocationCodes = %v, want [de]", req.LocationCodes)
	}
	if !slices.Equal(req.OccupationUris, euresOccupationURIs) {
		t.Errorf("OccupationUris = %v, want %v", req.OccupationUris, euresOccupationURIs)
	}
	if req.SortSearch != "MOST_RECENT" {
		t.Errorf("SortSearch = %q, want MOST_RECENT", req.SortSearch)
	}
	if req.ResultsPerPage != euresPageSize {
		t.Errorf("ResultsPerPage = %d, want %d", req.ResultsPerPage, euresPageSize)
	}
	if req.Page != 1 {
		t.Errorf("Page = %d, want 1", req.Page)
	}
	// The request must not narrow the listing to a publication window: a window hides every
	// posting older than it from every crawl, so none of them can ever be seen again.
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "publicationPeriod") {
		t.Errorf("request carries a publication window: %s", raw)
	}
}

func TestEuresPaginates(t *testing.T) {
	full := euresFullPage(euresPageSize, 0, euresPageSize+3)
	short := euresFullPage(3, 1000, euresPageSize+3)
	fake := &euresFake{searchByPage: map[int]string{1: full, 2: short}}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.searchPages, []int{1, 2}) {
		t.Errorf("requested pages = %v, want [1 2]", fake.searchPages)
	}
	if len(jobs) != euresPageSize+3 {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), euresPageSize+3)
	}
}

// A walk that reads fewer postings than the search states it holds did not list the board, and
// must fail rather than hand the sweep a partial board to close against.
func TestEuresFailsAShortWalk(t *testing.T) {
	fake := &euresFake{searchByPage: map[int]string{
		1: euresFullPage(euresPageSize, 0, 120),
		2: euresFullPage(10, 100, 120),
	}}
	if _, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"}); err == nil {
		t.Fatal("Fetch read 60 of 120 stated postings and reported success")
	}
}

// A posting withdrawn mid-walk lowers the total later pages state; the walk is complete
// against that lower total and must not fail the board over it.
func TestEuresToleratesAPostingWithdrawnMidWalk(t *testing.T) {
	fake := &euresFake{searchByPage: map[int]string{
		1: euresFullPage(euresPageSize, 0, euresPageSize+3),
		2: euresFullPage(2, 1000, euresPageSize+2),
	}}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != euresPageSize+2 {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), euresPageSize+2)
	}
}

// A later page stating no results at all is a broken answer, not a withdrawal: taking its zero
// as the bar would pass a walk that read a fraction of the board.
func TestEuresFailsWhenALaterPageStatesNoResults(t *testing.T) {
	fake := &euresFake{searchByPage: map[int]string{
		1: euresFullPage(euresPageSize, 0, 120),
		2: `{"numberRecords":0,"jvs":[]}`,
	}}
	if _, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"}); err == nil {
		t.Fatal("Fetch read 50 of 120 stated postings and reported success on a zero-total page")
	}
}

// The search counts a posting it indexes twice twice (measured live 2026-10-28: region "dea"
// stated 4,731 results and served 4,730 distinct ids, on every walk). The walk read every
// position the search stated, so it is complete; the duplicate is yielded once.
func TestEuresAcceptsAPostingTheSearchListsTwice(t *testing.T) {
	const page = `{"numberRecords":3,"jvs":[
	  {"id":"ID-0","title":"T","employer":{"name":"Co"}},
	  {"id":"ID-0","title":"T","employer":{"name":"Co"}},
	  {"id":"ID-1","title":"T","employer":{"name":"Co"}}]}`
	fake := &euresFake{searchByPage: map[int]string{1: page}}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}
}

// The same holds for a country listed by region: a posting a region lists twice counts toward
// the country's total once per listing.
func TestEuresPartitionsCountAPostingTheSearchListsTwice(t *testing.T) {
	country := euresFacetedPage(1, 0, 4, "de", "de1", "de2")
	const region = `{"numberRecords":3,"jvs":[
	  {"id":"ID-0","title":"T","employer":{"name":"Co"}},
	  {"id":"ID-0","title":"T","employer":{"name":"Co"}},
	  {"id":"ID-1","title":"T","employer":{"name":"Co"}}]}`
	fake := &euresFake{searchByLoc: map[string]map[int]string{
		"de":  {1: country},
		"de1": {1: region},
		"de2": {1: euresFullPage(1, 2, 1)},
	}}
	jobs, err := newTestEures(fake, 3).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("len(jobs) = %d, want 3", len(jobs))
	}
}

// A country holding more postings than the API will page through is listed region by region —
// the sub-regions its own location facet names, "not specified" included — and the union,
// deduplicated (a posting may name several regions), must cover the country's total.
func TestEuresPartitionsACountryPastTheDepthCap(t *testing.T) {
	country := euresFacetedPage(1, 0, 5, "de", "de1", "de-NS")
	fake := &euresFake{searchByLoc: map[string]map[int]string{
		"de":    {1: country},
		"de1":   {1: euresFullPage(3, 0, 3)}, // ID-0..ID-2
		"de-NS": {1: euresFullPage(3, 2, 3)}, // ID-2..ID-4, ID-2 named in both
	}}
	jobs, err := newTestEures(fake, 4).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	slices.Sort(ids)
	if want := []string{"ID-0", "ID-1", "ID-2", "ID-3", "ID-4"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestEuresFailsWhenRegionsDoNotCoverTheCountry(t *testing.T) {
	country := euresFacetedPage(1, 0, 6, "de", "de1", "de-NS")
	fake := &euresFake{searchByLoc: map[string]map[int]string{
		"de":    {1: country},
		"de1":   {1: euresFullPage(3, 0, 3)},
		"de-NS": {1: euresFullPage(2, 3, 2)},
	}}
	if _, err := newTestEures(fake, 4).Fetch(context.Background(), CompanyEntry{Board: "de"}); err == nil {
		t.Fatal("Fetch listed 5 of the country's 6 postings and reported success")
	}
}

func TestEuresFailsWhenARegionIsPastTheDepthCap(t *testing.T) {
	country := euresFacetedPage(1, 0, 5, "de", "de1")
	fake := &euresFake{searchByLoc: map[string]map[int]string{
		"de":  {1: country},
		"de1": {1: euresFullPage(1, 0, 5)},
	}}
	if _, err := newTestEures(fake, 4).Fetch(context.Background(), CompanyEntry{Board: "de"}); err == nil {
		t.Fatal("Fetch accepted a region the API cannot page to its end")
	}
}

func TestEuresFailsPastTheDepthCapWithNoRegions(t *testing.T) {
	fake := &euresFake{searchByLoc: map[string]map[int]string{
		"de": {1: euresFullPage(1, 0, 5)},
	}}
	if _, err := newTestEures(fake, 4).Fetch(context.Background(), CompanyEntry{Board: "de"}); err == nil {
		t.Fatal("Fetch accepted a country it cannot page to its end and cannot split")
	}
}

// A posting the catalogue already holds is re-listed as a SeenRefresh with no detail request:
// the detail is read only for its location, which a stored posting already has.
func TestEuresFetchNewSkipsTheDetailOfASeenPosting(t *testing.T) {
	fake := &euresFake{searchByPage: map[int]string{1: euresFullPage(2, 0, 2)}}
	jobs, err := NewEures(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "de"},
		func(id string) bool { return id == "ID-0" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if seen := byID["ID-0"]; !seen.SeenRefresh || seen.Title != "T" || seen.Company != "Co" || seen.URL == "" {
		t.Errorf("seen posting = %+v, want a SeenRefresh carrying title, company and URL", seen)
	}
	if byID["ID-1"].SeenRefresh {
		t.Error("an unseen posting must be hydrated, not refreshed")
	}
	if !slices.Equal(fake.detailCalls, []string{"ID-1"}) {
		t.Errorf("detail requests = %v, want only the unseen posting's", fake.detailCalls)
	}
}

func TestEuresMapsFieldsHappyPath(t *testing.T) {
	const page1 = `{"numberRecords":1,"jvs":[{
	  "id":"MTAwMDEtMTAwMTEzOTMxMS1TIDE",
	  "title":"Software Engineer",
	  "description":"<p>We build things.</p>",
	  "employer":{"name":"SUSS MicroTec Solutions GmbH & Co. KG"},
	  "creationDate":1739403609768,
	  "locationMap":{"DE":["DE12B"],"EL":["EL30"]},
	  "positionOfferingCode":"directhire",
	  "positionScheduleCodes":["fulltime"]
	}]}`
	const detail = `{"preferredLanguage":"de","jvProfiles":{"de":{"locations":[
	  {"countryCode":"de","cityName":"Karlsruhe, Baden","region":"de122"}
	]}}}`
	fake := &euresFake{
		searchByPage: map[int]string{1: page1},
		detailByID:   map[string]string{"MTAwMDEtMTAwMTEzOTMxMS1TIDE": detail},
	}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "MTAwMDEtMTAwMTEzOTMxMS1TIDE" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://europa.eu/eures/portal/jv-se/jv-details/MTAwMDEtMTAwMTEzOTMxMS1TIDE?lang=en" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Software Engineer" || j.Company != "SUSS MicroTec Solutions GmbH & Co. KG" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Description != "<p>We build things.</p>" {
		t.Errorf("Description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2025-02-12" {
		t.Errorf("PostedAt = %v, want 2025-02-12", j.PostedAt)
	}
	if !j.IsTechHint {
		t.Error("IsTechHint = false, want true (adapter is ICT-occupation-scoped)")
	}
	if !slices.Equal(dedupSorted(j.Countries), []string{"de", "gr"}) {
		t.Errorf("Countries = %v, want [de gr] (EL normalizes to gr)", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (from positionScheduleCodes)", j.EmploymentType)
	}
	if j.Location != "Karlsruhe, Baden, Germany" {
		t.Errorf("Location = %q, want city + country from the detail fetch", j.Location)
	}
}

func TestEuresNoMatchingPostingsYieldsEmptyNotError(t *testing.T) {
	fake := &euresFake{} // no searchByPage entries => every page answers zero records
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "mt"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0 for a board with no matching postings", len(jobs))
	}
}

func TestEuresLocationFallsBackWhenDetailFails(t *testing.T) {
	const page1 = `{"numberRecords":1,"jvs":[{"id":"X1","title":"T","employer":{"name":"Co"}}]}`
	fake := &euresFake{
		searchByPage: map[int]string{1: page1},
		detailErr:    map[string]bool{"X1": true},
	}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "fr"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("a failed detail fetch must not drop the posting; len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].Location != "France" {
		t.Errorf("Location = %q, want the board's country name as a fallback", jobs[0].Location)
	}
}

func TestEuresEmploymentTypeMapping(t *testing.T) {
	cases := []struct {
		name     string
		offering string
		schedule []string
		want     string
	}{
		{"internship offering wins", "internship", []string{"fulltime"}, "internship"},
		{"contract offering wins", "contract", []string{"fulltime"}, "contract"},
		{"fulltime schedule maps", "directhire", []string{"fulltime"}, "full_time"},
		{"parttime schedule maps", "directhire", []string{"parttime"}, "part_time"},
		{"neither maps", "directhire", []string{"flextime"}, ""},
		{"nothing stated", "", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := euresEmploymentType(c.offering, c.schedule); got != c.want {
				t.Errorf("euresEmploymentType(%q, %v) = %q, want %q", c.offering, c.schedule, got, c.want)
			}
		})
	}
}

func TestEuresMarkers(t *testing.T) {
	src := NewEures(nil)
	if src.Provider() != "eures" {
		t.Errorf("Provider() = %q, want eures", src.Provider())
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("eures must implement the aggregator marker (re-lists national PES/partner feeds)")
	}
	if _, ok := src.(boardless); ok {
		t.Error("eures must NOT be boardless — board is the country code")
	}
	if _, ok := src.(fullBoardListing); !ok {
		t.Error("eures must implement fullBoardListing — every walk is verified against the search's own total")
	}
	if _, ok := src.(fullCatalog); ok {
		t.Error("eures must NOT be fullCatalog — one board is one country, not the whole EURES catalogue")
	}
	if _, ok := All(nil)["eures"]; !ok {
		t.Error("All() should register provider eures")
	}
	if !slices.Contains(FilterableProviders(), "eures") {
		t.Error("FilterableProviders() should include eures")
	}
	reg := All(nil)
	if !slices.Contains(AggregatorProviders(reg), "eures") {
		t.Error("AggregatorProviders() should include eures")
	}
	if !slices.Contains(BoardKeyedProviders(reg), "eures") {
		t.Error("BoardKeyedProviders() should include eures (board = country)")
	}
	if slices.Contains(FullCatalogProviders(reg), "eures") {
		t.Error("FullCatalogProviders() should NOT include eures (one board is one country)")
	}
}

// dedupSorted returns a sorted copy of ss for order-independent slice comparison.
func dedupSorted(ss []string) []string {
	out := slices.Clone(ss)
	slices.Sort(out)
	return out
}
