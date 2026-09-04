package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// emagineFake serves the POST search listing keyed by the request's skipCount and the per-job
// detail GET keyed by the id in the URL. It records the detail ids it served so a test can assert
// that only unseen postings are hydrated; the adapter fans detail fetches out across a worker
// pool, so that recorder is written concurrently and needs the lock.
type emagineFake struct {
	pages     map[int]string // skipCount -> search response JSON
	details   map[int]string // job id -> detail response JSON
	detailErr map[int]bool   // job id -> detail fetch fails

	mu        sync.Mutex
	detailIDs []int
}

func (f *emagineFake) PostJSON(_ context.Context, _ string, body, v any) error {
	req, ok := body.(emagineSearchRequest)
	if !ok {
		return fmt.Errorf("search body is %T, want emagineSearchRequest", body)
	}
	page, ok := f.pages[req.SkipCount]
	if !ok {
		return fmt.Errorf("no page at skipCount %d", req.SkipCount)
	}
	return json.Unmarshal([]byte(page), v)
}

func (f *emagineFake) GetJSON(_ context.Context, url string, v any) error {
	// .../api/JobAds/details/{id}/EN
	parts := strings.Split(strings.TrimSuffix(url, "/EN"), "/")
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return fmt.Errorf("detail url %q carries no id", url)
	}
	f.mu.Lock()
	f.detailIDs = append(f.detailIDs, id)
	f.mu.Unlock()
	if f.detailErr[id] {
		return errors.New("detail boom")
	}
	body, ok := f.details[id]
	if !ok {
		return fmt.Errorf("no detail for id %d", id)
	}
	return json.Unmarshal([]byte(body), v)
}

// emagineListing is one page holding three postings: a remote one whose location is
// country-only, a hybrid one with a city, and a part-time one whose location object is null.
const emagineListing = `{"items":[
{"id":178956,"title":"Senior AI Platform Engineer","isPartTime":false,
 "jobAdWorkLocation":{"workLocationType":"Remote","city":null,"region":null,"country":"Portugal"},
 "area":{"id":2140,"name":"Data & Analytics"}},
{"id":178953,"title":"Junior Business Analyst","isPartTime":false,
 "jobAdWorkLocation":{"workLocationType":"Hybrid","city":"Warsaw","region":"Mazovia","country":"Poland"},
 "area":{"id":2133,"name":"Project Delivery"}},
{"id":178950,"title":"Part-time Tester","isPartTime":true,
 "jobAdWorkLocation":null,"area":{"id":2141,"name":"Test & QA"}}
],"totalCount":3}`

func emagineFetch(t *testing.T, f *emagineFake) map[string]Job {
	t.Helper()
	jobs, err := emagine{list: f, detail: f}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	return byID
}

func TestEmagineFetchMapsListing(t *testing.T) {
	byID := emagineFetch(t, &emagineFake{pages: map[int]string{0: emagineListing}})

	if len(byID) != 3 {
		t.Fatalf("got %d jobs, want 3", len(byID))
	}
	got := byID["178953"]
	if got.Title != "Junior Business Analyst" {
		t.Errorf("Title = %q", got.Title)
	}
	if want := "https://portal.emagine.org/jobs/178953/junior-business-analyst"; got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
	if got.Company != "emagine" {
		t.Errorf("Company = %q, want emagine (the portal hides the end client)", got.Company)
	}
	if want := "Warsaw, Mazovia, Poland"; got.Location != want {
		t.Errorf("Location = %q, want %q", got.Location, want)
	}
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", got.WorkMode)
	}
}

// The portal is a freelance-contract marketplace, so employment type is a source fact rather
// than a guess: every posting is a contract unless it states part-time.
func TestEmagineEmploymentTypeIsContractUnlessPartTime(t *testing.T) {
	byID := emagineFetch(t, &emagineFake{pages: map[int]string{0: emagineListing}})

	if got := byID["178956"].EmploymentType; got != "contract" {
		t.Errorf("EmploymentType = %q, want contract", got)
	}
	if got := byID["178950"].EmploymentType; got != "part_time" {
		t.Errorf("part-time posting EmploymentType = %q, want part_time", got)
	}
}

// A country-only posting keeps just the country, and a null location object yields no location
// rather than a stray separator.
func TestEmagineLocationTolerates(t *testing.T) {
	byID := emagineFetch(t, &emagineFake{pages: map[int]string{0: emagineListing}})

	if got := byID["178956"].Location; got != "Portugal" {
		t.Errorf("country-only Location = %q, want Portugal", got)
	}
	if got := byID["178950"]; got.Location != "" || got.WorkMode != "" {
		t.Errorf("null location yielded Location=%q WorkMode=%q, want both empty", got.Location, got.WorkMode)
	}
}

// The API states no publication date — startDate is when the consultant is expected on the
// project, not when the ad went up — so PostedAt stays nil rather than carrying a wrong date.
func TestEmagineLeavesPostedAtUnset(t *testing.T) {
	for id, j := range emagineFetch(t, &emagineFake{pages: map[int]string{0: emagineListing}}) {
		if j.PostedAt != nil {
			t.Errorf("job %s PostedAt = %v, want nil", id, *j.PostedAt)
		}
	}
}

// The listing pages by skipCount until totalCount is reached. Sorting is by ascending id — a
// stable order under concurrent publishing, unlike a recency sort where a posting created
// mid-crawl shifts the window and pushes a posting past the page boundary unseen.
func TestEmaginePagesUntilTotalCount(t *testing.T) {
	first := `{"items":[{"id":1,"title":"One","jobAdWorkLocation":null}],"totalCount":2}`
	second := `{"items":[{"id":2,"title":"Two","jobAdWorkLocation":null}],"totalCount":2}`
	byID := emagineFetch(t, &emagineFake{pages: map[int]string{0: first, 1: second}})

	if len(byID) != 2 {
		t.Fatalf("got %d jobs, want both pages (2)", len(byID))
	}
}

// A page that comes back empty ends the crawl even if totalCount claims more, so a shrinking or
// lying total can never spin the pager forever.
func TestEmagineStopsOnEmptyPage(t *testing.T) {
	first := `{"items":[{"id":1,"title":"One","jobAdWorkLocation":null}],"totalCount":99}`
	empty := `{"items":[],"totalCount":99}`
	byID := emagineFetch(t, &emagineFake{pages: map[int]string{0: first, 1: empty}})

	if len(byID) != 1 {
		t.Fatalf("got %d jobs, want 1", len(byID))
	}
}

// The detail endpoint carries the description the listing omits plus emagine's own seniority,
// which maps onto freehire's vocabulary. It states the grade's DISPLAY name ("Mid level"), not
// the lookup id ("Medium") — verified against the live API.
func TestEmagineDetailMapsDescriptionAndSeniority(t *testing.T) {
	detail := `{"id":178950,"status":"Open","seniority":"Mid level",
"description":"<p>Test things.</p><script>alert(1)</script>"}`
	fake := &emagineFake{
		pages:   map[int]string{0: emagineListing},
		details: map[int]string{178950: detail},
	}
	jobs, err := emagine{list: fake, detail: fake}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var got Job
	for _, j := range jobs {
		if j.ExternalID == "178950" {
			got = j
		}
	}
	if !strings.Contains(got.Description, "Test things.") || strings.Contains(got.Description, "<script>") {
		t.Errorf("Description not sanitized/assembled: %q", got.Description)
	}
	if got.Seniority != "middle" {
		t.Errorf("Seniority = %q, want middle (emagine grades it \"Mid level\")", got.Seniority)
	}
}

// The grade arrives as the display name on the detail payload but as the lookup id in the
// portal's own filter vocabulary, and the two spellings differ for the middle grade. Both are
// accepted so a payload that switches to ids does not silently drop every seniority.
func TestEmagineSeniorityAcceptsIDAndDisplayName(t *testing.T) {
	for _, tc := range []struct{ grade, want string }{
		{"Mid level", "middle"},
		{"Medium", "middle"},
		{"Entry level", "junior"},
		{"Entry", "junior"},
		{"Senior", "senior"},
		{"", ""},
		{"Freelance", ""},
	} {
		if got := emagineSeniority(tc.grade); got != tc.want {
			t.Errorf("emagineSeniority(%q) = %q, want %q", tc.grade, got, tc.want)
		}
	}
}

// emagine's area is a broad practice, not a role: only the three that pin exactly one freehire
// category are mapped, and a broad one (Data & Analytics spans data engineering, data science
// and analytics) leaves the category empty so the title dictionary decides.
func TestEmagineAreaMapsOnlyUnambiguousCategories(t *testing.T) {
	for _, tc := range []struct{ area, want string }{
		{"Test & QA", "qa"},
		{"UX, UI & Visual Design", "design"},
		{"Project Delivery", "project_management"},
		{"Data & Analytics", ""},
		{"Software Development", ""},
		{"IT Infrastructure & Operations", ""},
		{"", ""},
	} {
		if got := emagineCategory(tc.area); got != tc.want {
			t.Errorf("emagineCategory(%q) = %q, want %q", tc.area, got, tc.want)
		}
	}
}

// A posting the catalogue already holds is refreshed for liveness only: no detail request, and
// SeenRefresh set so the pipeline does not overwrite its stored description with an empty one.
func TestEmagineFetchNewHydratesOnlyUnseen(t *testing.T) {
	fake := &emagineFake{
		pages: map[int]string{0: emagineListing},
		details: map[int]string{
			178956: `{"status":"Open","description":"<p>New one.</p>"}`,
			178953: `{"status":"Open","description":"<p>Also new.</p>"}`,
		},
	}
	seen := func(id string) bool { return id == "178950" }

	jobs, err := emagine{list: fake, detail: fake}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	if slices.Contains(fake.detailIDs, 178950) {
		t.Errorf("detail fetched for a seen posting: %v", fake.detailIDs)
	}
	for _, j := range jobs {
		if j.ExternalID == "178950" && !j.SeenRefresh {
			t.Error("seen posting should carry SeenRefresh so its stored content survives")
		}
	}
}

// A failed detail must not cost the posting: it is ingested list-only, and its description
// arrives on a later crawl.
func TestEmagineDetailFailureFallsBackToListOnly(t *testing.T) {
	fake := &emagineFake{
		pages:     map[int]string{0: emagineListing},
		details:   map[int]string{178953: `{"status":"Open","description":"<p>Fine.</p>"}`, 178950: `{"status":"Open"}`},
		detailErr: map[int]bool{178956: true},
	}
	jobs, err := emagine{list: fake, detail: fake}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want the failed-detail posting kept (3)", len(jobs))
	}
}

// The listing only carries open ads, but a posting can be taken down between the listing and its
// detail; the detail states the status, so a closed one is dropped rather than ingested dead.
func TestEmagineDropsPostingClosedBeforeDetail(t *testing.T) {
	fake := &emagineFake{
		pages: map[int]string{0: emagineListing},
		details: map[int]string{
			178956: `{"status":"Closed","description":"<p>Gone.</p>"}`,
			178953: `{"status":"Open","description":"<p>Live.</p>"}`,
			178950: `{"status":"Open","description":"<p>Live.</p>"}`,
		},
	}
	jobs, err := emagine{list: fake, detail: fake}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "178956" {
			t.Error("a posting closed before its detail was fetched should be dropped")
		}
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
}

// Every collection in the filter is [Required] server-side: a nil slice serializes as null and
// the request 400s, so the filter must be built with empty slices.
func TestEmagineSearchFilterSerializesEmptyArrays(t *testing.T) {
	body, err := json.Marshal(emagineSearchBody(0))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "null") {
		t.Errorf("search body carries a null the API rejects: %s", body)
	}
	for _, field := range []string{"textFilters", "workLocationTypes", "workLocations",
		"professionalRolesIds", "consultantSeniorities", "languageProficiencies",
		"industriesIds", "recordIdsToExclude", "supportedLanguageId"} {
		if !strings.Contains(string(body), `"`+field+`"`) {
			t.Errorf("search body omits required field %q: %s", field, body)
		}
	}
}

// The listing is three POSTs a crawl, but the detail fan-out is one GET per posting and the API
// starts refusing connections under sustained concurrency, so detail rides its OWN transport —
// the registry gives it an in-flight-bounded getter. A dropped description is permanent (the next
// crawl sees the posting as ingested and never re-fetches it), so the two must stay separable.
func TestEmagineFetchesDetailThroughItsOwnTransport(t *testing.T) {
	list := &emagineFake{pages: map[int]string{0: emagineListing}}
	detail := &emagineFake{details: map[int]string{
		178956: `{"status":"Open","description":"<p>One.</p>"}`,
		178953: `{"status":"Open","description":"<p>Two.</p>"}`,
		178950: `{"status":"Open","description":"<p>Three.</p>"}`,
	}}

	jobs, err := emagine{list: list, detail: detail}.FetchNew(
		context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(detail.detailIDs) != 3 {
		t.Errorf("detail transport served %d requests, want 3", len(detail.detailIDs))
	}
	if len(list.detailIDs) != 0 {
		t.Errorf("listing transport served %d detail requests, want 0", len(list.detailIDs))
	}
	for _, j := range jobs {
		if j.Description == "" {
			t.Errorf("job %s came back without a description", j.ExternalID)
		}
	}
}

func TestEmagineProvider(t *testing.T) {
	if got := NewEmagine(nil, nil).Provider(); got != "emagine" {
		t.Errorf("Provider() = %q, want emagine", got)
	}
}

// Boardless (one public API, no per-tenant board) but not an aggregator: every posting is
// contracted through emagine itself, so the source facet would duplicate the company filter.
func TestEmagineIsBoardlessNotAggregator(t *testing.T) {
	s := NewEmagine(nil, nil)
	if _, ok := s.(boardless); !ok {
		t.Error("emagine should implement the boardless marker")
	}
	if _, ok := s.(aggregator); ok {
		t.Error("emagine should NOT be an aggregator (single company)")
	}
	if slices.Contains(FilterableProviders(), "emagine") {
		t.Error("FilterableProviders() should exclude boardless single-company emagine")
	}
}

func TestEmagineIsHydratingSource(t *testing.T) {
	if _, ok := NewEmagine(nil, nil).(HydratingSource); !ok {
		t.Error("emagine should implement the HydratingSource marker")
	}
}

func TestEmagineRegistered(t *testing.T) {
	if _, ok := All(nil)["emagine"]; !ok {
		t.Error("All() should register provider emagine")
	}
}
