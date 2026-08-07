package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestWorkdayEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full time": "full_time",
		"full time": "full_time",
		"Part time": "part_time",
		"part time": "part_time",
		"":          "",
		"Seasonal":  "",
	}
	for in, want := range cases {
		if got := workdayEmploymentType(in); got != want {
			t.Errorf("workdayEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkdayProvider(t *testing.T) {
	if got := NewWorkday(nil).Provider(); got != "workday" {
		t.Errorf("Provider() = %q, want %q", got, "workday")
	}
}

func TestWorkdayFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 2, "jobPostings": [
			{"title": "Backend Engineer", "externalPath": "/job/Berlin/Backend_JR-1", "locationsText": "Berlin, Germany"},
			{"title": "Data Engineer", "externalPath": "/job/Remote/Data_JR-2", "locationsText": "Remote, US"}
		]}`).
		route("Backend_JR-1", `{"jobPostingInfo": {
			"title": "Backend Engineer",
			"jobDescription": "<p>Build the backend.</p>",
			"location": "Berlin, Germany",
			"startDate": "2024-06-11",
			"externalUrl": "https://acme.wd1.myworkdayjobs.com/en-US/Careers/job/Berlin/Backend_JR-1",
			"remoteType": "On-site",
			"timeType": "Full time",
			"jobRequisitionLocation": {"country": {"descriptor": "Germany", "alpha2Code": "DE"}}
		}}`).
		route("Data_JR-2", `{"jobPostingInfo": {
			"title": "Data Engineer",
			"jobDescription": "<p>Crunch data.</p>",
			"location": "Remote, US",
			"startDate": "2024-06-12",
			"remoteType": "Remote"
		}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["/job/Berlin/Backend_JR-1"]
	if !ok {
		t.Fatal("posting Backend_JR-1 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.URL != "https://acme.wd1.myworkdayjobs.com/en-US/Careers/job/Berlin/Backend_JR-1" {
		t.Errorf("URL = %q, want the detail externalUrl", j.URL)
	}
	if !strings.Contains(j.Description, "Build the backend.") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true, want false for an on-site role")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed startDate (2024)", j.PostedAt)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (from timeType)", j.EmploymentType)
	}
	if got, want := j.Countries, []string{"de"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Countries = %v, want %v (normalized from jobRequisitionLocation.country.alpha2Code)", got, want)
	}

	d := byID["/job/Remote/Data_JR-2"]
	if !d.Remote {
		t.Error("Remote = false, want true from remoteType")
	}
	// This posting's detail carries no jobRequisitionLocation at all.
	if d.Countries != nil {
		t.Errorf("Countries = %v, want nil when the detail states no country", d.Countries)
	}
	if d.URL != "https://acme.wd1.myworkdayjobs.com/Careers/job/Remote/Data_JR-2" {
		t.Errorf("URL = %q, want the path constructed from host+site when externalUrl is absent", d.URL)
	}
}

// A Workday board costs one detail request per posting, and the large ones carry tens of
// thousands — re-fetching all of them every hour is what rate-limits the crawl into failure. So
// FetchNew hydrates only what the catalogue lacks and re-lists the rest for a liveness refresh.
func TestWorkdayFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 2, "jobPostings": [
			{"title": "Backend Engineer", "externalPath": "/job/Berlin/Backend_JR-1", "locationsText": "Berlin, Germany"},
			{"title": "Data Engineer", "externalPath": "/job/Remote/Data_JR-2", "locationsText": "Remote, US"}
		]}`).
		route("Data_JR-2", `{"jobPostingInfo": {
			"title": "Data Engineer",
			"jobDescription": "<p>Crunch data.</p>",
			"location": "Remote, US",
			"remoteType": "Remote"
		}}`)
	// Backend_JR-1 has no detail route on purpose: requesting it would be an error, which is
	// exactly the request this path must not make.
	seen := func(externalID string) bool { return externalID == "/job/Berlin/Backend_JR-1" }

	jobs, err := NewWorkday(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (both listed postings are emitted)", len(jobs))
	}
	if fake.calls != 2 {
		t.Errorf("http calls = %d, want 2 (one listing page + one detail for the unseen posting)", fake.calls)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	refresh, ok := byID["/job/Berlin/Backend_JR-1"]
	if !ok {
		t.Fatal("the seen posting must still be emitted, as a liveness refresh")
	}
	if !refresh.SeenRefresh {
		t.Error("SeenRefresh = false, want true — the pipeline must route it to touch, not upsert")
	}
	if refresh.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want the listing title so the row stays resolvable", refresh.Title)
	}
	if refresh.URL != "https://acme.wd1.myworkdayjobs.com/Careers/job/Berlin/Backend_JR-1" {
		t.Errorf("URL = %q, want the listing-derived URL", refresh.URL)
	}
	if refresh.Description != "" {
		t.Errorf("Description = %q, want empty — no detail was fetched", refresh.Description)
	}

	hydrated := byID["/job/Remote/Data_JR-2"]
	if hydrated.SeenRefresh {
		t.Error("SeenRefresh = true for an unseen posting, want false")
	}
	if !strings.Contains(hydrated.Description, "Crunch data.") {
		t.Errorf("Description = %q, want the hydrated detail", hydrated.Description)
	}
}

// Without a seen-set the pipeline cannot say what the catalogue has, so the adapter must behave
// as it always did rather than skip every description.
func TestWorkdayFetchNewWithoutSeenSetHydratesEverything(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 1, "jobPostings": [
			{"title": "Backend Engineer", "externalPath": "/job/Berlin/Backend_JR-1", "locationsText": "Berlin, Germany"}
		]}`).
		route("Backend_JR-1", `{"jobPostingInfo": {"title": "Backend Engineer", "jobDescription": "<p>Build.</p>"}}`)

	jobs, err := NewWorkday(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].SeenRefresh {
		t.Fatalf("jobs = %+v, want one hydrated posting", jobs)
	}
	if !strings.Contains(jobs[0].Description, "Build.") {
		t.Errorf("Description = %q, want the hydrated detail", jobs[0].Description)
	}
}

func TestWorkdayFetchSkipsFailedDetail(t *testing.T) {
	// JR-2 has no detail route -> its detail fetch errors and the posting is skipped,
	// but JR-1 still comes through.
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 2, "jobPostings": [
			{"title": "Engineer", "externalPath": "/job/X/JR-1", "locationsText": "Berlin"},
			{"title": "Broken", "externalPath": "/job/Y/JR-2", "locationsText": "NYC"}
		]}`).
		route("/job/X/JR-1", `{"jobPostingInfo": {"title": "Engineer", "jobDescription": "<p>ok</p>", "location": "Berlin"}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "/job/X/JR-1" {
		t.Fatalf("want only JR-1 to survive, got %d jobs", len(jobs))
	}
}

// pagedWorkday returns a canned list page per call (in order), delegating detail
// GetJSONs to the embedded routedHTTP. It mimics pg.wd5.myworkdayjobs.com, which
// reports the real `total` only on the first page and `total:0` thereafter.
type pagedWorkday struct {
	*routedHTTP
	pages []string
	call  int
}

func (p *pagedWorkday) PostJSON(_ context.Context, _ string, _, v any) error {
	body := `{"total":0,"jobPostings":[]}`
	if p.call < len(p.pages) {
		body = p.pages[p.call]
	}
	p.call++
	return json.Unmarshal([]byte(body), v)
}

func workdayDetailBody(title string) string {
	return `{"jobPostingInfo":{"title":"` + title + `","jobDescription":"<p>x</p>","location":"Berlin, Germany","startDate":"2024-06-11"}}`
}

// facetedWorkday serves a listing page keyed by the request's appliedFacets (JSON-marshaled,
// which — because Go's encoding/json sorts map keys — gives a stable key regardless of build
// order). This is what lets a test control exactly what a facet-scoped re-query returns, which
// a routedHTTP (URL-only routing) cannot do: every facet-scoped query hits the same listing URL.
type facetedWorkday struct {
	*routedHTTP
	mu     sync.Mutex
	pages  map[string]string
	served map[string]bool
	calls  int
}

func (f *facetedWorkday) route(appliedFacets, body string) *facetedWorkday {
	if f.pages == nil {
		f.pages = map[string]string{}
	}
	f.pages[appliedFacets] = body
	return f
}

// PostJSON serves each appliedFacets key's canned page exactly once; a repeat request for the
// same key (i.e. a second listing page, since no test here has more than one page per key)
// gets an empty page, so pagination terminates instead of looping on a fake that — unlike the
// real API — doesn't vary by offset.
func (f *facetedWorkday) PostJSON(_ context.Context, _ string, body, v any) error {
	f.mu.Lock()
	f.calls++
	m, _ := body.(map[string]any)
	applied, _ := m["appliedFacets"].(map[string]any)
	key, err := json.Marshal(applied)
	if err != nil {
		f.mu.Unlock()
		return fmt.Errorf("facetedWorkday: marshal appliedFacets: %w", err)
	}
	page, ok := f.pages[string(key)]
	alreadyServed := f.served[string(key)]
	if f.served == nil {
		f.served = map[string]bool{}
	}
	f.served[string(key)] = true
	f.mu.Unlock()
	if !ok {
		return fmt.Errorf("facetedWorkday: no page for appliedFacets=%s", key)
	}
	if alreadyServed {
		return json.Unmarshal([]byte(`{"total":0,"jobPostings":[]}`), v)
	}
	return json.Unmarshal([]byte(page), v)
}

// A board whose first page reports the capped total (2000) must not stop there: it should
// re-query per value of the response's own facet, and return the union.
func TestWorkdaySplitsByFacetWhenCapped(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[
				{"id":"fam-eng","count":1200},
				{"id":"fam-sales","count":800}
			]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/ENG-1","locationsText":"Berlin"}
		]}`).
		route(`{"jobFamilyGroup":["fam-sales"]}`, `{"total":1,"jobPostings":[
			{"title":"Sales Rep","externalPath":"/job/X/SALES-1","locationsText":"NYC"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2 (one per facet value, capped total must not stop the walk)", len(postings))
	}
	got := map[string]bool{}
	for _, p := range postings {
		got[p.ExternalPath] = true
	}
	if !got["/job/X/ENG-1"] || !got["/job/X/SALES-1"] {
		t.Errorf("postings = %+v, want both facet slices represented", postings)
	}
}

// When more than one facet dimension is available, the split must pick the one whose largest
// single value is smallest — the dimension that leaves the smallest oversized remainder — not
// the dimension with the single biggest value anywhere. The latter was tried first and, live
// against a real capped board, kept picking a dimension whose top bucket was still ~2000-sized,
// making no real progress. Only dimB's values have routes registered: if the wrong dimension
// (dimA) were picked, the request would hit an unregistered appliedFacets key and error.
func TestWorkdaySplitPicksDimensionWithSmallestMaxValue(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimA","values":[{"id":"a1","count":1900},{"id":"a2","count":100}]},
			{"facetParameter":"dimB","values":[{"id":"b1","count":700},{"id":"b2","count":700},{"id":"b3","count":600}]}
		]}`).
		route(`{"dimB":["b1"]}`, `{"total":1,"jobPostings":[{"title":"B1","externalPath":"/job/X/B1-1","locationsText":"Berlin"}]}`).
		route(`{"dimB":["b2"]}`, `{"total":1,"jobPostings":[{"title":"B2","externalPath":"/job/X/B2-1","locationsText":"Berlin"}]}`).
		route(`{"dimB":["b3"]}`, `{"total":1,"jobPostings":[{"title":"B3","externalPath":"/job/X/B3-1","locationsText":"Berlin"}]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v (want dimB picked — dimA's max value of 1900 is worse than dimB's 700)", err)
	}
	if len(postings) != 3 {
		t.Fatalf("len(postings) = %d, want 3 (one per dimB value)", len(postings))
	}
}

// A facet slice that is itself still capped must recurse into a second, different facet
// dimension — Workday's own facet does not rescope against itself once applied (verified live),
// so the second level has to be a dimension not yet used in this branch.
func TestWorkdaySplitsRecursesIntoSecondDimensionWhenSliceStillCapped(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[{"id":"fam-eng","count":2000}]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[{"id":"fam-eng","count":2000}]},
			{"facetParameter":"workerSubType","values":[
				{"id":"full-time","count":1500},
				{"id":"contract","count":500}
			]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"],"workerSubType":["full-time"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer FT","externalPath":"/job/X/FT-1","locationsText":"Berlin"}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"],"workerSubType":["contract"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer Contract","externalPath":"/job/X/CT-1","locationsText":"Berlin"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	got := map[string]bool{}
	for _, p := range postings {
		got[p.ExternalPath] = true
	}
	if !got["/job/X/FT-1"] || !got["/job/X/CT-1"] {
		t.Errorf("postings = %+v, want both second-level facet slices represented", postings)
	}
}

// Recursion must not go deeper than maxFacetDepth combined dimensions: a slice still capped
// at that depth is paged as-is (best effort) rather than recursed further.
func TestWorkdaySplitStopsAtMaxDepth(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimA","values":[{"id":"a1","count":2000}]}
		]}`).
		route(`{"dimA":["a1"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimB","values":[{"id":"b1","count":2000}]}
		]}`).
		route(`{"dimA":["a1"],"dimB":["b1"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimC","values":[{"id":"c1","count":2000}]}
		]}`).
		// Three dimensions applied (dimA, dimB, dimC) is the depth limit; this slice is still
		// capped and carries a fourth, unused dimension, but recursion must not use it —
		// it must page this response as-is instead.
		route(`{"dimA":["a1"],"dimB":["b1"],"dimC":["c1"]}`, `{"total":2000,"jobPostings":[
			{"title":"Leaf","externalPath":"/job/X/LEAF-1","locationsText":"Remote"}
		],"facets":[
			{"facetParameter":"dimD","values":[{"id":"d1","count":2000}]}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 || postings[0].ExternalPath != "/job/X/LEAF-1" {
		t.Fatalf("postings = %+v, want the depth-limit leaf's single posting, no dimD request", postings)
	}
	if _, ok := fake.pages[`{"dimA":["a1"],"dimB":["b1"],"dimC":["c1"],"dimD":["d1"]}`]; ok {
		t.Error("a dimD route exists on the fake but must never be requested past max depth")
	}
}

// Postings that surface in more than one recursed slice (a multi-counting facet dimension can
// place the same posting under two different values) must be deduped by ExternalPath.
func TestWorkdaySplitDedupsOverlappingPostings(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"workerSubType","values":[
				{"id":"remote-eligible","count":1200},
				{"id":"full-time","count":1900}
			]}
		]}`).
		route(`{"workerSubType":["remote-eligible"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/SHARED-1","locationsText":"Remote"}
		]}`).
		route(`{"workerSubType":["full-time"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/SHARED-1","locationsText":"Remote"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("len(postings) = %d, want 1 — the same posting from two slices must be deduped", len(postings))
	}
}

// A board below the cap must page exactly as before: no appliedFacets-scoped requests at all.
func TestWorkdayBelowCapDoesNotSplit(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/E-1","locationsText":"Berlin"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 || postings[0].ExternalPath != "/job/X/E-1" {
		t.Fatalf("postings = %+v, want the single below-cap posting", postings)
	}
	if fake.calls != 1 {
		t.Errorf("calls = %d, want 1 (a single unfiltered page, no facet split triggered)", fake.calls)
	}
}

// A capped page carrying no facets at all cannot be split further — this must be visible (a log
// line), not a silent truncation, which is the exact failure mode this change exists to fix.
func TestWorkdayLogsWhenCappedWithNoUsableDimension(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/E-1","locationsText":"Berlin"}
		],"facets":[]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("len(postings) = %d, want 1 — the capped page's own postings, best effort", len(postings))
	}
	if !strings.Contains(buf.String(), "workday") || !strings.Contains(buf.String(), "acme") {
		t.Errorf("log output = %q, want a warning naming the provider and board", buf.String())
	}
}

// rateLimitedWorkday fails the listing POST and/or a detail GET with a given status code a
// fixed number of times before succeeding, mimicking Workday rate-limiting a burst with 403.
type rateLimitedWorkday struct {
	*routedHTTP
	mu           sync.Mutex
	listBody     string
	listFailCode int
	listFails    int
	listCalls    int
	getFailCode  int
	getFails     int
}

func (r *rateLimitedWorkday) PostJSON(_ context.Context, _ string, _, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listCalls++
	if r.listFails > 0 {
		r.listFails--
		return &StatusError{Method: "POST", Code: r.listFailCode}
	}
	return json.Unmarshal([]byte(r.listBody), v)
}

func (r *rateLimitedWorkday) GetJSON(ctx context.Context, url string, v any) error {
	r.mu.Lock()
	if r.getFails > 0 {
		r.getFails--
		code := r.getFailCode
		r.mu.Unlock()
		return &StatusError{Method: "GET", Code: code, URL: url}
	}
	r.mu.Unlock()
	return r.routedHTTP.GetJSON(ctx, url, v)
}

// A 403 on the listing request must be retried, not treated as fatal — Workday returns 403 for
// a burst of requests on some tenants, the same way Eightfold's 403 already is (eightfold.go).
func TestWorkdayRetriesRateLimitedListing(t *testing.T) {
	fake := &rateLimitedWorkday{
		routedHTTP: (&routedHTTP{}).
			route("ENG-1", workdayDetailBody("Engineer")),
		listBody: `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/ENG-1","locationsText":"Berlin"}
		]}`,
		listFailCode: 403,
		listFails:    2,
	}
	jobs, err := workday{http: fake}.Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (a rate-limited listing must retry past the 403)", len(jobs))
	}
	if fake.listCalls != 3 {
		t.Errorf("listCalls = %d, want 3 (2 failures then success)", fake.listCalls)
	}
}

// A non-rate-limit failure (e.g. 404) must not be retried — retry stays scoped to the
// rate-limit case, and the board fails fast instead of retrying a request that will never
// succeed.
func TestWorkdayDoesNotRetryNonRateLimitListingError(t *testing.T) {
	fake := &rateLimitedWorkday{
		routedHTTP:   &routedHTTP{},
		listFailCode: 404,
		listFails:    99,
	}
	_, err := workday{http: fake}.Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err == nil {
		t.Fatal("Fetch: want an error for a non-retryable listing failure, got nil")
	}
	if fake.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1 (no retry for a non-rate-limit error)", fake.listCalls)
	}
}

// A board reporting total only on its first page must still be drained fully:
// stopping at a later page's total:0 drops the postings past it, and a dropped
// posting loses its last_seen_at stamp and is closed by the 48h unseen sweep.
func TestWorkdayPagesByFirstPageTotal(t *testing.T) {
	fake := &pagedWorkday{
		routedHTTP: (&routedHTTP{}).
			route("A_JR-1", workdayDetailBody("A")).
			route("B_JR-2", workdayDetailBody("B")).
			route("C_JR-3", workdayDetailBody("C")).
			route("D_JR-4", workdayDetailBody("D")),
		pages: []string{
			`{"total":4,"jobPostings":[{"title":"A","externalPath":"/job/X/A_JR-1"},{"title":"B","externalPath":"/job/X/B_JR-2"}]}`,
			`{"total":0,"jobPostings":[{"title":"C","externalPath":"/job/X/C_JR-3"}]}`,
			`{"total":0,"jobPostings":[{"title":"D","externalPath":"/job/X/D_JR-4"}]}`,
		},
	}
	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("len(jobs) = %d, want 4 (all pages drained despite total:0 after page one)", len(jobs))
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ExternalID] = true
	}
	if !got["/job/X/D_JR-4"] {
		t.Error("posting on page three missing — pagination stopped early on a later page's total:0")
	}
}

func TestParseWorkdayBoardRejectsMalformed(t *testing.T) {
	for _, board := range []string{"", "no-slash-host", "/onlysite", "host-no-dot/site"} {
		if _, err := parseWorkdayBoard(board); err == nil {
			t.Errorf("parseWorkdayBoard(%q) = nil error, want error", board)
		}
	}
}

// Workday publishes a career site under either of two host shapes. On myworkdayjobs.com the
// tenant is the host's own first label; on myworkdaysite.com the host is shared across
// tenants (wd1.myworkdaysite.com) and the tenant lives in the path instead, so the board
// spells it out as a third segment.
func TestParseWorkdayBoardTakesTenantFromPathWhenSpelledOut(t *testing.T) {
	cases := map[string]workdayBoard{
		"acme.wd1.myworkdayjobs.com/Careers": {
			host: "acme.wd1.myworkdayjobs.com", tenant: "acme", site: "Careers",
			publicPath: "Careers",
		},
		"wd1.myworkdaysite.com/snapchat/snap": {
			host: "wd1.myworkdaysite.com", tenant: "snapchat", site: "snap",
			publicPath: "recruiting/snapchat/snap",
		},
	}
	for board, want := range cases {
		got, err := parseWorkdayBoard(board)
		if err != nil {
			t.Errorf("parseWorkdayBoard(%q): %v", board, err)
			continue
		}
		if got != want {
			t.Errorf("parseWorkdayBoard(%q) = %+v, want %+v", board, got, want)
		}
	}
}

// The routes below are the assertion: routedHTTP answers by URL substring, so the board only
// yields a posting if the adapter addressed the shared host with the path tenant — i.e. built
// /wday/cxs/snapchat/snap/jobs rather than /wday/cxs/wd1/snapchat%2Fsnap/jobs.
func TestWorkdayFetchesPathTenantBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		route("https://wd1.myworkdaysite.com/wday/cxs/snapchat/snap/jobs",
			`{"total":1,"jobPostings":[{"title":"Engineer","externalPath":"/job/X/JR-1","locationsText":"Berlin"}]}`).
		route("/job/X/JR-1", `{"jobPostingInfo":{"title":"Engineer","jobDescription":"<p>ok</p>","location":"Berlin"}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Snap", Provider: "workday", Board: "wd1.myworkdaysite.com/snapchat/snap",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	// The public site is not served at the CXS path: /snap/job/... answers 500, the posting
	// lives under /recruiting/<tenant>/<site>.
	if jobs[0].URL != "https://wd1.myworkdaysite.com/recruiting/snapchat/snap/job/X/JR-1" {
		t.Errorf("URL = %q, want the posting's public /recruiting path", jobs[0].URL)
	}
}
