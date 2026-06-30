package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/enrich"
)

// getmatchHTTP is a route-aware test JSONGetter. getmatch has two endpoints: a paged list
// (/api/offers?offset=) and a per-offer detail (/api/offers/{id}). This fake routes detail
// requests by the id in the path and list requests by the offset query parameter, recording
// both so a test can assert pagination stops at meta.total and a detail is fetched per offer.
type getmatchHTTP struct {
	pages      map[int]string // list body keyed by requested offset
	details    map[int]string // detail body keyed by offer id
	failList   bool           // every list request fails
	failPage   map[int]bool   // a specific offset's list request fails
	detailErr  map[int]bool   // a specific offer's detail request fails
	gotOffsets []int
	gotDetails []int
}

var (
	getmatchDetailRE = regexp.MustCompile(`/api/offers/(\d+)`)
	getmatchOffsetRE = regexp.MustCompile(`offset=(\d+)`)
)

func (f *getmatchHTTP) GetJSON(_ context.Context, url string, v any) error {
	if m := getmatchDetailRE.FindStringSubmatch(url); m != nil {
		id, _ := strconv.Atoi(m[1])
		f.gotDetails = append(f.gotDetails, id)
		if f.detailErr[id] {
			return errors.New("getmatchHTTP: detail boom")
		}
		raw, ok := f.details[id]
		if !ok {
			return errors.New("getmatchHTTP: no detail for id")
		}
		return json.Unmarshal([]byte(raw), v)
	}
	offset := 0
	if m := getmatchOffsetRE.FindStringSubmatch(url); m != nil {
		offset, _ = strconv.Atoi(m[1])
	}
	f.gotOffsets = append(f.gotOffsets, offset)
	if f.failList || f.failPage[offset] {
		return errors.New("getmatchHTTP: list boom")
	}
	raw, ok := f.pages[offset]
	if !ok {
		raw = `{"meta":{"total":0,"offset":0,"limit":100},"offers":[]}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestGetmatchProvider(t *testing.T) {
	if got := NewGetmatch(nil).Provider(); got != "getmatch" {
		t.Errorf("Provider() = %q, want %q", got, "getmatch")
	}
}

func TestGetmatchFetchMapsOffer(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":1,"offset":0,"limit":100},"offers":[
			{"id":34895,"position":"Senior Golang Developer","url":"/vacancies/34895-senior-golang",
			 "published_at":"2026-06-19T12:55:17.948391","offer_description":"short summary",
			 "company":{"name":"HR Prime"},
			 "location_items":[{"label":"Москва","format":"office"},{"label":"Москва","format":"office"}]}
		]}`},
		details: map[int]string{34895: `{"id":34895,"description":"<h2>О компании</h2><p>Full body</p><script>x()</script>"}`},
	}

	jobs, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{Company: "getmatch", Provider: "getmatch"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "34895" {
		t.Errorf("ExternalID = %q, want 34895", j.ExternalID)
	}
	if want := "https://getmatch.ru/vacancies/34895-senior-golang"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Title != "Senior Golang Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	// The marketplace posting carries its OWN employer, not the configured placeholder.
	if j.Company != "HR Prime" {
		t.Errorf("Company = %q, want HR Prime (per-offer, not the entry's getmatch)", j.Company)
	}
	// Full HTML comes from the detail endpoint, sanitized (script stripped).
	if !strings.Contains(j.Description, "Full body") || strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not detail-sourced+sanitized: %q", j.Description)
	}
	// A single distinct work mode (office) → onsite; not remote.
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want onsite/false", j.WorkMode, j.Remote)
	}
	if want := "Москва"; j.Location != want {
		t.Errorf("Location = %q, want %q (distinct labels joined)", j.Location, want)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 19, 12, 55, 17, 948391000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-19T12:55:17.948391 (no-tz layout)", j.PostedAt)
	}
}

func TestGetmatchWorkMode(t *testing.T) {
	tests := []struct {
		name  string
		items []getmatchLocation
		want  string
	}{
		{"single remote", []getmatchLocation{{Format: "remote"}}, "remote"},
		{"all hybrid", []getmatchLocation{{Format: "hybrid"}, {Format: "hybrid"}}, "hybrid"},
		{"all office", []getmatchLocation{{Format: "office"}}, "onsite"},
		{"mixed remote+office", []getmatchLocation{{Format: "remote"}, {Format: "office"}}, ""},
		{"office plus relocation flags only", []getmatchLocation{
			{Format: "office"}, {Format: "relocation_company"}, {Format: "relocation_candidate"}}, "onsite"},
		{"relocation only", []getmatchLocation{{Format: "relocation_company"}}, ""},
		{"none", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getmatchWorkMode(tc.items); got != tc.want {
				t.Errorf("getmatchWorkMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetmatchRemoteFlagSetOnlyForRemote(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":1,"offset":0,"limit":100},"offers":[
			{"id":1,"position":"Remote Eng","url":"/vacancies/1-x","company":{"name":"Acme"},
			 "offer_description":"x","location_items":[{"label":"Россия","format":"remote"}]}
		]}`},
		details: map[int]string{1: `{"id":1,"description":"<p>body</p>"}`},
	}
	jobs, _ := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 || jobs[0].WorkMode != "remote" || !jobs[0].Remote {
		t.Fatalf("remote offer: WorkMode=%q Remote=%v, want remote/true", jobs[0].WorkMode, jobs[0].Remote)
	}
}

func TestGetmatchDescriptionFallback(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":2,"offset":0,"limit":100},"offers":[
			{"id":10,"position":"Empty detail","url":"/vacancies/10-x","company":{"name":"A"},
			 "offer_description":"<p>summary ten</p>","location_items":[{"label":"X","format":"remote"}]},
			{"id":11,"position":"Detail errors","url":"/vacancies/11-y","company":{"name":"B"},
			 "offer_description":"<p>summary eleven</p>","location_items":[{"label":"Y","format":"remote"}]}
		]}`},
		details:   map[int]string{10: `{"id":10,"description":""}`},
		detailErr: map[int]bool{11: true},
	}
	jobs, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// Empty detail description → fall back to the list offer_description.
	if !strings.Contains(byID["10"].Description, "summary ten") {
		t.Errorf("id 10 description = %q, want fallback to offer_description", byID["10"].Description)
	}
	// A failed detail request → also fall back, never drop the offer.
	if _, ok := byID["11"]; !ok {
		t.Fatal("id 11 dropped on detail error, want it kept via fallback")
	}
	if !strings.Contains(byID["11"].Description, "summary eleven") {
		t.Errorf("id 11 description = %q, want fallback to offer_description", byID["11"].Description)
	}
}

func TestGetmatchPaginatesAndStopsAtTotal(t *testing.T) {
	// total=150 with a 100 page size: offsets 0 and 100 are fetched, then 100+100>=150 stops.
	page := func(id int) string {
		return `{"meta":{"total":150,"offset":0,"limit":100},"offers":[
			{"id":` + strconv.Itoa(id) + `,"position":"P","url":"/vacancies/x","company":{"name":"C"},
			 "offer_description":"d","location_items":[{"label":"L","format":"remote"}]}]}`
	}
	fake := &getmatchHTTP{
		pages:   map[int]string{0: page(1), 100: page(2)},
		details: map[int]string{1: `{"description":"<p>a</p>"}`, 2: `{"description":"<p>b</p>"}`},
	}
	jobs, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (two pages)", len(jobs))
	}
	if !slices.Equal(fake.gotOffsets, []int{0, 100}) {
		t.Errorf("fetched offsets = %v, want [0 100] (stop at total)", fake.gotOffsets)
	}
}

func TestGetmatchFirstPageErrorFailsBoard(t *testing.T) {
	fake := &getmatchHTTP{failList: true}
	if _, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("Fetch: want first-page error, got nil")
	}
}

func TestGetmatchLaterPageErrorEndsEnumeration(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":150,"offset":0,"limit":100},"offers":[
			{"id":1,"position":"P","url":"/vacancies/x","company":{"name":"C"},
			 "offer_description":"d","location_items":[{"label":"L","format":"remote"}]}]}`},
		details:  map[int]string{1: `{"description":"<p>a</p>"}`},
		failPage: map[int]bool{100: true},
	}
	jobs, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: want no error on later-page failure, got %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (gathered before the failing page)", len(jobs))
	}
}

func TestGetmatchSeniority(t *testing.T) {
	tests := []struct {
		grade string
		want  string
	}{
		{"senior", "senior"},
		{"middle", "middle"},
		{"lead", "lead"},
		{"c_level", "c_level"},
		{"Senior", "senior"}, // case/space tolerant
		{"trainee", ""},      // not in freehire's vocabulary → dropped
		{"", ""},
	}
	for _, tc := range tests {
		if got := getmatchSeniority(tc.grade); got != tc.want {
			t.Errorf("getmatchSeniority(%q) = %q, want %q", tc.grade, got, tc.want)
		}
	}
}

func TestGetmatchCategory(t *testing.T) {
	tests := []struct {
		name  string
		specs []string
		want  string
	}{
		{"single mapped", []string{"python"}, "backend"},
		{"android to mobile", []string{"android"}, "mobile"},
		{"data engineering passthrough", []string{"data_engineering"}, "data_engineering"},
		{"duplicate same category", []string{"python", "golang"}, "backend"},
		{"conflict drops", []string{"python", "android"}, ""},
		{"unmappable drops", []string{"business_analyst"}, ""},
		{"unmappable mixed with mapped keeps mapped", []string{"business_analyst", "python"}, "backend"},
		{"none", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getmatchCategory(tc.specs); got != tc.want {
				t.Errorf("getmatchCategory(%v) = %q, want %q", tc.specs, got, tc.want)
			}
		})
	}
}

// Every category the specialization map targets must be a real CategoryValues member,
// so the map cannot drift out of the controlled vocabulary.
func TestGetmatchCategoryMapTargetsAreValid(t *testing.T) {
	for code, cat := range getmatchSpecializationCategory {
		if !slices.Contains(enrich.CategoryValues, cat) {
			t.Errorf("specialization %q maps to %q, not in enrich.CategoryValues", code, cat)
		}
	}
}

func TestGetmatchSkills(t *testing.T) {
	// Known technologies are canonicalized through skilltag; noise tokens are dropped.
	got := getmatchSkills([]getmatchSkill{{Name: "Golang"}, {Name: "Kiss"}, {Name: "Docker"}})
	if !slices.Equal(got, []string{"docker", "go"}) {
		t.Errorf("getmatchSkills = %v, want [docker go] (canonical, noise dropped)", got)
	}
	if got := getmatchSkills(nil); len(got) != 0 {
		t.Errorf("getmatchSkills(nil) = %v, want empty", got)
	}
}

func TestGetmatchFetchSetsStructuredFacets(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":1,"offset":0,"limit":100},"offers":[
			{"id":700,"position":"Разработчик","url":"/vacancies/700-x","company":{"name":"C"},
			 "offer_description":"s","location_items":[{"label":"Москва","format":"remote"}]}
		]}`},
		details: map[int]string{700: `{"id":700,"description":"<p>body</p>",
			"seniority":"senior","specializations":["python"],
			"skills_objects":[{"name":"Golang"},{"name":"Kiss"}],
			"required_years_of_experience":4}`},
	}
	jobs, err := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := jobs[0]
	if j.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior", j.Seniority)
	}
	if j.Category != "backend" {
		t.Errorf("Category = %q, want backend (python)", j.Category)
	}
	if !slices.Equal(j.Skills, []string{"go"}) {
		t.Errorf("Skills = %v, want [go] (Golang canonical, Kiss dropped)", j.Skills)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 4 {
		t.Errorf("ExperienceYearsMin = %v, want 4", j.ExperienceYearsMin)
	}
}

// A detail that fails (or omits the structured fields) leaves the facets empty/nil —
// the adapter never guesses.
func TestGetmatchFetchStructuredFacetsAbsentOnDetailError(t *testing.T) {
	fake := &getmatchHTTP{
		pages: map[int]string{0: `{"meta":{"total":1,"offset":0,"limit":100},"offers":[
			{"id":701,"position":"Backend Developer","url":"/vacancies/701-x","company":{"name":"C"},
			 "offer_description":"s","location_items":[{"label":"L","format":"remote"}]}
		]}`},
		detailErr: map[int]bool{701: true},
	}
	jobs, _ := NewGetmatch(fake).Fetch(context.Background(), CompanyEntry{})
	j := jobs[0]
	if j.Seniority != "" || j.Category != "" || len(j.Skills) != 0 || j.ExperienceYearsMin != nil {
		t.Errorf("structured facets should be empty on detail error, got sen=%q cat=%q skills=%v exp=%v",
			j.Seniority, j.Category, j.Skills, j.ExperienceYearsMin)
	}
}

func TestGetmatchRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["getmatch"]
	if !ok {
		t.Fatal(`All(nil)["getmatch"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("getmatch should be boardless (one global feed, no board id)")
	}
	if _, isAggregator := s.(aggregator); !isAggregator {
		t.Error("getmatch should be an aggregator (multi-company marketplace)")
	}
	if !slices.Contains(FilterableProviders(), "getmatch") {
		t.Error("FilterableProviders() should include the getmatch aggregator")
	}
}
