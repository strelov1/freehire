package sources

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestUSAJobsProvider(t *testing.T) {
	if got := NewUSAJobs(nil, "k").Provider(); got != "usajobs" {
		t.Errorf("Provider() = %q, want usajobs", got)
	}
}

func TestUSAJobsIsBoardlessAggregator(t *testing.T) {
	s := NewUSAJobs(nil, "k")
	if _, ok := s.(boardless); !ok {
		t.Error("usajobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("usajobs should implement the aggregator marker")
	}
}

// USAJobs is the one keyed source: it is registered (and filterable) only when
// USAJOBS_API_KEY is set, so other crawls stay unaffected when it is unconfigured.
func TestUSAJobsRegisteredOnlyWhenKeySet(t *testing.T) {
	t.Setenv("USAJOBS_API_KEY", "")
	if _, ok := All(nil)["usajobs"]; ok {
		t.Error("All() should NOT register usajobs without USAJOBS_API_KEY")
	}

	t.Setenv("USAJOBS_API_KEY", "test-key")
	if _, ok := All(nil)["usajobs"]; !ok {
		t.Error("All() should register usajobs when USAJOBS_API_KEY is set")
	}
	if !slices.Contains(FilterableProviders(), "usajobs") {
		t.Error("FilterableProviders() should include usajobs when configured")
	}
}

func TestUSAJobsBoardFileValidates(t *testing.T) {
	t.Setenv("USAJOBS_API_KEY", "test-key")
	cfg, err := LoadConfig("../../sources/usajobs.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/usajobs.yml fails validation: %v", err)
	}
}

// fakeUSAJobs serves the search fixture for page 1 and an empty page afterwards, and
// records the auth header so the test asserts the key is carried on every request.
type fakeUSAJobs struct {
	page1      string
	gotHeaders map[string]string
}

func (f *fakeUSAJobs) GetJSONWithHeaders(_ context.Context, url string, headers map[string]string, v any) error {
	f.gotHeaders = headers
	body := `{"SearchResult":{"SearchResultItems":[]}}`
	if strings.Contains(url, "Page=1") {
		body = f.page1
	}
	return json.Unmarshal([]byte(body), v)
}

func TestUSAJobsFetchMapsAndPaginates(t *testing.T) {
	page1, err := os.ReadFile("testdata/usajobs_search.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fake := &fakeUSAJobs{page1: string(page1)}

	jobs, err := NewUSAJobs(fake, "secret-key").Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := fake.gotHeaders["Authorization-Key"]; got != "secret-key" {
		t.Errorf("Authorization-Key header = %q, want secret-key", got)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "871922900" {
		t.Errorf("ExternalID = %q, want 871922900", j.ExternalID)
	}
	if j.Company != "Internal Revenue Service" {
		t.Errorf("Company = %q, want Internal Revenue Service", j.Company)
	}
	if j.Title != "Information Technology Specialist (Applications Software)" {
		t.Errorf("Title = %q", j.Title)
	}
	// The :443 default-port noise is stripped to a clean canonical URL.
	if j.URL != "https://www.usajobs.gov/job/871922900" {
		t.Errorf("URL = %q, want clean job URL", j.URL)
	}
	// First concrete location feeds the location dictionary (Multiple-Locations display is unparseable).
	if j.Location != "Anchorage, Alaska" {
		t.Errorf("Location = %q, want Anchorage, Alaska", j.Location)
	}
	// Telework-eligible but not fully remote → hybrid (structured signal only).
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true, want false (RemoteIndicator false)")
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-06-05" {
		t.Errorf("PostedAt = %v, want 2026-06-05", j.PostedAt)
	}
	if !strings.Contains(j.Description, "technical guidance") {
		t.Errorf("Description missing MajorDuties content: %q", j.Description)
	}
}
