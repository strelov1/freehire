package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// getroFake serves the POST search listing by page, the collection metadata (label), and the SSR
// detail HTML routed by the job slug in the URL.
type getroFake struct {
	pages      map[int]string    // page number -> search response JSON
	label      string            // collection metadata label (board subdomain)
	labelErr   bool              // metadata lookup fails
	detailHTML map[string]string // job slug -> detail page HTML
	detailErr  map[string]bool   // job slug -> detail fetch fails
	detailURLs []string          // detail URLs fetched, for asserting new-only hydration
}

func (f *getroFake) PostJSON(_ context.Context, _ string, body, v any) error {
	page := body.(map[string]int)["page"]
	return json.Unmarshal([]byte(f.pages[page]), v)
}

func (f *getroFake) GetJSON(_ context.Context, _ string, v any) error {
	if f.labelErr {
		return errors.New("meta boom")
	}
	return json.Unmarshal([]byte(fmt.Sprintf(`{"data":{"attributes":{"label":%q}}}`, f.label)), v)
}

func (f *getroFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.detailURLs = append(f.detailURLs, url)
	slug := url[strings.LastIndex(url, "/")+1:]
	if f.detailErr[slug] {
		return nil, errors.New("detail boom")
	}
	return html.Parse(strings.NewReader(f.detailHTML[slug]))
}

// getroDetailPage builds a minimal SSR detail page carrying the description in __NEXT_DATA__.
func getroDetailPage(desc string) string {
	payload, _ := json.Marshal(map[string]any{
		"props": map[string]any{"pageProps": map[string]any{
			"initialState": map[string]any{"jobs": map[string]any{
				"currentJob": map[string]any{"description": desc},
			}},
		}},
	})
	return `<html><body><script id="__NEXT_DATA__" type="application/json">` + string(payload) + `</script></body></html>`
}

// 1700000000 is a safely-past epoch (2023-11-14) so NotFuture never drops it regardless of the
// test clock.
const getroPostedEpoch = 1700000000

var getroListingJSON = fmt.Sprintf(`{"results":{"count":3,"jobs":[
  {"id":101,"slug":"101-desktop-analyst","title":"Desktop Analyst","url":"https://boards.greenhouse.io/acme/jobs/1","work_mode":"on_site","seniority":"internship","skills":["Python","Widgets"],"locations":["Zürich, Switzerland"],"created_at":%d,"organization":{"name":"Acme","slug":"acme-1"}},
  {"id":102,"slug":"102-backend","title":"Backend Engineer","url":"https://jobs.lever.co/beta/2","work_mode":"remote","seniority":"senior","skills":[],"locations":["Anywhere"],"created_at":%d,"organization":{"name":"Beta","slug":"beta-2"}},
  {"id":103,"slug":"103-drop","title":"No Company","url":"https://x/3","work_mode":"on_site","seniority":"director","skills":[],"locations":[],"created_at":%d,"organization":{"name":"","slug":""}}
]}}`, getroPostedEpoch, getroPostedEpoch, getroPostedEpoch)

func TestGetroFetchMapsAndDrops(t *testing.T) {
	fake := &getroFake{pages: map[int]string{0: getroListingJSON}}
	jobs, err := NewGetro(fake).Fetch(context.Background(), CompanyEntry{Board: "15272"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (no-company dropped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j := byID["101"]
	if j.URL != "https://boards.greenhouse.io/acme/jobs/1" {
		t.Errorf("URL = %q, want the first-party ATS link", j.URL)
	}
	if j.Title != "Desktop Analyst" || j.Company != "Acme" {
		t.Errorf("title/company: %q / %q", j.Title, j.Company)
	}
	if j.Location != "Zürich, Switzerland" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (on_site normalized)", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true for a Zürich onsite job, want false")
	}
	if j.Seniority != "intern" {
		t.Errorf("Seniority = %q, want intern (internship->intern)", j.Seniority)
	}
	if !slices.Contains(j.Skills, "python") {
		t.Errorf("Skills = %v, want the canonicalized python", j.Skills)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2023 {
		t.Errorf("PostedAt = %v, want 2023 (epoch seconds)", j.PostedAt)
	}
	r := byID["102"]
	if !r.Remote || r.WorkMode != "remote" || r.Seniority != "senior" {
		t.Errorf("remote job: Remote=%v WorkMode=%q Seniority=%q", r.Remote, r.WorkMode, r.Seniority)
	}
}

func TestGetroListPaginates(t *testing.T) {
	page0 := fmt.Sprintf(`{"results":{"count":3,"jobs":[
	  {"id":1,"slug":"1-a","title":"A","url":"https://x/1","organization":{"name":"Co"},"created_at":%d},
	  {"id":2,"slug":"2-b","title":"B","url":"https://x/2","organization":{"name":"Co"},"created_at":%d}
	]}}`, getroPostedEpoch, getroPostedEpoch)
	page1 := fmt.Sprintf(`{"results":{"count":3,"jobs":[
	  {"id":3,"slug":"3-c","title":"C","url":"https://x/3","organization":{"name":"Co"},"created_at":%d}
	]}}`, getroPostedEpoch)
	fake := &getroFake{pages: map[int]string{0: page0, 1: page1}}
	jobs, err := NewGetro(fake).Fetch(context.Background(), CompanyEntry{Board: "15272"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two pages", len(jobs))
	}
}

func TestGetroFetchNewHydratesNewOnly(t *testing.T) {
	listing := fmt.Sprintf(`{"results":{"count":3,"jobs":[
	  {"id":201,"slug":"201-seen","title":"Seen","url":"https://x/201","organization":{"name":"Co","slug":"co"},"created_at":%d},
	  {"id":202,"slug":"202-new","title":"New","url":"https://x/202","organization":{"name":"Co","slug":"co"},"created_at":%d},
	  {"id":203,"slug":"203-err","title":"Err","url":"https://x/203","organization":{"name":"Co","slug":"co"},"created_at":%d}
	]}}`, getroPostedEpoch, getroPostedEpoch, getroPostedEpoch)
	fake := &getroFake{
		pages:      map[int]string{0: listing},
		label:      "jobsinvc",
		detailHTML: map[string]string{"202-new": getroDetailPage("<p>Great role</p>")},
		detailErr:  map[string]bool{"203-err": true},
	}
	seen := func(id string) bool { return id == "201" }
	jobs, err := NewGetro(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "15272"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	// Detail is fetched only for the unseen postings (202, 203), never for the seen one (201).
	var slugs []string
	for _, u := range fake.detailURLs {
		slugs = append(slugs, u[strings.LastIndex(u, "/")+1:])
	}
	slices.Sort(slugs)
	if !slices.Equal(slugs, []string{"202-new", "203-err"}) {
		t.Errorf("detail slugs = %v, want [202-new 203-err] (201 not fetched)", slugs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["201"].SeenRefresh || byID["201"].Description != "" {
		t.Errorf("201: SeenRefresh=%v desc=%q, want refresh-only with empty desc", byID["201"].SeenRefresh, byID["201"].Description)
	}
	if d := byID["202"].Description; !strings.Contains(d, "Great role") {
		t.Errorf("202 description = %q, want the hydrated body", d)
	}
	if byID["203"].SeenRefresh || byID["203"].Description != "" {
		t.Errorf("203 should fall back to list-only (empty desc, not a refresh), got desc=%q refresh=%v", byID["203"].Description, byID["203"].SeenRefresh)
	}
}

// When the board subdomain cannot be resolved, FetchNew must degrade to list-only rather than
// failing the board: no detail is fetched and every posting is ingested list-only.
func TestGetroFetchNewDegradesWithoutLabel(t *testing.T) {
	listing := fmt.Sprintf(`{"results":{"count":1,"jobs":[
	  {"id":301,"slug":"301-x","title":"X","url":"https://x/301","organization":{"name":"Co"},"created_at":%d}
	]}}`, getroPostedEpoch)
	fake := &getroFake{pages: map[int]string{0: listing}, labelErr: true}
	seen := func(string) bool { return false }
	jobs, err := NewGetro(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "15272"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "" {
		t.Fatalf("jobs = %+v, want one list-only job with empty description", jobs)
	}
	if len(fake.detailURLs) != 0 {
		t.Errorf("detailURLs = %v, want none fetched without a label", fake.detailURLs)
	}
}

func TestGetroWorkMode(t *testing.T) {
	cases := map[string]string{
		"on_site": "onsite",
		"remote":  "remote",
		"hybrid":  "hybrid",
		"":        "",
		"unknown": "",
	}
	for in, want := range cases {
		if got := getroWorkMode(in); got != want {
			t.Errorf("getroWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetroSeniority(t *testing.T) {
	cases := map[string]string{
		"internship":  "intern",
		"entry_level": "junior",
		"senior":      "senior",
		"cxo":         "c_level",
		"":            "",
	}
	for in, want := range cases {
		if got := getroSeniority(in); got != want {
			t.Errorf("getroSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetroProviderRegistered(t *testing.T) {
	s := NewGetro(nil)
	if s.Provider() != "getro" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); ok {
		t.Error("getro is board-based (network id), it must NOT be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("getro should be an aggregator")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("getro should implement HydratingSource")
	}
	if _, ok := All(nil)["getro"]; !ok {
		t.Error("All() should register getro")
	}
	if !slices.Contains(FilterableProviders(), "getro") {
		t.Error("FilterableProviders() should include getro")
	}
}

func TestGetroBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/getro.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/getro.yml fails validation: %v", err)
	}
}
