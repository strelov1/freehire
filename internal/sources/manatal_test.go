package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestManatalProvider(t *testing.T) {
	if got := NewManatal(nil).Provider(); got != "manatal" {
		t.Errorf("Provider() = %q, want manatal", got)
	}
}

func TestManatalIsBoardBasedAndFilterable(t *testing.T) {
	s := NewManatal(nil)
	// Manatal is per-tenant (board = career-page slug), so it must NOT be boardless.
	if _, ok := s.(boardless); ok {
		t.Error("manatal must not implement the boardless marker (it is board-based)")
	}
	if _, ok := All(nil)["manatal"]; !ok {
		t.Error("All() should register provider manatal")
	}
	if !slices.Contains(FilterableProviders(), "manatal") {
		t.Error("FilterableProviders() should include manatal")
	}
}

func TestManatalBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/manatal.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/manatal.yml fails validation: %v", err)
	}
}

func TestManatalFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"count":3,"next":"https://open.api.manatal.com/open/v3/career-page/acme/jobs/?page=2","previous":null,"results":[
{"id":1,"hash":"AB12CD34","organization_name":"Engineering","position_name":"Backend Engineer","description":"<p>Build &amp; ship.</p>","country":"Thailand","state":"Bangkok","city":"Bangkok","location_display":"Bangkok, Thailand","is_remote":true,"contract_details":"full_time"},
{"id":2,"hash":"","organization_name":"Sales","position_name":"skip me","description":"x","location_display":"Remote","is_remote":null,"contract_details":"internship"}
]}`
	// page2 carries a null next, terminating the walk.
	page2 := `{"count":3,"next":null,"previous":"x","results":[
{"id":3,"hash":"ZZ99","position_name":"QA Intern","description":"<p>Test.</p>","country":"","state":"","city":"","location_display":"Remote","is_remote":null,"contract_details":"internship"}
]}`
	// page=2 routed first so it wins over the base career-page route (routedHTTP is first-match).
	fake := (&routedHTTP{}).route("page=2", page2).route("career-page/acme", page1)

	jobs, err := NewManatal(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the hashless posting dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "AB12CD34" {
		t.Errorf("ExternalID = %q, want the posting hash AB12CD34", j.ExternalID)
	}
	// URL is the public career-page posting page, keyed by tenant slug + hash.
	if j.URL != "https://www.careers-page.com/acme/job/AB12CD34" {
		t.Errorf("URL = %q, want careers-page.com/acme/job/AB12CD34", j.URL)
	}
	// Company is the configured tenant, never organization_name (a department).
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want the configured Acme (not organization_name)", j.Company)
	}
	if j.Location != "Bangkok, Thailand" {
		t.Errorf("Location = %q, want location_display", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true from is_remote", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (the API carries no publish date)", j.PostedAt)
	}

	// The second-page posting: no structured remote flag, but location text is "Remote" so the
	// heuristic still flags it; WorkMode stays empty (structured signal only).
	q := jobs[1]
	if q.ExternalID != "ZZ99" || q.Title != "QA Intern" {
		t.Errorf("bad page-2 mapping: %+v", q)
	}
	if !q.Remote {
		t.Error("page-2 job should be Remote via the location heuristic")
	}
	if q.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (is_remote unset → no structured signal)", q.WorkMode)
	}
	if q.EmploymentType != "internship" {
		t.Errorf("EmploymentType = %q, want internship", q.EmploymentType)
	}
}

func TestManatalEmploymentType(t *testing.T) {
	cases := map[string]string{
		"full_time":  "full_time",
		"part_time":  "part_time",
		"contract":   "contract",
		"temporary":  "contract",
		"freelance":  "contract",
		"internship": "internship",
		"":           "",
		"weird":      "",
	}
	for in, want := range cases {
		if got := manatalEmploymentType(in); got != want {
			t.Errorf("manatalEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
