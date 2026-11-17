package sources

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// kariyernetFixtures holds the posting page the fake serves: the Franklin Electric page fetched
// on 2026-11-17, trimmed to its server-rendered state (testdata/kariyernet/posting_4597170.html).
//
//go:embed testdata/kariyernet
var kariyernetFixtures embed.FS

// kariyernetFake serves responses shaped like the ones the search gateway answered on
// 2026-11-17 (rows and the Franklin Electric posting trimmed to the fields the adapter reads).
// The unfiltered request answers the filter list (areas, default the one work area "78"); a
// request for a work area is answered from byArea[area], or from pages when byArea is unset.
// pages and byArea map a currentPage to a search response. details maps a posting id to the
// posting JSON (the shape the gateway's /job answered, and the page's state carries), which the
// fake serves as the posting's page; a missing id answers 404. refusals counts the 403s a page
// answers before it is served.
type kariyernetFake struct {
	areas   []string
	pages   map[int]string
	byArea  map[string]map[int]string
	details map[string]string
	// refusals maps a posting id to how many reads answer 403 before the page is served; -1 is
	// every read.
	refusals map[string]int

	mu       sync.Mutex
	searches []map[string]any
	read     []string
}

func (f *kariyernetFake) PostJSON(_ context.Context, url string, body, v any) error {
	if url != kariyernetSearchURL {
		return fmt.Errorf("kariyernetFake: no POST route for %s", url)
	}
	req := body.(map[string]any)
	f.mu.Lock()
	f.searches = append(f.searches, req)
	f.mu.Unlock()
	pages := f.pages
	if depts, ok := req["departments"].([]string); !ok {
		areas := f.areas
		if areas == nil {
			areas = []string{"78"}
		}
		var items []string
		for _, a := range areas {
			items = append(items, fmt.Sprintf(`{"value":%q}`, a))
		}
		return json.Unmarshal([]byte(fmt.Sprintf(`{"data":{"totalJobCount":1,"filters":{"workAreas":{"items":[%s]}}}}`, strings.Join(items, ","))), v)
	} else if f.byArea != nil {
		pages = f.byArea[depts[0]]
	}
	raw, ok := pages[req["currentPage"].(int)]
	if !ok {
		return fmt.Errorf("kariyernetFake: no search page %v", req["currentPage"])
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *kariyernetFake) GetText(_ context.Context, u string) (string, error) {
	if !strings.HasPrefix(u, kariyernetBase+"/is-ilani/") {
		return "", fmt.Errorf("kariyernetFake: no GET route for %s", u)
	}
	id := u[strings.LastIndexAny(u, "-/")+1:]
	f.mu.Lock()
	f.read = append(f.read, id)
	left, refused := f.refusals[id]
	if refused && left > 0 {
		f.refusals[id]--
	}
	f.mu.Unlock()
	if refused && left != 0 {
		return "", &StatusError{Method: "GET", Code: 403, URL: u}
	}
	raw, ok := f.details[id]
	if !ok {
		return "", &StatusError{Method: "GET", Code: 404, URL: u}
	}
	if strings.HasPrefix(raw, "<") {
		return raw, nil
	}
	return kariyernetPageOf(raw), nil
}

// kariyernetPageOf wraps posting JSON in the page shape the site serves: the posting is the
// jobInfo of the Nuxt state.
func kariyernetPageOf(raw string) string {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		panic(err)
	}
	return `<html><body><script>window.__NUXT__=(function(){return {layout:"default",state:{jobDetail:{jobInfo:` + string(env.Data) + `}}}}());</script></body></html>`
}

func kariyernetRow(id int, title, company string, confidential bool) string {
	return fmt.Sprintf(`{"id":%d,"title":%q,"companyName":%q,"confidential":%t}`, id, title, company, confidential)
}

func kariyernetPage(total int, rows ...string) string {
	return fmt.Sprintf(`{"statusCode":"Success","data":{"totalJobCount":%d,"jobs":{"items":[%s]}}}`, total, strings.Join(rows, ","))
}

const kariyernetFranklin = `{"statusCode":"Success","data":{
 "jobGeneralInformation":{"id":4597170,"title":" Senior  Purchasing Specialist","jobUrl":"/is-ilani/franklin-electric-senior-purchasing-specialist-4597170",
  "confidential":false,"qualifications":"Bachelor's degree","qualificationsWithHtml":"<p>Franklin Electric is looking for a buyer.</p><ul><li>5 years</li></ul><script>x()</script>",
  "isActive":true,"postingDate":"2026-11-13","closingDateNumeric":"12.12.2026",
  "locations":[{"countryName":"Türkiye","cityName":"İzmir"},{"countryName":"Türkiye","cityName":"İzmir"},{"countryName":"Almanya","cityName":"Berlin"}]},
 "jobPositionInformation":{"workType":"Periodical","workModel":"Hybrid","positionLevel":5,"isRemoteWork":false},
 "jobCompanyInformation":{"companyName":"Franklin Electric"}}}`

func kariyernetDetailFor(id int, mutate func(string) string) string {
	s := strings.NewReplacer("4597170", strconv.Itoa(id)).Replace(kariyernetFranklin)
	if mutate != nil {
		s = mutate(s)
	}
	return s
}

func TestKariyerNetIsABoardlessAggregator(t *testing.T) {
	s := NewKariyerNet(nil)
	if s.Provider() != "kariyernet" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("kariyernet must be boardless: one search names the whole slice")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("kariyernet must be an aggregator: every posting names its own employer")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("kariyernet must hydrate: the search carries no body")
	}
}

func TestKariyerNetMapsAPosting(t *testing.T) {
	f := &kariyernetFake{
		pages:   map[int]string{1: kariyernetPage(1, kariyernetRow(4597170, "Senior Purchasing Specialist", "Franklin Electric", false))},
		details: map[string]string{"4597170": kariyernetFranklin},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "4597170" || j.Title != "Senior Purchasing Specialist" || j.Company != "Franklin Electric" {
		t.Errorf("identity = %q %q %q", j.ExternalID, j.Title, j.Company)
	}
	if j.URL != "https://www.kariyer.net/is-ilani/franklin-electric-senior-purchasing-specialist-4597170" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "İzmir; Berlin, Almanya" {
		t.Errorf("Location = %q: distinct cities, country named only when not Türkiye", j.Location)
	}
	if !strings.Contains(j.Description, "Franklin Electric is looking for a buyer.") || strings.Contains(j.Description, "<script") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("WorkMode = %q Remote = %v", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "contract" || j.Seniority != "junior" {
		t.Errorf("EmploymentType = %q Seniority = %q", j.EmploymentType, j.Seniority)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-13" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-12" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if j.IsTechHint {
		t.Error("the work-area filter is the employer's own pick and must not hint a posting technical")
	}
	if len(j.FallbackCountries) != 1 || j.FallbackCountries[0] != "tr" {
		t.Errorf("FallbackCountries = %v", j.FallbackCountries)
	}
}

func TestKariyerNetAsksForEachWorkAreaNewestFirst(t *testing.T) {
	f := &kariyernetFake{
		pages:   map[int]string{1: kariyernetPage(1, kariyernetRow(4597170, "x", "Franklin Electric", false))},
		details: map[string]string{"4597170": kariyernetFranklin},
	}
	if _, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	if len(f.searches) != 2 || f.searches[0]["departments"] != nil {
		t.Fatalf("searches = %v, want the unfiltered filter-list request, then one per area", f.searches)
	}
	req := f.searches[1]
	if got := fmt.Sprint(req["departments"]); got != "[78]" {
		t.Errorf("departments = %s", got)
	}
	if req["size"] != kariyernetPageSize || req["sortType"] != "SortByDate" || req["sortDirection"] != "Descending" {
		t.Errorf("request = %v", req)
	}
}

// The search prepends sponsored rows to every page, so a page repeats ids and its count runs
// above its size; the walk dedups, and stops on the page that adds nothing new.
func TestKariyerNetWalksPagesAndDedupsSponsoredRows(t *testing.T) {
	a, b, c := kariyernetRow(1, "A", "Co", false), kariyernetRow(2, "B", "Co", false), kariyernetRow(3, "C", "Co", false)
	f := &kariyernetFake{
		pages: map[int]string{
			1: kariyernetPage(3, a, b),
			2: kariyernetPage(3, a, c),
			3: kariyernetPage(3, a),
		},
		details: map[string]string{
			"1": kariyernetDetailFor(1, nil), "2": kariyernetDetailFor(2, nil), "3": kariyernetDetailFor(3, nil),
		},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want 3 distinct", len(jobs))
	}
	if len(f.searches) != 3 {
		t.Errorf("%d searches, want 3: the filter list, then two pages, the walk ending once the listed total is collected", len(f.searches))
	}
}

func TestKariyerNetReadsOnlyNewPostingsAndRefreshesSeenOnes(t *testing.T) {
	f := &kariyernetFake{
		pages: map[int]string{1: kariyernetPage(2,
			kariyernetRow(1, "Old", "Acme", false), kariyernetRow(2, "New", "Acme", false))},
		details: map[string]string{"2": kariyernetDetailFor(2, nil)},
	}
	jobs, err := NewKariyerNet(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "1" })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.read) != 1 || f.read[0] != "2" {
		t.Errorf("detail reads = %v, want only the new posting", f.read)
	}
	var refreshed, full int
	for _, j := range jobs {
		switch {
		case j.SeenRefresh && j.ExternalID == "1" && j.Title == "Old" && j.Company == "Acme":
			refreshed++
		case !j.SeenRefresh && j.ExternalID == "2":
			full++
		}
	}
	if refreshed != 1 || full != 1 {
		t.Errorf("jobs = %+v", jobs)
	}
}

func TestKariyerNetSkipsConfidentialPostingsBeforeAnyRequest(t *testing.T) {
	f := &kariyernetFake{
		pages: map[int]string{1: kariyernetPage(2,
			kariyernetRow(1, "Blind", "Gizli Firma", true), kariyernetRow(2, "Named", "Acme", false))},
		details: map[string]string{"2": kariyernetDetailFor(2, nil)},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "2" {
		t.Errorf("jobs = %+v", jobs)
	}
	if len(f.read) != 1 {
		t.Errorf("detail reads = %v: the blind posting names no employer and costs no request", f.read)
	}
}

func TestKariyerNetDefersWhatItCannotUse(t *testing.T) {
	inactive := kariyernetDetailFor(2, func(s string) string { return strings.Replace(s, `"isActive":true`, `"isActive":false`, 1) })
	noBody := kariyernetDetailFor(3, func(s string) string {
		s = strings.Replace(s, `"qualificationsWithHtml":"<p>Franklin Electric is looking for a buyer.</p><ul><li>5 years</li></ul><script>x()</script>"`, `"qualificationsWithHtml":""`, 1)
		return strings.Replace(s, `"qualifications":"Bachelor's degree"`, `"qualifications":""`, 1)
	})
	wrongID := kariyernetDetailFor(9, nil)
	f := &kariyernetFake{
		pages: map[int]string{1: kariyernetPage(5,
			kariyernetRow(1, "ok", "Acme", false), kariyernetRow(2, "inactive", "Acme", false),
			kariyernetRow(3, "no body", "Acme", false), kariyernetRow(4, "gone", "Acme", false),
			kariyernetRow(5, "mismatch", "Acme", false))},
		details: map[string]string{"1": kariyernetDetailFor(1, nil), "2": inactive, "3": noBody, "5": wrongID},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Errorf("jobs = %+v, want only the readable, active posting", jobs)
	}
}

func TestKariyerNetBoundsDetailReadsPerRun(t *testing.T) {
	var rows []string
	details := map[string]string{}
	for i := 1; i <= 5; i++ {
		rows = append(rows, kariyernetRow(i, "t", "Acme", false))
		details[strconv.Itoa(i)] = kariyernetDetailFor(i, nil)
	}
	f := &kariyernetFake{pages: map[int]string{1: kariyernetPage(5, rows...)}, details: details}
	jobs, err := kariyernet{api: f, detailBudget: 2}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || len(f.read) != 2 {
		t.Errorf("jobs = %d reads = %d, want the budget of 2", len(jobs), len(f.read))
	}
	// Taken in listing order, so the newest postings are the ones read.
	if jobs[0].ExternalID != "1" || jobs[1].ExternalID != "2" {
		t.Errorf("read %s, %s: want the first two listed", jobs[0].ExternalID, jobs[1].ExternalID)
	}
}

func TestKariyerNetFailsWhenItReadsNoneOfWhatItListed(t *testing.T) {
	f := &kariyernetFake{
		pages: map[int]string{1: kariyernetPage(2, kariyernetRow(1, "a", "Acme", false), kariyernetRow(2, "b", "Acme", false))},
	}
	if _, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{}); err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want a refusal-shaped crawl to fail", err)
	}
}

func TestKariyerNetFailsOnAnEmptyOrBrokenListing(t *testing.T) {
	empty := &kariyernetFake{pages: map[int]string{1: kariyernetPage(0)}}
	if _, err := NewKariyerNet(empty).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a catalogue with no posting at all must fail, not read as an empty market")
	}
	second := &kariyernetFake{pages: map[int]string{1: kariyernetPage(9, kariyernetRow(1, "a", "Acme", false))}}
	if _, err := NewKariyerNet(second).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a later page failing must fail the crawl")
	}
	noAreas := &kariyernetFake{areas: []string{}}
	if _, err := NewKariyerNet(noAreas).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a search naming no work area cannot be read whole and must fail")
	}
	var se *StatusError
	failing := kariyernetFailingAPI{err: &StatusError{Method: "POST", Code: 403, URL: kariyernetSearchURL}}
	if _, err := NewKariyerNet(failing).Fetch(context.Background(), CompanyEntry{}); !errors.As(err, &se) {
		t.Errorf("err = %v, want the search's status error to surface", err)
	}
}

type kariyernetFailingAPI struct{ err error }

func (f kariyernetFailingAPI) PostJSON(context.Context, string, any, any) error { return f.err }
func (f kariyernetFailingAPI) GetText(context.Context, string) (string, error)  { return "", f.err }

func TestKariyerNetWorkModeFallsBackToTheRemoteFlag(t *testing.T) {
	d := kariyernetDetailFor(7, func(s string) string {
		return strings.Replace(s, `"workModel":"Hybrid","positionLevel":5,"isRemoteWork":false`, `"workModel":"","positionLevel":5,"isRemoteWork":true`, 1)
	})
	var detail kariyernetDetail
	if err := json.Unmarshal([]byte(d), &detail); err != nil {
		t.Fatal(err)
	}
	j, ok := detail.toJob("7")
	if !ok || j.WorkMode != "remote" || !j.Remote {
		t.Errorf("job = %+v ok = %v", j, ok)
	}
}

// The catalogue is read one work area at a time, the IT areas first so the detail budget reaches
// them; a posting listed under two areas is read once, an area holding nothing is fine, and each
// area is walked to its own end.
func TestKariyerNetReadsEveryWorkAreaITFirst(t *testing.T) {
	a, b, c := kariyernetRow(1, "Cashier", "Shop", false), kariyernetRow(2, "Developer", "Soft", false), kariyernetRow(3, "Nurse", "Clinic", false)
	f := &kariyernetFake{
		areas: []string{"36", "45", "78", "5"},
		byArea: map[string]map[int]string{
			"36": {1: kariyernetPage(2, a, c)},
			"45": {1: kariyernetPage(0)},
			"78": {1: kariyernetPage(1, b)},
			"5":  {1: kariyernetPage(2, b, c)},
		},
		details: map[string]string{"1": kariyernetDetailFor(1, nil), "2": kariyernetDetailFor(2, nil), "3": kariyernetDetailFor(3, nil)},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("got %d jobs, want the 3 distinct postings of the 4 areas", len(jobs))
	}
	var order []string
	for _, s := range f.searches[1:] {
		order = append(order, s["departments"].([]string)[0])
	}
	if got := strings.Join(order, ","); got != "78,5,36,45" {
		t.Errorf("areas walked %s, want the IT areas (78, 5) first, the rest in the filter's order", got)
	}
	// The IT areas come first, so the postings they hold are the first the budget reaches.
	f.read = nil
	one, err := kariyernet{api: f, detailBudget: 1}.Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(one) != 1 || one[0].ExternalID != "2" {
		t.Errorf("a budget of one read %v (err %v), want the IT posting 2", one, err)
	}
}

// An area whose walk reaches the search's deepest page without a page that adds nothing has not
// been read whole, and the crawl fails rather than storing a part of the catalogue as all of it.
func TestKariyerNetFailsWhenAnAreaNeverEnds(t *testing.T) {
	pages := map[int]string{}
	for p := 1; p <= kariyernetMaxPages; p++ {
		pages[p] = kariyernetPage(1_000_000, kariyernetRow(p, "t", "Acme", false))
	}
	f := &kariyernetFake{pages: pages}
	_, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err == nil || !strings.Contains(err.Error(), "end was not reached") {
		t.Errorf("err = %v, want the unfinished area to fail the crawl", err)
	}
}

// The page of a real posting (testdata/kariyernet/posting_4597170.html, trimmed to its state) is
// read through the state's own parameter names: the title, company and dates are arguments the
// returned object points at.
func TestKariyerNetReadsARealPage(t *testing.T) {
	page, err := kariyernetFixtures.ReadFile("testdata/kariyernet/posting_4597170.html")
	if err != nil {
		t.Fatal(err)
	}
	f := &kariyernetFake{
		pages: map[int]string{1: kariyernetPage(1,
			`{"id":4597170,"title":"x","companyName":"x","confidential":false,"jobUrl":"/is-ilani/franklin-electric-senior-purchasing-specialist-4597170"}`)},
		details: map[string]string{"4597170": string(page)},
	}
	jobs, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %d err = %v", len(jobs), err)
	}
	j := jobs[0]
	// The posting was republished the morning the page was fetched, so the page's own date is the 18th.
	if j.Title != "Senior Purchasing Specialist" || j.Company != "Franklin Electric" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.URL != "https://www.kariyer.net/is-ilani/franklin-electric-senior-purchasing-specialist-4597170" {
		t.Errorf("URL = %q", j.URL)
	}
	if !strings.Contains(j.Description, "Senior Purchasing Specialist") || !strings.Contains(j.Description, "Torbalı, İzmir") {
		t.Errorf("Description = %.120q", j.Description)
	}
	if j.Location != "İzmir" || j.WorkMode != "onsite" || j.EmploymentType != "full_time" {
		t.Errorf("location/mode/type = %q / %q / %q", j.Location, j.WorkMode, j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-18" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-12" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if got := f.read; len(got) != 1 || got[0] != "4597170" {
		t.Errorf("pages read = %v", got)
	}
}

// Over the proxy each request leaves from a different exit, so a refused page is asked again; a
// page refused on every attempt defers its posting, and a crawl in which every page is refused
// fails rather than reporting a quiet run.
func TestKariyerNetAsksARefusedPageAgain(t *testing.T) {
	rows := kariyernetPage(2, kariyernetRow(1, "a", "Acme", false), kariyernetRow(2, "b", "Acme", false))
	f := &kariyernetFake{
		pages:    map[int]string{1: rows},
		details:  map[string]string{"1": kariyernetDetailFor(1, nil), "2": kariyernetDetailFor(2, nil)},
		refusals: map[string]int{"1": kariyernetPageRetries, "2": -1},
	}
	jobs, err := kariyernet{api: f, retryBackoff: time.Nanosecond}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Errorf("jobs = %+v, want the page served on its last attempt only", jobs)
	}
	count := map[string]int{}
	for _, id := range f.read {
		count[id]++
	}
	if count["1"] != kariyernetPageRetries+1 || count["2"] != kariyernetPageRetries+1 {
		t.Errorf("reads per posting = %v, want %d attempts each", count, kariyernetPageRetries+1)
	}

	every := &kariyernetFake{
		pages:    map[int]string{1: rows},
		details:  map[string]string{"1": kariyernetDetailFor(1, nil), "2": kariyernetDetailFor(2, nil)},
		refusals: map[string]int{"1": -1, "2": -1},
	}
	if _, err := (kariyernet{api: every, retryBackoff: time.Nanosecond}).Fetch(context.Background(), CompanyEntry{}); err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want a crawl whose every page is refused to fail", err)
	}
}

// A page that is not the posting (a challenge page, a moved layout) is no answer: the posting is
// deferred, never stored half-read.
func TestKariyerNetDefersAPageWithoutState(t *testing.T) {
	for name, page := range map[string]string{
		"challenge":  "<html><body>Just a moment...</body></html>",
		"no jobInfo": `<html><script>window.__NUXT__=(function(a){return {state:{x:a}}}(1));</script></html>`,
		"call":       `<html><script>window.__NUXT__=(function(a){return {state:{jobDetail:{jobInfo:{id:Date.now()}}}}}(1));</script></html>`,
		"unbound":    `<html><script>window.__NUXT__=(function(a){return {state:{jobDetail:{jobInfo:{id:zz}}}}}(1));</script></html>`,
	} {
		if _, err := kariyernetPageDetail(page); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// The state's literals: parameters resolved to their arguments, minified booleans, void 0,
// JavaScript escapes (Nuxt writes "/" as \u002F) and astral characters spelled as a pair.
func TestKariyerNetPageLiteral(t *testing.T) {
	page := `<script>window.__NUXT__=(function(a,b,c,d){return {state:{jobDetail:{jobInfo:{
		jobGeneralInformation:{id:7,title:a,confidential:!1,isActive:!0,qualificationsWithHtml:"<p>x\u003C\u002Fp>\n\ud83d\ude00 \x41",
		locations:[{cityName:b,countryName:c}],postingDate:d,jobCode:void 0},
		jobCompanyInformation:{companyName:"Acme"}}}}}}("Title, \"quoted\"","İzmir","Türkiye","2026-11-13"));</script>`
	d, err := kariyernetPageDetail(page)
	if err != nil {
		t.Fatal(err)
	}
	g := d.Data.General
	if g.Title != `Title, "quoted"` || g.ID != 7 || g.Confidential || !g.IsActive || g.PostingDate != "2026-11-13" {
		t.Errorf("general = %+v", g)
	}
	if g.QualificationsHTML != "<p>x</p>\n\U0001F600 A" {
		t.Errorf("html = %q", g.QualificationsHTML)
	}
	if len(g.Locations) != 1 || g.Locations[0].CityName != "İzmir" || d.Data.Company.Name != "Acme" {
		t.Errorf("locations = %+v company = %q", g.Locations, d.Data.Company.Name)
	}
}
