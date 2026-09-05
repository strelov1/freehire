package sources

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// jobleadsFake is a test jobleadsHTTP: it answers each search page with the next canned
// response in order, serves one detail body per posting id (matched on the URL), and
// records every search request so tests can assert the request shape. failPage makes that
// page's search fail, exercising the walk's page-1-vs-later split; detailCalls counts the
// detail requests issued.
type jobleadsFake struct {
	pages       []string
	details     map[string]string
	failPage    int
	reqs        []jobleadsSearchReq
	detailCalls int
}

func (f *jobleadsFake) PostJSON(_ context.Context, _ string, body, v any) error {
	req, ok := body.(jobleadsSearchReq)
	if !ok {
		return errors.New("fake: body not a jobleadsSearchReq")
	}
	f.reqs = append(f.reqs, req)
	if req.Page == f.failPage {
		return errors.New("fake: page failed")
	}
	if req.Page < 1 || req.Page > len(f.pages) {
		return errors.New("fake: page out of range")
	}
	return json.Unmarshal([]byte(f.pages[req.Page-1]), v)
}

func (f *jobleadsFake) GetJSON(_ context.Context, url string, v any) error {
	f.detailCalls++
	for id, body := range f.details {
		if strings.Contains(url, id) {
			return json.Unmarshal([]byte(body), v)
		}
	}
	return errors.New("fake: no detail for " + url)
}

// jobleadsSearchPage renders one search page response holding the given postings.
func jobleadsSearchPage(postings ...string) string {
	return `{"totalResultCount":` + strconv.Itoa(len(postings)) + `,"preciseResultCount":true,"currentResultCount":` +
		strconv.Itoa(len(postings)) + `,"page":1,"limit":100,"jobResults":[` + strings.Join(postings, ",") + `]}`
}

const (
	// The feed ids deliberately differ from the slug hashes, exactly as live: the catalog
	// id is a variant-scoped handle, the detail key is the slug hash.
	jobleadsP1 = `{"id":"external-4964a963929486ed633123177efa3b2f","companyName":"DotEnv S.r.L.","jobTitle":"Frontend Developer (Mid-Senior)","jobLink":"/it/job/frontend-developer-mid-senior--ferrara--ecde1148d365affee309f0df8bb2812a8","cityName":["Ferrara"],"regionName":["Emilia-Romagna"],"alpha2Country":"IT","validFrom":1767319685,"jobLocationType":["hybrid"],"contractType":["full_time"],"jobDescription":null}`
	jobleadsP2 = `{"id":"external-1d395e48ce14e05faf2a654272b97a96","companyName":"HNRG","jobTitle":"React Developer","jobLink":"/it/job/react-developer--limena--e6a7e4e033a20bc1da22e2a6bc665ee03","cityName":["Limena"],"regionName":["Veneto"],"alpha2Country":"IT","validFrom":1786425092,"jobLocationType":["in_person"],"contractType":["full_time"],"jobDescription":null}`
	jobleadsP3 = `{"id":"external-bad","companyName":"","jobTitle":"no company drops the posting","jobLink":"/it/job/x"}`
)

func TestJobleadsProvider(t *testing.T) {
	if got := NewJobleads(nil).Provider(); got != "jobleads" {
		t.Errorf("Provider() = %q, want jobleads", got)
	}
}

func TestJobleadsMarkers(t *testing.T) {
	s := NewJobleads(nil)
	if _, ok := s.(aggregator); !ok {
		t.Error("jobleads should implement the aggregator marker")
	}
	if _, ok := s.(boardless); ok {
		t.Error("jobleads must NOT be boardless: the board is the search keyword")
	}
	g, ok := s.(sweepGrace)
	if !ok {
		t.Fatal("jobleads should implement the sweepGrace marker")
	}
	if want := 14 * 24 * time.Hour; g.sweepGrace() != want {
		t.Errorf("sweepGrace() = %v, want %v (volatile vdb window)", g.sweepGrace(), want)
	}
}

func TestJobleadsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["jobleads"]; !ok {
		t.Error("All() should register provider jobleads")
	}
	if !slices.Contains(FilterableProviders(), "jobleads") {
		t.Error("FilterableProviders() should include jobleads")
	}
	if !slices.Contains(AggregatorProviders(All(nil)), "jobleads") {
		t.Error("AggregatorProviders() should include jobleads — its copies must be suppressed in the cross-source dedup")
	}
}

func TestJobleadsFetchMapsAndWalks(t *testing.T) {
	fake := &jobleadsFake{
		pages: []string{
			jobleadsSearchPage(jobleadsP1, jobleadsP2, jobleadsP3),
			jobleadsSearchPage(), // empty → the walk ends
		},
	}
	jobs, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "Frontend Developer"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (company-less posting dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "external-4964a963929486ed633123177efa3b2f" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://www.jobleads.com/it/job/frontend-developer-mid-senior--ferrara--ecde1148d365affee309f0df8bb2812a8" {
		t.Errorf("URL = %q, want absolute join of jobLink", j.URL)
	}
	if j.Company != "DotEnv S.r.L." || j.Title != "Frontend Developer (Mid-Senior)" {
		t.Errorf("Company/Title = %q / %q", j.Company, j.Title)
	}
	if j.Location != "Ferrara, Emilia-Romagna" {
		t.Errorf("Location = %q, want city + region", j.Location)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want hybrid/false (only fully-remote is Remote)", j.WorkMode, j.Remote)
	}
	if j.Countries == nil || j.Countries[0] != "it" {
		t.Errorf("Countries = %v, want [it] normalized", j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Unix() != 1767319685 {
		t.Errorf("PostedAt = %v, want unix 1767319685 parsed", j.PostedAt)
	}
	if j.Description != "" {
		t.Error("Description set on the list-only feed, want empty until hydration")
	}
	if jobs[1].WorkMode != "onsite" || jobs[1].Remote {
		t.Errorf("second posting: WorkMode/Remote = %q/%v, want onsite/false", jobs[1].WorkMode, jobs[1].Remote)
	}
}

func TestJobleadsFetchRequestShape(t *testing.T) {
	fake := &jobleadsFake{pages: []string{jobleadsSearchPage(jobleadsP1), jobleadsSearchPage()}}
	_, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "Frontend Developer", Region: "IT"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	req := fake.reqs[0]
	if len(req.Keywords) != 1 || req.Keywords[0] != "Frontend Developer" {
		t.Errorf("Keywords = %v, want the board as the keyword", req.Keywords)
	}
	if req.Limit != 100 || req.Page != 1 || req.EngineOptions.EngineType != "vdbSearch" {
		t.Errorf("Limit/Page/Engine = %d/%d/%q", req.Limit, req.Page, req.EngineOptions.EngineType)
	}
	if len(req.Filters) != 1 {
		t.Fatalf("Filters = %v, want the country filter", req.Filters)
	}
	f := req.Filters[0]
	if f.Key != "location" || f.Operator != "eq" {
		t.Errorf("filter key/operator = %q/%q", f.Key, f.Operator)
	}
	if !strings.Contains(toJSON(t, f.Value), `"alpha2Country":"IT"`) {
		t.Errorf("filter value = %v, want alpha2Country IT", f.Value)
	}
}

func TestJobleadsFetchNoRegionSendsNoFilter(t *testing.T) {
	fake := &jobleadsFake{pages: []string{jobleadsSearchPage(), jobleadsSearchPage()}}
	if _, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "React Developer"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := fake.reqs[0]; len(got.Filters) != 0 {
		t.Errorf("Filters = %v, want none without a region", got.Filters)
	}
}

func TestJobleadsFetchFirstPageFailureFailsBoard(t *testing.T) {
	fake := &jobleadsFake{failPage: 1}
	if _, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "x"}); err == nil {
		t.Error("Fetch succeeded, want a board-level error when page 1 fails")
	}
}

func TestJobleadsFetchLaterPageFailureEndsWalk(t *testing.T) {
	fake := &jobleadsFake{pages: []string{jobleadsSearchPage(jobleadsP1)}, failPage: 2}
	jobs, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "x"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want the page-1 links despite the page-2 failure", len(jobs))
	}
}

func TestJobleadsDetailID(t *testing.T) {
	cases := map[string]string{
		"/it/job/frontend-developer-mid-senior--ferrara--ecde1148d365affee309f0df8bb2812a8": "ecde1148d365affee309f0df8bb2812a8",
		"/it/job/react-developer--limena--e6a7e4e033a20bc1da22e2a6bc665ee03":                "e6a7e4e033a20bc1da22e2a6bc665ee03",
		"/it/job/short-hex--ecde1148d365a":                                                  "",
		"/it/job/not-hex--zzde1148d365affee309f0df8bb2812a8":                                "",
		"/it/job/no-hash-at-all":                                                            "",
		"":                                                                                  "",
	}
	for link, want := range cases {
		if got := jobleadsDetailID(link); got != want {
			t.Errorf("jobleadsDetailID(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestJobleadsFetchNewHydratesOnlyUnseen(t *testing.T) {
	// The posting's feed id (external-4964...) differs from its slug hash (ecde1148...),
	// exactly as in the live sample: the detail call must be keyed on the slug hash, never
	// on the feed id — TestJobleadsDetailID pins the extraction, this test pins the wiring.
	fake := &jobleadsFake{
		pages: []string{jobleadsSearchPage(jobleadsP1, jobleadsP2)},
		details: map[string]string{
			"e6a7e4e033a20bc1da22e2a6bc665ee03": `{"status":"OK","payload":{"type":"JSON","content":{
"id":"e6a7e4e033a20bc1da22e2a6bc665ee03",
"jobSummary":"<p>HNRG cerca uno sviluppatore React.<\/p>",
"responsibilities":["Costruire SPA moderne."],
"qualifications":["React avanzato."],
"benefits":["Progetti internazionali"]
}}}`,
		},
	}
	jobs, err := NewJobleads(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "React Developer"},
		func(externalID string) bool { return externalID == jobleadsP1ID })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	seenJob, newJob := jobs[0], jobs[1]
	if !seenJob.SeenRefresh {
		t.Error("already-ingested posting should be a SeenRefresh touch, not a content rewrite")
	}
	if seenJob.Description != "" {
		t.Error("SeenRefresh posting carries a description, want none")
	}
	if newJob.SeenRefresh {
		t.Error("new posting marked SeenRefresh")
	}
	// Sanitize keeps safe HTML structure (the catalogue stores markup-rich bodies), so the
	// jobSummary's <p> survives; the plain-text lists are joined as-is.
	want := "<p>HNRG cerca uno sviluppatore React.</p>\n\nCostruire SPA moderne.\n\nReact avanzato.\n\nProgetti internazionali"
	if newJob.Description != want {
		t.Errorf("Description = %q, want the sanitized assembly %q", newJob.Description, want)
	}
}

func TestJobleadsFetchNewDetailFailureKeepsListOnly(t *testing.T) {
	fake := &jobleadsFake{
		pages: []string{jobleadsSearchPage(jobleadsP2)},
		// no detail route → every fetchDetail fails
	}
	jobs, err := NewJobleads(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "x"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want the posting kept list-only", len(jobs))
	}
	if jobs[0].Description != "" {
		t.Error("failed detail should leave the description empty, not fabricate one")
	}
}

// jobleadsP1ID is the ExternalID of jobleadsP1.
const jobleadsP1ID = "external-4964a963929486ed633123177efa3b2f"

// TestJobleadsMaxPagesCapsWalk pins the safety bound: an edge that never answers an empty
// page (every page full) must still terminate the walk at jobleadsMaxPages instead of
// looping forever — the failure adp.go's MaxPages exists for.
func TestJobleadsMaxPagesCapsWalk(t *testing.T) {
	pages := make([]string, jobleadsMaxPages+10)
	for i := range pages {
		pages[i] = jobleadsSearchPage(jobleadsP1)
	}
	fake := &jobleadsFake{pages: pages}
	jobs, err := NewJobleads(fake).Fetch(context.Background(), CompanyEntry{Board: "x"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fake.reqs) != jobleadsMaxPages {
		t.Errorf("walked %d pages, want the cap at %d", len(fake.reqs), jobleadsMaxPages)
	}
	if len(jobs) != jobleadsMaxPages {
		t.Errorf("got %d jobs, want %d (one per capped page)", len(jobs), jobleadsMaxPages)
	}
}

// TestJobleadsFetchNewSkipsDetailWithoutHash pins the doc's promise: a posting whose link
// carries no extractable hash is kept list-only WITHOUT issuing a detail request.
func TestJobleadsFetchNewSkipsDetailWithoutHash(t *testing.T) {
	noHash := `{"id":"external-nohash","companyName":"NoLink","jobTitle":"No Hash Role","jobLink":"/it/job/no-hash-at-all","cityName":["Roma"],"regionName":["Lazio"],"alpha2Country":"IT","validFrom":1767319685,"jobLocationType":["hybrid"],"jobDescription":null}`
	fake := &jobleadsFake{pages: []string{jobleadsSearchPage(noHash)}}
	jobs, err := NewJobleads(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "x"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if fake.detailCalls != 0 {
		t.Errorf("issued %d detail calls, want 0 (no extractable hash)", fake.detailCalls)
	}
	if jobs[0].Description != "" {
		t.Error("description set on a posting whose detail was never fetched")
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
