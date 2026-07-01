package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWorkAtAStartupProvider(t *testing.T) {
	if got := NewWorkAtAStartup(nil).Provider(); got != "workatastartup" {
		t.Errorf("Provider() = %q, want workatastartup", got)
	}
}

func TestWorkAtAStartupIsBoardlessAggregator(t *testing.T) {
	s := NewWorkAtAStartup(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("workatastartup should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("workatastartup should implement the aggregator marker")
	}
}

func TestWorkAtAStartupRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["workatastartup"]; !ok {
		t.Error("All() should register provider workatastartup")
	}
	if !slices.Contains(FilterableProviders(), "workatastartup") {
		t.Error("FilterableProviders() should include workatastartup")
	}
}

func TestWorkAtAStartupBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/workatastartup.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/workatastartup.yml fails validation: %v", err)
	}
}

func TestWorkAtAStartupMissingKeyErrors(t *testing.T) {
	t.Setenv(waasKeyEnv, "")
	_, err := NewWorkAtAStartup(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("Fetch should error when WAAS_ALGOLIA_KEY is unset")
	}
}

func TestWorkAtAStartupWorkMode(t *testing.T) {
	cases := map[string]string{"only": "remote", "yes": "remote", "no": "onsite", "": ""}
	for in, want := range cases {
		if got := waasWorkMode(in); got != want {
			t.Errorf("waasWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkAtAStartupFetchMapsHits(t *testing.T) {
	t.Setenv(waasKeyEnv, "test-key")
	fake := newWaasIndexFake(
		`{"id":96853,"title":"Founding Account Executive","description":"## About\n\nSell **stuff**.","remote":"only","created_at":"2026-06-17T17:44:11.932Z","company_name":"Ergo","locations_for_search":["San Francisco, CA, US","San Francisco","CA","US"],"search_path":"https://www.ycombinator.com/companies/ergo/jobs/VDySCKB-founding-account-executive"}`,
		`{"id":0,"title":"NoID","company_name":"x"}`,
		`{"id":12,"title":"NoCompany","company_name":"","remote":"no"}`,
	)

	jobs, err := NewWorkAtAStartup(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (zero-id and no-company dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "96853" || j.Company != "Ergo" || j.Title != "Founding Account Executive" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://www.ycombinator.com/companies/ergo/jobs/VDySCKB-founding-account-executive" {
		t.Errorf("URL should use search_path: %q", j.URL)
	}
	if j.Location != "San Francisco, CA, US" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	if !strings.Contains(j.Description, "<h2") || !strings.Contains(j.Description, "<strong>stuff</strong>") {
		t.Errorf("Description should be markdown-rendered HTML: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 17, 17, 44, 11, 932000000, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestWorkAtAStartupURLFallsBackToId(t *testing.T) {
	t.Setenv(waasKeyEnv, "test-key")
	fake := newWaasIndexFake(`{"id":555,"title":"Role","company_name":"Acme","remote":"no"}`)
	jobs, _ := NewWorkAtAStartup(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 || jobs[0].URL != "https://www.workatastartup.com/jobs/555" {
		t.Fatalf("URL = %q, want id-built fallback", jobs[0].URL)
	}
}

// TestWorkAtAStartupToleratesNestedLocations locks in a real-index quirk: some records store
// nested-array garbage in locations_for_search (e.g. [[[["Remote"]]], "Remote"]) instead of a
// flat []string. The adapter must skip the nested elements and take the first scalar.
func TestWorkAtAStartupToleratesNestedLocations(t *testing.T) {
	t.Setenv(waasKeyEnv, "test-key")
	fake := newWaasIndexFake(`{"id":64767,"title":"Eng","company_name":"Acme","remote":"only","locations_for_search":[[[[[["Remote"]]]]],"Remote"]}`)
	jobs, err := NewWorkAtAStartup(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Location != "Remote" {
		t.Errorf("Location = %q, want %q (first scalar past the nested garbage)", jobs[0].Location, "Remote")
	}
}

// TestWorkAtAStartupBisectsPastPaginationCap is the core coverage test: the index holds far
// more than one page (waasHitsPerPage), so a single sweep would silently truncate at the cap.
// The adapter must bisect the id space and return every posting, including high-id ones that a
// plain first-page query never reaches.
func TestWorkAtAStartupBisectsPastPaginationCap(t *testing.T) {
	t.Setenv(waasKeyEnv, "test-key")

	// 2500 postings (ids 1..2500) spread across the id space, > 2 pages at the 1000 cap.
	var hitJSONs []string
	for id := 1; id <= 2500; id++ {
		hitJSONs = append(hitJSONs, fmt.Sprintf(`{"id":%d,"company_name":"Co%d","title":"Role","remote":"no"}`, id, id))
	}
	// One deliberately high id, well past any single page, standing in for the posting the
	// old single-sweep adapter dropped past the cap.
	const highID = 999001
	hitJSONs = append(hitJSONs, fmt.Sprintf(`{"id":%d,"company_name":"GoGoGrandparent","title":"Backend Engineer","remote":"only"}`, highID))
	fake := newWaasIndexFake(hitJSONs...)

	jobs, err := NewWorkAtAStartup(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2501 {
		t.Fatalf("got %d jobs, want 2501 — bisection must reach every posting past the cap", len(jobs))
	}
	ids := make(map[string]bool, len(jobs))
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	if !ids[strconv.Itoa(highID)] {
		t.Errorf("high-id posting %d was not fetched — bisection failed to reach it", highID)
	}
	// No leaf window ever exceeded the retrieval cap.
	if fake.maxPageReturned > waasHitsPerPage {
		t.Errorf("a query returned %d hits, above the %d cap", fake.maxPageReturned, waasHitsPerPage)
	}
}

// waasIndexFake models the Algolia index for tests: it filters its dataset by the half-open id
// window carried in numericFilters and caps each response at the requested hitsPerPage, mirroring
// Algolia's page limit. Bodies are the exact JSON hit strings, so the adapter's real decode path
// (markdown, locations, search_path) is exercised.
type waasIndexFake struct {
	hits []struct { // parsed id + raw JSON, in ascending id order
		id  int64
		raw string
	}
	maxPageReturned int
}

func newWaasIndexFake(hitJSONs ...string) *waasIndexFake {
	f := &waasIndexFake{}
	for _, raw := range hitJSONs {
		var h struct {
			ID json.Number `json:"id"`
		}
		if err := json.Unmarshal([]byte(raw), &h); err != nil {
			panic("waasIndexFake: bad hit JSON: " + err.Error())
		}
		id, _ := h.ID.Int64()
		f.hits = append(f.hits, struct {
			id  int64
			raw string
		}{id, raw})
	}
	slices.SortFunc(f.hits, func(a, b struct {
		id  int64
		raw string
	}) int {
		return int(a.id - b.id)
	})
	return f
}

func (f *waasIndexFake) PostJSONWithHeaders(_ context.Context, _ string, _ map[string]string, body, v any) error {
	m, _ := body.(map[string]any)
	params, _ := m["params"].(string)
	q, err := parseWaasParams(params)
	if err != nil {
		return err
	}
	var matched []string
	for _, h := range f.hits {
		if h.id >= q.lo && h.id < q.hi {
			matched = append(matched, h.raw)
		}
	}
	if q.hitsPerPage < len(matched) {
		matched = matched[:q.hitsPerPage]
	}
	if len(matched) > f.maxPageReturned {
		f.maxPageReturned = len(matched)
	}
	resp := fmt.Sprintf(`{"hits":[%s]}`, strings.Join(matched, ","))
	return json.Unmarshal([]byte(resp), v)
}

type waasQuery struct {
	lo, hi      int64
	hitsPerPage int
}

func parseWaasParams(params string) (waasQuery, error) {
	q := waasQuery{lo: math.MinInt64, hi: math.MaxInt64}
	for _, kv := range strings.Split(params, "&") {
		key, val, _ := strings.Cut(kv, "=")
		val, err := neturl.QueryUnescape(val)
		if err != nil {
			return q, err
		}
		switch key {
		case "hitsPerPage":
			q.hitsPerPage, _ = strconv.Atoi(val)
		case "numericFilters":
			var clauses []string
			if err := json.Unmarshal([]byte(val), &clauses); err != nil {
				return q, fmt.Errorf("bad numericFilters %q: %w", val, err)
			}
			for _, c := range clauses {
				if v, ok := strings.CutPrefix(c, "id>="); ok {
					q.lo, _ = strconv.ParseInt(v, 10, 64)
				} else if v, ok := strings.CutPrefix(c, "id<"); ok {
					q.hi, _ = strconv.ParseInt(v, 10, 64)
				}
			}
		}
	}
	return q, nil
}
