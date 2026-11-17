package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// kariyernetFake serves responses shaped like the ones the search gateway answered on
// 2026-11-17 (rows and the Franklin Electric posting trimmed to the fields the adapter reads).
// pages maps a currentPage to a search response; details maps a posting id to a /job response,
// and a missing id answers 404 as the gateway does.
type kariyernetFake struct {
	pages   map[int]string
	details map[string]string

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
	raw, ok := f.pages[req["currentPage"].(int)]
	if !ok {
		return fmt.Errorf("kariyernetFake: no search page %v", req["currentPage"])
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *kariyernetFake) GetJSON(_ context.Context, u string, v any) error {
	id, ok := strings.CutPrefix(u, kariyernetDetailURL)
	if !ok {
		return fmt.Errorf("kariyernetFake: no GET route for %s", u)
	}
	f.mu.Lock()
	f.read = append(f.read, id)
	f.mu.Unlock()
	raw, ok := f.details[id]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return json.Unmarshal([]byte(raw), v)
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

func TestKariyerNetAsksForTheITSliceNewestFirst(t *testing.T) {
	f := &kariyernetFake{
		pages:   map[int]string{1: kariyernetPage(1, kariyernetRow(4597170, "x", "Franklin Electric", false))},
		details: map[string]string{"4597170": kariyernetFranklin},
	}
	if _, err := NewKariyerNet(f).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	req := f.searches[0]
	if got := fmt.Sprint(req["departments"]); got != "[78 5 55 60 54]" {
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
	if len(f.searches) != 2 {
		t.Errorf("%d searches, want 2: the walk ends once the listed total is collected", len(f.searches))
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
		t.Error("a first page with no posting must fail, not read as an empty market")
	}
	second := &kariyernetFake{pages: map[int]string{1: kariyernetPage(9, kariyernetRow(1, "a", "Acme", false))}}
	if _, err := NewKariyerNet(second).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a later page failing must fail the crawl")
	}
	var se *StatusError
	failing := kariyernetFailingAPI{err: &StatusError{Method: "POST", Code: 403, URL: kariyernetSearchURL}}
	if _, err := NewKariyerNet(failing).Fetch(context.Background(), CompanyEntry{}); !errors.As(err, &se) {
		t.Errorf("err = %v, want the search's status error to surface", err)
	}
}

type kariyernetFailingAPI struct{ err error }

func (f kariyernetFailingAPI) PostJSON(context.Context, string, any, any) error { return f.err }
func (f kariyernetFailingAPI) GetJSON(context.Context, string, any) error       { return f.err }

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
