package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// naukriFake serves the real search and posting fixtures (trimmed from responses fetched
// 2026-10-18: five rows of the functional-area-5 search, noOfJobs set to match). pages maps a
// pageNo to a search response; a posting id with no fixture answers the platform's 303
// expired-job answer, and ids in fail answer the given error instead.
type naukriFake struct {
	pages   map[string]string
	details map[string]string
	fail    map[string]error

	mu        sync.Mutex
	headers   []map[string]string
	searches  []url.Values
	requested []string
}

var naukriDetailRE = regexp.MustCompile(`/jobapi/v4/job/(\d+)\?microsite=y$`)

func (f *naukriFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.headers = append(f.headers, headers)
	f.mu.Unlock()
	if m := naukriDetailRE.FindStringSubmatch(u); m != nil {
		id := m[1]
		f.mu.Lock()
		f.requested = append(f.requested, id)
		f.mu.Unlock()
		if err := f.fail[id]; err != nil {
			return err
		}
		raw, ok := f.details[id]
		if !ok {
			return &StatusError{Method: "GET", Code: 303, URL: u}
		}
		return json.Unmarshal([]byte(raw), v)
	}
	if strings.HasPrefix(u, "https://www.naukri.com/jobapi/v3/search?") {
		parsed, err := url.Parse(u)
		if err != nil {
			return err
		}
		q := parsed.Query()
		f.mu.Lock()
		f.searches = append(f.searches, q)
		f.mu.Unlock()
		page := q.Get("pageNo")
		if err := f.fail["page"+page]; err != nil {
			return err
		}
		raw, ok := f.pages[page]
		if !ok {
			return fmt.Errorf("naukriFake: no search page %s", page)
		}
		return json.Unmarshal([]byte(raw), v)
	}
	return fmt.Errorf("naukriFake: no route for %s", u)
}

func (f *naukriFake) detailCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.requested)
	slices.Sort(out)
	return out
}

func naukriRead(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var naukriFixtureIDs = []string{"181026004692", "181026004690", "240826006851", "181026004646", "181026004273"}

func naukriFixtures(t *testing.T) *naukriFake {
	t.Helper()
	f := &naukriFake{
		pages:   map[string]string{"1": naukriRead(t, "naukri_search.json")},
		details: map[string]string{},
		fail:    map[string]error{},
	}
	for _, id := range naukriFixtureIDs {
		f.details[id] = naukriRead(t, "naukri_job_"+id+".json")
	}
	return f
}

// naukriSyntheticPages builds n full search pages from the fixture's first row, each row with
// its own id, declaring total postings — for walks the five-row fixture is too short to show.
func naukriSyntheticPages(t *testing.T, n, perPage, total int) map[string]string {
	t.Helper()
	var src struct {
		JobDetails []map[string]any `json:"jobDetails"`
	}
	if err := json.Unmarshal([]byte(naukriRead(t, "naukri_search.json")), &src); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	id := 100000000000
	for p := 1; p <= n; p++ {
		rows := make([]map[string]any, 0, perPage)
		for i := 0; i < perPage; i++ {
			row := map[string]any{}
			for k, v := range src.JobDetails[0] {
				row[k] = v
			}
			id++
			row["jobId"] = strconv.Itoa(id)
			row["jdURL"] = "/job-listings-synthetic-" + strconv.Itoa(id)
			rows = append(rows, row)
		}
		raw, err := json.Marshal(map[string]any{"noOfJobs": total, "jobDetails": rows})
		if err != nil {
			t.Fatal(err)
		}
		pages[strconv.Itoa(p)] = string(raw)
	}
	return pages
}

func naukriByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestNaukriRegistrationAndMarkers(t *testing.T) {
	s := NewNaukri(nil)
	if s.Provider() != "naukri" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("naukri should be an aggregator")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("naukri should be a HydratingSource")
	}
	if g, ok := s.(sweepGrace); !ok || g.sweepGrace() != naukriSweepGrace {
		t.Error("naukri should declare its own sweep grace")
	}
	// The board is the slice to crawl, and the walk is window-capped, so neither marker holds.
	if _, ok := s.(boardless); ok {
		t.Error("naukri is not boardless: a board is a functional area")
	}
	if _, ok := s.(fullBoardListing); ok {
		t.Error("naukri must not claim fullBoardListing")
	}
	if _, ok := All(nil)["naukri"]; !ok {
		t.Error("All() should register naukri")
	}
	if !slices.Contains(FilterableProviders(), "naukri") {
		t.Error("FilterableProviders() should include naukri")
	}
	if _, ok := proxiedProviders["naukri"]; !ok {
		t.Error("naukri must egress through the proxy: the edge tarpits datacenter IPs")
	}
}

func TestNaukriFetchMapsTheRealFixtures(t *testing.T) {
	f := naukriFixtures(t)
	jobs, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Company: "Naukri", Board: "5"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	// Every request carries the app's identity headers; without them the API answers 400.
	for _, h := range f.headers {
		if h["appid"] != "105" || h["systemid"] != "105" {
			t.Fatalf("request headers = %v, want appid/systemid 105", h)
		}
	}
	q := f.searches[0]
	for k, want := range map[string]string{
		"functionAreaIdGid": "5", "jobAge": strconv.Itoa(naukriJobAge), "sort": "f",
		"noOfResults": strconv.Itoa(naukriPageSize), "pageNo": "1",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("search %s = %q, want %q", k, got, want)
		}
	}

	by := naukriByID(jobs)
	tcs := by["181026004692"]
	if tcs.Title != "Salesforce commerce cloud Support Engineer" || tcs.Company != "Tata Consultancy Services" {
		t.Errorf("identity = %q / %q", tcs.Title, tcs.Company)
	}
	if want := "https://www.naukri.com/job-listings-salesforce-commerce-cloud-support-engineer-tata-consultancy-services-pune-chennai-bengaluru-6-to-11-years-181026004692"; tcs.URL != want {
		t.Errorf("URL = %q", tcs.URL)
	}
	if tcs.Location != "Pune; Chennai; Bengaluru" {
		t.Errorf("Location = %q, want the posting's three places", tcs.Location)
	}
	if tcs.WorkMode != "onsite" || tcs.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want onsite", tcs.WorkMode, tcs.Remote)
	}
	if tcs.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", tcs.EmploymentType)
	}
	if tcs.ExperienceYearsMin == nil || *tcs.ExperienceYearsMin != 6 {
		t.Errorf("ExperienceYearsMin = %v, want 6", tcs.ExperienceYearsMin)
	}
	if !tcs.IsTechHint {
		t.Error("functional area 5 (Engineering - Software & QA) should set IsTechHint")
	}
	if !strings.Contains(tcs.Description, "Salesforce Commerce Cloud (SFCC) Support Engineer") || !strings.Contains(tcs.Description, "<li>") {
		t.Errorf("Description should be the posting's full HTML body, got %.120q", tcs.Description)
	}
	if want := time.UnixMilli(1792346647302); tcs.PostedAt == nil || !tcs.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", tcs.PostedAt, want)
	}
	if tcs.SalaryMin != nil || tcs.SalaryMax != nil {
		t.Errorf("a hidden salary must not be published: %v-%v", tcs.SalaryMin, tcs.SalaryMax)
	}

	if j := by["181026004690"]; j.WorkMode != "hybrid" || j.Remote || j.Location != "Bengaluru" {
		t.Errorf("hybrid posting: WorkMode=%q Remote=%v Location=%q", j.WorkMode, j.Remote, j.Location)
	}
	if j := by["181026004273"]; j.WorkMode != "remote" || !j.Remote {
		t.Errorf("remote posting: WorkMode=%q Remote=%v", j.WorkMode, j.Remote)
	}

	// A published range is per annum in rupees: "95 Lacs-2.5 Cr PA".
	consult := by["240826006851"]
	if consult.SalaryMin == nil || *consult.SalaryMin != 9500000 || consult.SalaryMax == nil || *consult.SalaryMax != 25000000 {
		t.Errorf("salary = %v-%v, want 9500000-25000000", consult.SalaryMin, consult.SalaryMax)
	}
	if consult.SalaryCurrency != "INR" || consult.SalaryPeriod != "year" {
		t.Errorf("salary currency/period = %q/%q", consult.SalaryCurrency, consult.SalaryPeriod)
	}
	if consult.Company != "Dev Solutions" || consult.Location != "Mumbai (All Areas)" {
		t.Errorf("consultant posting: %q at %q", consult.Company, consult.Location)
	}
}

func TestNaukriTechHintFollowsTheFunctionalArea(t *testing.T) {
	for board, want := range map[string]bool{"5": true, "8": true, "3": true, "4": false} {
		jobs, err := NewNaukri(naukriFixtures(t)).Fetch(context.Background(), CompanyEntry{Board: board})
		if err != nil {
			t.Fatalf("board %s: %v", board, err)
		}
		for _, j := range jobs {
			if j.IsTechHint != want {
				t.Errorf("board %s: IsTechHint = %v, want %v", board, j.IsTechHint, want)
				break
			}
		}
	}
}

func TestNaukriRejectsANonNumericBoard(t *testing.T) {
	f := naukriFixtures(t)
	for _, board := range []string{"", "it", "5&jobAge=30", "abc"} {
		if _, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q should be refused", board)
		}
	}
	if len(f.searches) != 0 {
		t.Errorf("a refused board must not reach the API, got %d searches", len(f.searches))
	}
}

func TestNaukriFetchNewSkipsSeenPostings(t *testing.T) {
	f := naukriFixtures(t)
	seen := map[string]bool{"181026004692": true, "240826006851": true}
	src := NewNaukri(f).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "5"}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	by := naukriByID(jobs)
	for id := range seen {
		j := by[id]
		if !j.SeenRefresh || j.Description != "" {
			t.Errorf("seen %s should be a body-less SeenRefresh, got %+v", id, j)
		}
	}
	if got, want := f.detailCalls(), []string{"181026004273", "181026004646", "181026004690"}; !slices.Equal(got, want) {
		t.Errorf("detail calls = %v, want only the unseen %v", got, want)
	}
}

func TestNaukriExpiredPostingIsDroppedAndAFailedDetailDeferred(t *testing.T) {
	f := naukriFixtures(t)
	delete(f.details, "181026004646")                                                     // the platform's 303 expired-job answer
	f.fail["181026004690"] = &StatusError{Method: "GET", Code: 500, URL: "naukri detail"} // no answer at all
	jobs, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "5"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	by := naukriByID(jobs)
	if _, ok := by["181026004646"]; ok {
		t.Error("an expired posting must not be yielded")
	}
	if _, ok := by["181026004690"]; ok {
		t.Error("a posting whose detail failed must be deferred, not stored body-less")
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3", len(jobs))
	}
}

func TestNaukriFailsWhenNoDetailAnswers(t *testing.T) {
	f := naukriFixtures(t)
	for _, id := range naukriFixtureIDs {
		f.fail[id] = errors.New("connection reset")
	}
	if _, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "5"}); err == nil {
		t.Fatal("a crawl that listed postings and read none of them must fail")
	}
}

func TestNaukriEmptyAreaIsQuietNotBroken(t *testing.T) {
	f := &naukriFake{pages: map[string]string{"1": `{"noOfJobs":0,"jobDetails":[]}`}}
	jobs, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "99999"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("Fetch = %d jobs, %v; want none and no error", len(jobs), err)
	}
}

func TestNaukriFirstPageFailureFailsTheBoard(t *testing.T) {
	f := naukriFixtures(t)
	f.fail["page1"] = &StatusError{Method: "GET", Code: 403, URL: "naukri search"}
	if _, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "5"}); err == nil {
		t.Fatal("a board whose first search page fails must fail")
	}
}

func TestNaukriWalksToTheDeclaredTotal(t *testing.T) {
	f := &naukriFake{pages: naukriSyntheticPages(t, 3, naukriPageSize, 2*naukriPageSize+5), fail: map[string]error{}}
	src := NewNaukri(f).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "5"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(f.searches) != 3 {
		t.Errorf("searched %d pages, want 3 for %d postings", len(f.searches), 2*naukriPageSize+5)
	}
	if len(jobs) != 3*naukriPageSize {
		t.Errorf("got %d jobs, want every distinct listed row", len(jobs))
	}
}

func TestNaukriLaterPageFailureKeepsWhatWasRead(t *testing.T) {
	f := &naukriFake{pages: naukriSyntheticPages(t, 3, naukriPageSize, 3*naukriPageSize), fail: map[string]error{}}
	f.fail["page2"] = &StatusError{Method: "GET", Code: 502, URL: "naukri search"}
	src := NewNaukri(f).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "5"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != naukriPageSize {
		t.Errorf("got %d jobs, want the first page's %d", len(jobs), naukriPageSize)
	}
}

func TestNaukriStopsAtThePlatformsDeepestPage(t *testing.T) {
	total := (naukriMaxPages + 10) * naukriPageSize
	f := &naukriFake{pages: naukriSyntheticPages(t, naukriMaxPages+1, 1, total), fail: map[string]error{}}
	src := NewNaukri(f).(HydratingSource)
	if _, err := src.FetchNew(context.Background(), CompanyEntry{Board: "5"}, func(string) bool { return true }); err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// Page 151 answers 400 "Invalid value", so the walk must never ask for it.
	for _, q := range f.searches {
		if n, _ := strconv.Atoi(q.Get("pageNo")); n > naukriMaxPages {
			t.Fatalf("requested page %d past the platform's deepest page %d", n, naukriMaxPages)
		}
	}
}

// naukriBacklogFake lists n new postings on every board's first page, each with a readable
// posting resource — a cold-start backlog larger than any budget.
func naukriBacklogFake(t *testing.T, n int) *naukriFake {
	t.Helper()
	f := &naukriFake{pages: naukriSyntheticPages(t, 1, n, n), fail: map[string]error{}, details: map[string]string{}}
	body := naukriRead(t, "naukri_job_181026004692.json")
	var pages struct {
		JobDetails []struct {
			JobID string `json:"jobId"`
		} `json:"jobDetails"`
	}
	if err := json.Unmarshal([]byte(f.pages["1"]), &pages); err != nil {
		t.Fatal(err)
	}
	for _, r := range pages.JobDetails {
		f.details[r.JobID] = strings.Replace(body, `"181026004692"`, strconv.Quote(r.JobID), 1)
	}
	return f
}

func TestNaukriOneBoardSpendsOnlyItsShare(t *testing.T) {
	f := naukriBacklogFake(t, naukriDetailBudget+20)
	jobs, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "5"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(f.detailCalls()) != naukriBoardDetailShare || len(jobs) != naukriBoardDetailShare {
		t.Errorf("detail calls = %d, jobs = %d; want both bounded at the board share %d",
			len(f.detailCalls()), len(jobs), naukriBoardDetailShare)
	}
}

// The budget is the RUN's: cmd/ingest crawls every board of the provider through one adapter,
// so the boards together never exceed it, and each technical area still gets its share.
func TestNaukriBoardsOfARunShareOneDetailBudget(t *testing.T) {
	f := naukriBacklogFake(t, naukriDetailBudget)
	src := NewNaukri(f)
	perBoard := map[string]int{}
	for _, board := range []string{"5", "8", "3", "4"} {
		before := len(f.detailCalls())
		if _, err := src.Fetch(context.Background(), CompanyEntry{Board: board}); err != nil {
			t.Fatalf("board %s: %v", board, err)
		}
		perBoard[board] = len(f.detailCalls()) - before
	}
	if total := len(f.detailCalls()); total != naukriDetailBudget {
		t.Errorf("detail calls across the run = %d, want the run budget %d", total, naukriDetailBudget)
	}
	for _, board := range []string{"5", "8", "3"} {
		if perBoard[board] != naukriBoardDetailShare {
			t.Errorf("board %s got %d details, want its share %d", board, perBoard[board], naukriBoardDetailShare)
		}
	}
	if perBoard["4"] != 0 {
		t.Errorf("board 4 got %d details after the run budget was spent", perBoard["4"])
	}
}

// naukriRecaptcha is the posting API's refusal of the egress IP, as prod's crawls received it
// on 2026-10-19: HTTP 406 {"message":"recaptcha required"}.
func naukriRecaptcha() error {
	return &StatusError{Method: "GET", Code: http.StatusNotAcceptable, URL: "naukri detail"}
}

// A refused posting API must stop the run's hydration after naukriRefusalLimit refusals: on
// 2026-10-19 each run sent 1,500 refused detail requests through the shared proxy, every one
// answered 406.
func TestNaukriRecaptchaRefusalStopsTheRunsHydration(t *testing.T) {
	f := naukriBacklogFake(t, naukriDetailBudget)
	for id := range f.details {
		f.fail[id] = naukriRecaptcha()
	}
	src := NewNaukri(f)
	_, err := src.Fetch(context.Background(), CompanyEntry{Board: "5"})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Fetch error = %v, want the board failed as refused", err)
	}
	// Only requests already in flight when the last allowed refusal lands may still go out.
	if n := len(f.detailCalls()); n > naukriRefusalLimit+naukriDetailWorkers {
		t.Errorf("sent %d detail requests, want at most the %d refusals allowed plus the %d in flight", n, naukriRefusalLimit, naukriDetailWorkers)
	}
	// The refusal is the egress IP's, so a later board of the same run asks nothing.
	before := len(f.detailCalls())
	if _, err := src.Fetch(context.Background(), CompanyEntry{Board: "8"}); err == nil {
		t.Error("a later board of a refused run must fail too")
	}
	if n := len(f.detailCalls()) - before; n != 0 {
		t.Errorf("a later board sent %d detail requests after the run was refused", n)
	}
}

func TestNaukriRefusalAfterSomeAnswersKeepsThem(t *testing.T) {
	f := naukriFixtures(t)
	f.fail["181026004273"] = naukriRecaptcha()
	jobs, err := NewNaukri(f).Fetch(context.Background(), CompanyEntry{Board: "5"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("postings hydrated before the refusal must be kept")
	}
	if _, ok := naukriByID(jobs)["181026004273"]; ok {
		t.Error("the refused posting must be deferred, not stored body-less")
	}
}

// A refused board still refreshes the liveness of the postings it already stores.
func TestNaukriRefusedBoardKeepsItsSeenRefreshes(t *testing.T) {
	f := naukriFixtures(t)
	for _, id := range naukriFixtureIDs {
		f.fail[id] = naukriRecaptcha()
	}
	seen := map[string]bool{"181026004692": true, "240826006851": true}
	src := NewNaukri(f).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "5"}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != len(seen) {
		t.Fatalf("got %d jobs, want the %d seen refreshes", len(jobs), len(seen))
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("%s should be a SeenRefresh", j.ExternalID)
		}
	}
}

func TestWithUserAgentOverridesOnlyTheCopy(t *testing.T) {
	base := NewClient()
	got, ok := withUserAgent(base, naukriUserAgent).(*Client)
	if !ok {
		t.Fatal("withUserAgent should return a *Client for a *Client")
	}
	if got.userAgent != naukriUserAgent {
		t.Errorf("userAgent = %q", got.userAgent)
	}
	if base.userAgent == naukriUserAgent {
		t.Error("withUserAgent must not change the shared client")
	}
	if withUserAgent(nil, naukriUserAgent) != nil {
		t.Error("the taxonomy path's nil client must stay nil")
	}
}

// A posting whose employer is already covered, or whose title the catalogue filter rejects, is
// yielded body-less without a detail request. Without the gate the budget went to exactly those
// postings: never stored, so never seen, so bought again every run (2026-10-25).
func TestNaukriGateSkipsTheDetailOfADiscardedPosting(t *testing.T) {
	f := naukriFixtures(t)
	keep := naukriFixtureIDs[0]
	gate := DetailGate{
		Covered:  func(cs []string) map[string]bool { return map[string]bool{} },
		Rejected: func(j Job) bool { return j.ExternalID != keep },
	}
	jobs, err := NewNaukri(f).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{Board: "5"}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.detailCalls(); !slices.Equal(got, []string{keep}) {
		t.Fatalf("detail calls = %v, want only %s", got, keep)
	}
	if len(jobs) != len(naukriFixtureIDs) {
		t.Fatalf("got %d jobs, want every listed posting yielded", len(jobs))
	}
	for _, j := range jobs {
		if (j.Description != "") != (j.ExternalID == keep) {
			t.Errorf("%s: body %d chars", j.ExternalID, len(j.Description))
		}
	}

	f = naukriFixtures(t)
	all := DetailGate{Covered: func(cs []string) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c] = true
		}
		return out
	}}
	if _, err := NewNaukri(f).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{Board: "5"}, func(string) bool { return false }, all); err != nil {
		t.Fatal(err)
	}
	if n := len(f.detailCalls()); n != 0 {
		t.Errorf("covered employers cost %d detail requests", n)
	}
}

// Through the rotating gateway a refusal is one bad exit: fewer than naukriRefusalLimit of them
// must not end the run's hydration (on 2026-10-25 a single 406 ended two runs' hydration).
func TestNaukriFewRefusalsDoNotEndTheRun(t *testing.T) {
	f := naukriFixtures(t)
	f.fail[naukriFixtureIDs[0]] = naukriRecaptcha()
	src := NewNaukri(f)
	if _, err := src.Fetch(context.Background(), CompanyEntry{Board: "5"}); err != nil {
		t.Fatal(err)
	}
	if src.(naukri).refused() {
		t.Error("one refusal ended the run's hydration")
	}
}
