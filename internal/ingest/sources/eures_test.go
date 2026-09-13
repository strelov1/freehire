package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// euresFake routes PostJSON calls (the paginated search) by the request's own Page field and
// GetJSON calls (per-posting detail) by the id suffix of the URL, so a single fake drives both
// stages.
type euresFake struct {
	searchByPage map[int]string // page -> search response JSON ("" => empty page)
	detailByID   map[string]string
	detailErr    map[string]bool
	gotBodies    []euresSearchRequest
	searchPages  []int
}

func (f *euresFake) PostJSON(_ context.Context, _ string, body, v any) error {
	req, ok := body.(euresSearchRequest)
	if !ok {
		return fmt.Errorf("euresFake.PostJSON: unexpected body type %T", body)
	}
	f.gotBodies = append(f.gotBodies, req)
	f.searchPages = append(f.searchPages, req.Page)
	b := f.searchByPage[req.Page]
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
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":"ID-%d","title":"T","employer":{"name":"Co"},"description":"<p>d</p>"}`, base+i)
	}
	return fmt.Sprintf(`{"numberRecords":%d,"jvs":[%s]}`, total, strings.Join(items, ","))
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
	if req.PublicationPeriod != "LAST_THREE_DAYS" {
		t.Errorf("PublicationPeriod = %q, want LAST_THREE_DAYS", req.PublicationPeriod)
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

func TestEuresPaginationDepthCapBackstop(t *testing.T) {
	fake := &euresFake{searchByPage: map[int]string{}}
	// Every page returns a full page and reports an enormous total, so only the depth-cap
	// backstop can end the loop.
	huge := euresFullPage(euresPageSize, 0, 100_000_000)
	for p := 1; p <= euresMaxPages+5; p++ {
		fake.searchByPage[p] = huge
	}
	jobs, err := NewEures(fake).Fetch(context.Background(), CompanyEntry{Board: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.searchPages) != euresMaxPages {
		t.Fatalf("requested %d pages, want exactly the depth-cap backstop of %d", len(fake.searchPages), euresMaxPages)
	}
	if want := euresMaxPages * euresPageSize; len(jobs) != want {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), want)
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
