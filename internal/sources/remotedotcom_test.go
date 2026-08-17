package sources

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// remotedotcomHTTP is a route-aware fake for the one transport remote.com needs. Both request
// shapes are GetTextWithHeaders — a listing page keyed by its ?page=N, a posting page by its
// slug — so the fake routes on the URL and records what it was asked for, letting a test assert
// both the walk's stop condition and that a hydrating run skips seen postings.
type remotedotcomHTTP struct {
	mu sync.Mutex

	pages    map[int]string    // listing flight keyed by page number
	postings map[string]string // posting-page flight keyed by posting slug

	pageErr    map[int]bool
	postingErr map[string]bool

	gotPages    []int
	gotPostings []string
	gotHeaders  map[string]string
}

var remotedotcomPageRE = regexp.MustCompile(`[?&]page=(\d+)`)

func (f *remotedotcomHTTP) GetTextWithHeaders(_ context.Context, url string, headers map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotHeaders = headers

	if m := remotedotcomPageRE.FindStringSubmatch(url); m != nil {
		page, _ := strconv.Atoi(m[1])
		f.gotPages = append(f.gotPages, page)
		if f.pageErr[page] {
			return "", errors.New("remotedotcomHTTP: page boom")
		}
		return f.pages[page], nil
	}

	slug := url[strings.LastIndex(url, "/")+1:]
	f.gotPostings = append(f.gotPostings, slug)
	if f.postingErr[slug] {
		return "", errors.New("remotedotcomHTTP: posting boom")
	}
	return f.postings[slug], nil
}

// remotedotcomListFlight wraps a jobsData object in a plausible listing flight — surrounded by
// other RSC content, so the brace scan has to isolate it rather than decode the whole stream.
func remotedotcomListFlight(jobsData string) string {
	return `2:["$","div",null,{"section":["$","$Le",null,{}],"children":[["$","$L16",null,{"jobsData":` +
		jobsData + `}],["$","footer",null,{}]]}]`
}

// remotedotcomDetailFlight wraps a schema.org JobPosting in the RSC TEXT ROW shape a posting
// page answers with: "<id>:T<hexlen>,<bytes>", the length in lowercase hex. A decoy row and a
// leading row keep the test honest about both the row scan and the JobPosting filter.
func remotedotcomDetailFlight(posting string) string {
	row := func(id, body string) string {
		return "\n" + id + ":T" + strconv.FormatInt(int64(len(body)), 16) + "," + body
	}
	return `0:["$","html",null,{}]` +
		row("a", `{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[]}`) +
		row("13", posting)
}

func TestRemotedotcomProvider(t *testing.T) {
	if got := NewRemotedotcom(nil).Provider(); got != "remotedotcom" {
		t.Errorf("Provider() = %q, want remotedotcom", got)
	}
}

func TestRemotedotcomIsBoardlessAggregatorHydrating(t *testing.T) {
	s := NewRemotedotcom(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("remotedotcom should implement the boardless marker (one public listing)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("remotedotcom should implement the aggregator marker (many employers)")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("remotedotcom should be a HydratingSource (the listing carries no body)")
	}
}

func TestRemotedotcomRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["remotedotcom"]; !ok {
		t.Error("All() should register provider remotedotcom")
	}
	if !slices.Contains(FilterableProviders(), "remotedotcom") {
		t.Error("FilterableProviders() should include remotedotcom")
	}
}

func TestRemotedotcomBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/remotedotcom.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/remotedotcom.yml fails validation: %v", err)
	}
}

// remotedotcomTwoPageFake serves a two-page board whose first page carries the mapping cases
// (a remote posting hired by country, a hybrid one with a desk, a title full of braces, and a
// draft that must be dropped) and whose second is the last per totalPages.
func remotedotcomTwoPageFake() *remotedotcomHTTP {
	page1 := `{"totalCount":4,"currentPage":1,"totalPages":2,"jobs":[
{"status":"published","title":"Senior Backend Developer","slug":"senior-backend-developer-j1a","publishedAt":"2026-08-14T07:55:01Z","employmentType":"full_time","seniority":["senior"],
 "companyProfile":{"name":"Proxify","slug":"proxify-c114"},
 "workplaceLocation":{"type":"remote"},
 "hiringLocation":{"type":"location","timezone":null,"timezoneRange":null,"includedLocations":[{"type":"country","value":{"code":"ZAF","name":"South Africa","alpha2Code":"ZA"}},{"type":"region","value":{"code":"north-america","name":"North America"}}]},
 "compensation":{"minimum":300000,"maximum":600000,"frequency":"monthly","currency":{"code":"EUR","name":"Euro","symbol":"€"}}},
{"status":"published","title":"Tax Analyst {Remote} [LATAM]","slug":"tax-analyst-j1b","publishedAt":"2026-08-10T09:00:00Z","employmentType":"contractor","seniority":["manager"],
 "companyProfile":{"name":"Colibri Group","slug":"colibri-c1xd"},
 "workplaceLocation":{"type":"hybrid","country":{"code":"ROU","name":"Romania","alpha2Code":"RO"},"city":"Iasi","frequency":"weekly","onSiteNumberOfDays":1},
 "hiringLocation":null,
 "compensation":null},
{"status":"draft","title":"Not Live","slug":"not-live-j1c","companyProfile":{"name":"Ghost Co","slug":"ghost-c1"}}
]}`
	page2 := `{"totalCount":4,"currentPage":2,"totalPages":2,"jobs":[
{"status":"published","title":"Support Engineer","slug":"support-engineer-j1d","publishedAt":"2026-08-01T00:00:00Z","employmentType":"full_time","seniority":["entry_level"],
 "companyProfile":{"name":"Welo","slug":"welo-c1iw"},
 "workplaceLocation":{"type":"remote"},
 "hiringLocation":{"type":"global","timezone":null,"includedLocations":null,"timezoneRange":null},
 "compensation":{"minimum":1000000,"maximum":1500000,"frequency":"yearly","currency":{"code":"USD","name":"United States Dollar","symbol":"$$"}}}
]}`
	return &remotedotcomHTTP{
		pages: map[int]string{
			1: remotedotcomListFlight(page1),
			2: remotedotcomListFlight(page2),
		},
		postings: map[string]string{
			"senior-backend-developer-j1a": remotedotcomDetailFlight(
				`{"@context":"https://schema.org","@type":"JobPosting","title":"Senior Backend Developer","description":"<p>Build services.</p>"}`),
			"tax-analyst-j1b": remotedotcomDetailFlight(
				`{"@context":"https://schema.org","@type":"JobPosting","title":"Tax Analyst","description":"<p>File returns.</p>"}`),
			"support-engineer-j1d": remotedotcomDetailFlight(
				`{"@context":"https://schema.org","@type":"JobPosting","title":"Support Engineer","description":"<p>Answer tickets.</p>"}`),
		},
	}
}

func TestRemotedotcomFetchPaginatesAndMaps(t *testing.T) {
	fake := remotedotcomTwoPageFake()
	jobs, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.gotHeaders["RSC"] != "1" {
		t.Errorf(`headers = %v, want RSC:"1" (without it remote.com answers HTML)`, fake.gotHeaders)
	}
	if !slices.Equal(fake.gotPages, []int{1, 2}) {
		t.Errorf("walked pages %v, want [1 2] (stop at totalPages, no page 3)", fake.gotPages)
	}
	if len(fake.gotPostings) != 0 {
		t.Errorf("Fetch requested posting pages %v, want none (list-only fallback)", fake.gotPostings)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the draft is dropped)", len(jobs))
	}

	remote := jobs[0]
	if remote.ExternalID != "senior-backend-developer-j1a" || remote.Company != "Proxify" {
		t.Errorf("bad identity: %+v", remote)
	}
	if want := "https://remote.com/jobs/proxify-c114/senior-backend-developer-j1a"; remote.URL != want {
		t.Errorf("URL = %q, want %q", remote.URL, want)
	}
	if !remote.Remote || remote.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote", remote.Remote, remote.WorkMode)
	}
	if remote.Location != "South Africa, North America" {
		t.Errorf("Location = %q, want the hiring locations joined", remote.Location)
	}
	if !slices.Equal(remote.Countries, []string{"za"}) {
		t.Errorf("Countries = %v, want [za] (the region resolves to no country)", remote.Countries)
	}
	if remote.Seniority != "senior" || remote.EmploymentType != "full_time" {
		t.Errorf("Seniority=%q EmploymentType=%q", remote.Seniority, remote.EmploymentType)
	}
	if remote.PostedAt == nil {
		t.Error("PostedAt nil, want the parsed publishedAt")
	}
	// 300000-600000 minor units of EUR a month is 3000-6000, not 300000-600000.
	if remote.SalaryMin == nil || *remote.SalaryMin != 3000 ||
		remote.SalaryMax == nil || *remote.SalaryMax != 6000 ||
		remote.SalaryCurrency != "EUR" || remote.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s, want 3000-6000 EUR/month",
			remote.SalaryMin, remote.SalaryMax, remote.SalaryCurrency, remote.SalaryPeriod)
	}

	// A title carrying literal braces must not unbalance the scan that isolates jobsData.
	hybrid := jobs[1]
	if hybrid.Title != "Tax Analyst {Remote} [LATAM]" {
		t.Errorf("Title = %q, want the braces preserved", hybrid.Title)
	}
	if hybrid.WorkMode != "hybrid" || hybrid.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want hybrid/false", hybrid.WorkMode, hybrid.Remote)
	}
	if hybrid.Location != "Iasi, Romania" {
		t.Errorf("Location = %q, want the workplace desk", hybrid.Location)
	}
	if !slices.Equal(hybrid.Countries, []string{"ro"}) {
		t.Errorf("Countries = %v, want [ro]", hybrid.Countries)
	}
	if hybrid.EmploymentType != "contract" || hybrid.Seniority != "lead" {
		t.Errorf("EmploymentType=%q Seniority=%q, want contract/lead", hybrid.EmploymentType, hybrid.Seniority)
	}
	if hybrid.SalaryPeriod != "" || hybrid.SalaryMin != nil {
		t.Errorf("salary set from a null compensation: %+v", hybrid)
	}

	global := jobs[2]
	if global.Location != "Worldwide" {
		t.Errorf("Location = %q, want Worldwide for a globally-hired posting", global.Location)
	}
	if global.SalaryMin == nil || *global.SalaryMin != 10000 || global.SalaryPeriod != "year" {
		t.Errorf("salary = %v %s, want 10000 .../year", global.SalaryMin, global.SalaryPeriod)
	}
}

// A timezone-hired posting is not located in the city the zone is named after. The zone name
// must reach the location dictionary as a timezone, or a work-from-anywhere-in-this-zone
// posting is filed under a city — and a country — nobody has to be in.
func TestRemotedotcomTimezoneHiringDoesNotBecomeACity(t *testing.T) {
	fake := &remotedotcomHTTP{pages: map[int]string{
		1: remotedotcomListFlight(`{"totalPages":1,"jobs":[
{"status":"published","title":"Referrals Coordinator","slug":"referrals-coordinator-j1m","companyProfile":{"name":"Synergy","slug":"synergy-c1"},
 "workplaceLocation":{"type":"remote"},
 "hiringLocation":{"type":"timezone","timezone":{"code":"america-chicago","name":"Chicago","offset":-5},"includedLocations":null,"timezoneRange":1}}
]}`),
	}}
	jobs, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if got := jobs[0].Location; got != "Chicago timezone" {
		t.Errorf("Location = %q, want %q — a bare city name is a place claim the posting never makes", got, "Chicago timezone")
	}
	if jobs[0].Countries != nil {
		t.Errorf("Countries = %v, want nil (a timezone states no country)", jobs[0].Countries)
	}
}

func TestRemotedotcomFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	fake := remotedotcomTwoPageFake()
	seen := func(id string) bool { return id == "tax-analyst-j1b" }

	jobs, err := NewRemotedotcom(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}

	slices.Sort(fake.gotPostings)
	want := []string{"senior-backend-developer-j1a", "support-engineer-j1d"}
	if !slices.Equal(fake.gotPostings, want) {
		t.Errorf("fetched posting pages %v, want %v (the seen one costs no request)", fake.gotPostings, want)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if got := byID["senior-backend-developer-j1a"]; got.Description != "<p>Build services.</p>" || got.SeenRefresh {
		t.Errorf("unseen posting: Description=%q SeenRefresh=%v", got.Description, got.SeenRefresh)
	}
	// A seen posting is a liveness touch: no body, and flagged so the pipeline does not
	// rewrite the content it hydrated when the posting was new.
	if got := byID["tax-analyst-j1b"]; !got.SeenRefresh || got.Description != "" {
		t.Errorf("seen posting: SeenRefresh=%v Description=%q", got.SeenRefresh, got.Description)
	}
}

func TestRemotedotcomFetchNewKeepsPostingWhoseBodyFailed(t *testing.T) {
	fake := remotedotcomTwoPageFake()
	fake.postingErr = map[string]bool{"senior-backend-developer-j1a": true}

	jobs, err := NewRemotedotcom(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 — a failed body must not drop the posting", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "senior-backend-developer-j1a" && j.Description != "" {
			t.Errorf("Description = %q, want empty after a failed body", j.Description)
		}
	}
}

func TestRemotedotcomFirstPageFailureIsBoardError(t *testing.T) {
	fake := remotedotcomTwoPageFake()
	fake.pageErr = map[int]bool{1: true}

	if _, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("Fetch: want an error when the first page fails")
	}
}

func TestRemotedotcomLaterPageFailureKeepsPartialWalk(t *testing.T) {
	fake := remotedotcomTwoPageFake()
	fake.pageErr = map[int]bool{2: true}

	jobs, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: want a partial walk, got error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want page 1's 2 published postings", len(jobs))
	}
}

func TestRemotedotcomWalkStopsOnAnEmptyPage(t *testing.T) {
	// totalPages overstates the board, so the empty page past the end is what must stop the
	// walk — otherwise the run would page all the way to the safety bound.
	fake := &remotedotcomHTTP{pages: map[int]string{
		1: remotedotcomListFlight(`{"totalPages":99,"jobs":[{"status":"published","title":"Only","slug":"only-j1","companyProfile":{"name":"Solo","slug":"solo-c1"}}]}`),
		2: remotedotcomListFlight(`{"totalPages":99,"jobs":[]}`),
	}}
	jobs, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !slices.Equal(fake.gotPages, []int{1, 2}) {
		t.Errorf("walked pages %v, want [1 2]", fake.gotPages)
	}
}

func TestRemotedotcomMissingJobsDataIsAnError(t *testing.T) {
	fake := &remotedotcomHTTP{pages: map[int]string{1: `2:["$","div",null,{"children":[]}]`}}
	if _, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("Fetch: a listing with no jobsData must fail loudly, not return an empty board")
	}
}

func TestFlightJobPostingReadsTheTextRow(t *testing.T) {
	flight := remotedotcomDetailFlight(
		`{"@context":"https://schema.org","@type":"JobPosting","description":"<p>Hello, {world}.</p>"}`)

	var got remotedotcomJobPosting
	if !flightJobPosting(flight, &got) {
		t.Fatal("flightJobPosting: want the JobPosting row found past the decoy row")
	}
	if want := "<p>Hello, {world}.</p>"; got.Description != want {
		t.Errorf("Description = %q, want %q", got.Description, want)
	}

	if flightJobPosting(`0:["$","html",null,{}]`, &got) {
		t.Error("flightJobPosting: want false for a flight carrying no JobPosting")
	}
}

// The safety bound only exists so a listing that stops honouring totalPages cannot page
// forever; assert it is wired rather than aspirational.
func TestRemotedotcomWalkIsBounded(t *testing.T) {
	page := remotedotcomListFlight(
		`{"totalPages":0,"jobs":[{"status":"published","title":"Endless","slug":"endless-j1","companyProfile":{"name":"Loop","slug":"loop-c1"}}]}`)
	fake := &remotedotcomHTTP{pages: map[int]string{}}
	for p := 1; p <= remotedotcomMaxPages+5; p++ {
		fake.pages[p] = page
	}
	if _, err := NewRemotedotcom(fake).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.gotPages) != remotedotcomMaxPages {
		t.Errorf("walked %d pages, want the %d-page bound", len(fake.gotPages), remotedotcomMaxPages)
	}
}
