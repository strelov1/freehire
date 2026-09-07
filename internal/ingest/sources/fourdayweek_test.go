package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// fourDayWeekFake serves the v2 list pages the tests wire; an unrouted URL is an error, so a
// test that asks for a page it did not set up fails loudly rather than silently ending the walk.
type fourDayWeekFake struct{ routes map[string]string }

func (f *fourDayWeekFake) GetJSON(_ context.Context, url string, v any) error {
	body, ok := f.routes[url]
	if !ok {
		return fmt.Errorf("fourDayWeekFake: no route for %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

func TestFourDayWeekProvider(t *testing.T) {
	if got := NewFourDayWeek(nil).Provider(); got != "4dayweek" {
		t.Errorf("Provider() = %q, want 4dayweek", got)
	}
}

func TestFourDayWeekIsBoardlessAggregator(t *testing.T) {
	s := NewFourDayWeek(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("4dayweek should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("4dayweek should implement the aggregator marker")
	}
}

func TestFourDayWeekRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["4dayweek"]; !ok {
		t.Error("All() should register provider 4dayweek")
	}
	if !slices.Contains(FilterableProviders(), "4dayweek") {
		t.Error("FilterableProviders() should include 4dayweek")
	}
}

// v2 carries the body and the canonical URL inline, so one list call is the whole crawl —
// there is no detail pass left to test, and no Pro-lock to work around.
func TestFourDayWeekFetchReadsEverythingFromTheList(t *testing.T) {
	page1 := `{"data":[
{"id":"abc-1","slug":"senior-backend-at-acme-1","title":"Senior Backend Engineer","company":{"name":"Acme"},"work_arrangement":"remote","level":"senior","category":"devops","posted_at":"2026-08-01T10:00:00Z","url":"https://4dayweek.io/job/senior-backend-at-acme-1","description":"<p>Great role &amp; team.</p>","locations":[{"city":"Berlin","country":"Germany","is_primary":true}],"stack":[{"name":"Go"},{"name":"Kubernetes"}]},
{"id":"","slug":"","title":"skip me","company":{"name":"NoID"}}
],"has_more":true}`
	page2 := `{"data":[],"has_more":false}`
	http := &fourDayWeekFake{routes: map[string]string{
		"https://4dayweek.io/api/v2/jobs?page=1&limit=100": page1,
		"https://4dayweek.io/api/v2/jobs?page=2&limit=100": page2,
	}}

	jobs, err := NewFourDayWeek(http).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job (the id-less posting is dropped), got %d: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "abc-1" || j.Title != "Senior Backend Engineer" || j.Company != "Acme" {
		t.Errorf("identity = %q/%q/%q", j.ExternalID, j.Title, j.Company)
	}
	if !strings.Contains(j.Description, "Great role") {
		t.Errorf("description came from the list, want the inline body, got %q", j.Description)
	}
	if j.URL != "https://4dayweek.io/job/senior-backend-at-acme-1" {
		t.Errorf("URL = %q, want the inline canonical url", j.URL)
	}
	if j.Location != "Berlin, Germany" || j.WorkMode != "remote" || j.Seniority != "senior" {
		t.Errorf("facets = %q/%q/%q", j.Location, j.WorkMode, j.Seniority)
	}
	if !slices.Contains(j.Skills, "go") {
		t.Errorf("skills = %v, want the stack canonicalised", j.Skills)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt is nil; posted_at is an RFC3339 string in v2, not an epoch")
	}
}

// The URL falls back to the slug for a posting that omits it, so a missing field costs a link
// rather than the whole posting.
func TestFourDayWeekFallsBackToTheSlugURL(t *testing.T) {
	page1 := `{"data":[{"id":"x1","slug":"role-at-acme","title":"Dev","company":{"name":"Acme"},"description":"<p>Body.</p>"}],"has_more":false}`
	http := &fourDayWeekFake{routes: map[string]string{
		"https://4dayweek.io/api/v2/jobs?page=1&limit=100": page1,
	}}

	jobs, err := NewFourDayWeek(http).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://4dayweek.io/job/role-at-acme" {
		t.Fatalf("want the slug-built url, got %+v", jobs)
	}
}

// The crawl must read the path robots.txt allows. /api/jobs is under that file's
// "Disallow: /api/" and is what the site began refusing in 2026-08.
func TestFourDayWeekReadsTheRobotsAllowedPath(t *testing.T) {
	if !strings.Contains(fourDayWeekListURL, "/api/v2/") {
		t.Errorf("list URL = %q, want the robots-allowed /api/v2 path", fourDayWeekListURL)
	}
}

func TestFourDayWeekSeniorityMapping(t *testing.T) {
	cases := map[string]string{
		"entry":     "junior",
		"mid":       "middle",
		"senior":    "senior",
		"lead":      "lead",
		"executive": "c_level",
		"":          "",
		"nonsense":  "",
	}
	for in, want := range cases {
		if got := fourDayWeekSeniority(in); got != want {
			t.Errorf("fourDayWeekSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFourDayWeekCategoryMapping(t *testing.T) {
	cases := map[string]string{
		"devops":    "devops",
		"security":  "security",
		"product":   "product",
		"design":    "design",
		"sales":     "sales",
		"marketing": "marketing",
		// Generic or unmapped 4dayweek categories stay empty so the title dictionary decides.
		"engineering":      "",
		"data":             "",
		"operations":       "",
		"customer-success": "",
	}
	for in, want := range cases {
		if got := fourDayWeekCategory(in); got != want {
			t.Errorf("fourDayWeekCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFourDayWeekWorkModeMapping(t *testing.T) {
	cases := map[string]string{
		"remote": "remote",
		"hybrid": "hybrid",
		"onsite": "onsite",
		"":       "",
		"weird":  "",
	}
	for in, want := range cases {
		if got := fourDayWeekWorkMode(in); got != want {
			t.Errorf("fourDayWeekWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}
