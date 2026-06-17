package sources

import (
	"context"
	"slices"
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
	resp := `{"hits":[
{"id":96853,"title":"Founding Account Executive","description":"## About\n\nSell **stuff**.","remote":"only","created_at":"2026-06-17T17:44:11.932Z","company_name":"Ergo","locations_for_search":["San Francisco, CA, US","San Francisco","CA","US"],"search_path":"https://www.ycombinator.com/companies/ergo/jobs/VDySCKB-founding-account-executive"},
{"id":0,"title":"NoID","company_name":"x"},
{"id":12,"title":"NoCompany","company_name":"","remote":"no"}
],"nbHits":2,"nbPages":1}`
	fake := (&routedHTTP{}).route("algolia.net", resp)

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
	resp := `{"hits":[{"id":555,"title":"Role","company_name":"Acme","remote":"no"}],"nbHits":1,"nbPages":1}`
	fake := (&routedHTTP{}).route("algolia.net", resp)
	jobs, _ := NewWorkAtAStartup(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 || jobs[0].URL != "https://www.workatastartup.com/jobs/555" {
		t.Fatalf("URL = %q, want id-built fallback", jobs[0].URL)
	}
}
