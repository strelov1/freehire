package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestReedProvider(t *testing.T) {
	if got := NewReed(nil, "k").Provider(); got != "reed" {
		t.Errorf("Provider() = %q, want reed", got)
	}
}

func TestReedIsBoardlessAggregator(t *testing.T) {
	s := NewReed(nil, "k")
	if _, ok := s.(boardless); !ok {
		t.Error("reed should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("reed should implement the aggregator marker")
	}
}

// Reed is keyed like usajobs: registered (and filterable) only when REED_API_KEY is set.
func TestReedRegisteredOnlyWhenKeySet(t *testing.T) {
	t.Setenv("REED_API_KEY", "")
	if _, ok := All(nil)["reed"]; ok {
		t.Error("All() should NOT register reed without REED_API_KEY")
	}
	t.Setenv("REED_API_KEY", "test-key")
	if _, ok := All(nil)["reed"]; !ok {
		t.Error("All() should register reed when REED_API_KEY is set")
	}
	if !slices.Contains(FilterableProviders(), "reed") {
		t.Error("FilterableProviders() should include reed when configured")
	}
}

func TestReedBoardFileValidates(t *testing.T) {
	t.Setenv("REED_API_KEY", "test-key")
	cfg, err := LoadConfig("../../sources/reed.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/reed.yml fails validation: %v", err)
	}
}

// fakeReed serves a fixed two-result search page for every keyword (so the test can assert
// the adapter dedups the same job ids across the curated keyword set), and a per-id detail.
// It records the Authorization header so the test asserts Basic auth is carried.
type fakeReed struct {
	mu       sync.Mutex
	lastAuth string
	searches int
}

const reedSearchTwo = `{"totalResults":2,"results":[
  {"jobId":1001,"employerName":"Acme","jobTitle":"Software Engineer","locationName":"London","jobUrl":"https://www.reed.co.uk/jobs/se/1001"},
  {"jobId":1002,"employerName":"Beta Ltd","jobTitle":"Backend Developer","locationName":"Leeds","jobUrl":"https://www.reed.co.uk/jobs/be/1002"}
]}`

// 1001 carries an employer externalUrl; 1002 has none (must fall back to the Reed jobUrl).
const reedDetail1001 = `{"jobId":1001,"employerName":"Acme","jobTitle":"Software Engineer","locationName":"London","datePosted":"17/06/2026","externalUrl":"https://careers.acme.com/jobs/1001","jobUrl":"https://www.reed.co.uk/jobs/se/1001","jobDescription":"<p>Full description for 1001 with plenty of detail.</p>"}`
const reedDetail1002 = `{"jobId":1002,"employerName":"Beta Ltd","jobTitle":"Backend Developer","locationName":"Leeds","datePosted":"17/06/2026","externalUrl":"","jobUrl":"https://www.reed.co.uk/jobs/be/1002","jobDescription":"<p>Full description for 1002.</p>"}`

func (f *fakeReed) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.lastAuth = headers["Authorization"]
	f.mu.Unlock()
	if strings.Contains(url, "/search") {
		f.mu.Lock()
		f.searches++
		f.mu.Unlock()
		return json.Unmarshal([]byte(reedSearchTwo), v)
	}
	switch {
	case strings.HasSuffix(url, "/1001"):
		return json.Unmarshal([]byte(reedDetail1001), v)
	case strings.HasSuffix(url, "/1002"):
		return json.Unmarshal([]byte(reedDetail1002), v)
	}
	return json.Unmarshal([]byte(`{}`), v)
}

func TestReedFetchDedupAuthAndDetailURL(t *testing.T) {
	const key = "test-key"
	f := &fakeReed{}
	jobs, err := NewReed(f, key).Fetch(context.Background(), CompanyEntry{Company: "Reed", Provider: "reed"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Every curated keyword search returns the same two ids; they must dedup to two jobs.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (deduped by jobId across keywords)", len(jobs))
	}
	if f.searches < 2 {
		t.Errorf("expected the adapter to search multiple curated keywords, got %d searches", f.searches)
	}

	// Basic auth: API key as username, blank password.
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(key+":"))
	if f.lastAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", f.lastAuth, wantAuth)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j1, ok := byID["1001"]
	if !ok {
		t.Fatal("missing job 1001")
	}
	if j1.URL != "https://careers.acme.com/jobs/1001" {
		t.Errorf("1001 URL = %q, want the employer externalUrl", j1.URL)
	}
	if !strings.Contains(j1.Description, "Full description for 1001") {
		t.Errorf("1001 description should come from detail, got %q", j1.Description)
	}
	if j1.Company != "Acme" {
		t.Errorf("1001 company = %q, want Acme", j1.Company)
	}
	if j1.PostedAt == nil || j1.PostedAt.Format("2006-01-02") != "2026-06-17" {
		t.Errorf("1001 PostedAt = %v, want 2026-06-17", j1.PostedAt)
	}

	j2 := byID["1002"]
	if j2.URL != "https://www.reed.co.uk/jobs/be/1002" {
		t.Errorf("1002 URL = %q, want the Reed jobUrl fallback (no externalUrl)", j2.URL)
	}
}

// flakyReed fails search calls for which failSearch returns true, succeeds otherwise; every
// detail succeeds. Lets a test drive partial vs total search failure.
type flakyReed struct {
	failSearch func(url string) bool
}

func (f flakyReed) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	if strings.Contains(url, "/search") {
		if f.failSearch(url) {
			return errors.New("boom")
		}
		return json.Unmarshal([]byte(reedSearchTwo), v)
	}
	switch {
	case strings.HasSuffix(url, "/1001"):
		return json.Unmarshal([]byte(reedDetail1001), v)
	default:
		return json.Unmarshal([]byte(reedDetail1002), v)
	}
}

// A single keyword's search failure must not abort the rest: the union of the other keywords
// still yields jobs (mirrors fetchDetailsStream dropping one bad detail).
func TestReedPartialKeywordFailureStillReturnsJobs(t *testing.T) {
	f := flakyReed{failSearch: func(u string) bool {
		return strings.Contains(u, url.QueryEscape("software developer"))
	}}
	jobs, err := NewReed(f, "k").Fetch(context.Background(), CompanyEntry{Provider: "reed"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (one failed keyword tolerated)", len(jobs))
	}
}

// If EVERY keyword search fails, Fetch returns an error rather than an empty (looks-like-no-jobs) crawl.
func TestReedAllSearchesFailReturnsError(t *testing.T) {
	f := flakyReed{failSearch: func(string) bool { return true }}
	if _, err := NewReed(f, "k").Fetch(context.Background(), CompanyEntry{Provider: "reed"}); err == nil {
		t.Fatal("Fetch should error when all keyword searches fail")
	}
}

// The captured live detail fixture maps to a Job with the employer's real externalUrl.
func TestReedDetailFixtureMaps(t *testing.T) {
	raw, err := os.ReadFile("testdata/reed_job.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var d reedJob
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	job, ok := d.toJob()
	if !ok {
		t.Fatal("fixture should map to a job")
	}
	if !strings.HasPrefix(job.URL, "https://careers.tesco.com/") {
		t.Errorf("URL = %q, want the Tesco externalUrl", job.URL)
	}
	if job.Company != "Tesco" {
		t.Errorf("company = %q, want Tesco", job.Company)
	}
	if len(job.Description) == 0 {
		t.Error("description should be non-empty")
	}
}
