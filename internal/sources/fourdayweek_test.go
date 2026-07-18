package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

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

func TestFourDayWeekBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/4dayweek.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/4dayweek.yml fails validation: %v", err)
	}
}

func TestFourDayWeekFetchHydratesUnlockedDropsLocked(t *testing.T) {
	page1 := `{"jobs":[
{"id":"abc-1","slug":"senior-backend-at-acme-1","title":"Senior Backend Engineer","company_name":"Acme","work_arrangement":"remote","level":"senior","category":"devops","posted":1784307599,"locations":[{"city":"Berlin","country":"Germany","is_primary":true}],"stack":[{"name":"Go"},{"name":"Kubernetes"}]},
{"id":"locked-2","slug":"locked-role-2","title":"Locked Role","company_name":"Globex"},
{"id":"","slug":"","title":"skip me","company_name":"NoID"}
],"has_more":true}`
	page2 := `{"jobs":[],"has_more":false}`
	// A free posting renders its body in article.prose; a Pro-locked posting shows the unlock
	// notice and has no article.prose.
	unlocked := `<html><body><div class="relative"><article class="prose prose-slate"><h2>About</h2><p>Great role &amp; team.</p></article></div></body></html>`
	locked := `<html><body><div class="paywall"><p>Full description locked. Unlock with Pro.</p></div></body></html>`
	// Detail routes precede the base list route; none of their match strings occur in a list URL.
	fake := (&routedHTTP{}).
		route("page=2", page2).
		route("/job/senior-backend-at-acme-1", unlocked).
		route("/job/locked-role-2", locked).
		route("api/jobs", page1)

	jobs, err := NewFourDayWeek(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (locked and empty-id postings dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "abc-1" || j.Company != "Acme" || j.Title != "Senior Backend Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://4dayweek.io/job/senior-backend-at-acme-1" {
		t.Errorf("URL = %q, want the public job page from the slug", j.URL)
	}
	if !strings.Contains(j.Description, "Great role") || !strings.Contains(j.Description, "team") {
		t.Errorf("Description not hydrated from article.prose: %q", j.Description)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.Seniority != "senior" || j.Category != "devops" {
		t.Errorf("structured facets lost: seniority=%q category=%q", j.Seniority, j.Category)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q, want \"Berlin, Germany\"", j.Location)
	}
	if len(j.Skills) == 0 {
		t.Errorf("Skills empty, want the stack canonicalized through skilltag")
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch")
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
