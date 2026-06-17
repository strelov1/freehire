package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// rateLimitedHTTP serves the pcsx list, then fails detail GETs with failErr for the first
// detailFails attempts before serving detail — mimicking Eightfold's ~290-requests/window
// per-IP cap (a 403). detailCalls counts detail attempts so a test can assert retry behaviour.
type rateLimitedHTTP struct {
	mu          sync.Mutex
	list        string
	detail      string
	failErr     string
	detailFails int
	detailCalls int
}

func (r *rateLimitedHTTP) GetJSON(_ context.Context, url string, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.Contains(url, "/api/pcsx/search") {
		return json.Unmarshal([]byte(r.list), v)
	}
	r.detailCalls++
	if r.detailFails > 0 {
		r.detailFails--
		return fmt.Errorf("sources: GET %s: %s", url, r.failErr)
	}
	return json.Unmarshal([]byte(r.detail), v)
}

// TestEightfoldRetriesRateLimitedDetail verifies a rate-limit (403) detail response is retried
// with backoff rather than dropped, so the catalogue is not capped at the per-window limit.
func TestEightfoldRetriesRateLimitedDetail(t *testing.T) {
	eightfoldRetryBase = 0 // no real sleeping in tests
	fake := &rateLimitedHTTP{
		list:        eightfoldList(1, `{"id": 111, "name": "Role", "locations": ["Remote"]}`),
		detail:      `{"id": 111, "job_description": "<p>desc</p>"}`,
		failErr:     "status 403",
		detailFails: 3,
	}
	jobs, err := NewEightfold(fake).Fetch(context.Background(), eightfoldEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (a rate-limited detail must retry past the 403)", len(jobs))
	}
	if fake.detailCalls != 4 {
		t.Errorf("detailCalls = %d, want 4 (3 retries then success)", fake.detailCalls)
	}
}

// TestEightfoldDoesNotRetryNonRateLimitError verifies a non-rate-limit failure (e.g. 404) is
// dropped immediately, so retry stays scoped to the rate-limit case.
func TestEightfoldDoesNotRetryNonRateLimitError(t *testing.T) {
	eightfoldRetryBase = 0
	fake := &rateLimitedHTTP{
		list:        eightfoldList(1, `{"id": 222, "name": "Gone", "locations": ["Remote"]}`),
		detail:      `{}`,
		failErr:     "status 404",
		detailFails: 99,
	}
	jobs, err := NewEightfold(fake).Fetch(context.Background(), eightfoldEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 (a 404 detail is dropped)", len(jobs))
	}
	if fake.detailCalls != 1 {
		t.Errorf("detailCalls = %d, want 1 (a 404 must not be retried)", fake.detailCalls)
	}
}

// eightfoldEntry is the shared configured board for the Fetch tests.
var eightfoldEntry = CompanyEntry{
	Company: "Microsoft", Provider: "eightfold",
	Board: "apply.careers.microsoft.com/microsoft.com",
}

func TestEightfoldProvider(t *testing.T) {
	if got := NewEightfold(nil).Provider(); got != "eightfold" {
		t.Errorf("Provider() = %q, want %q", got, "eightfold")
	}
}

func TestParseEightfoldBoard(t *testing.T) {
	t.Run("valid host/domain", func(t *testing.T) {
		b, err := parseEightfoldBoard("apply.careers.microsoft.com/microsoft.com")
		if err != nil {
			t.Fatalf("parseEightfoldBoard: %v", err)
		}
		if b.host != "apply.careers.microsoft.com" {
			t.Errorf("host = %q", b.host)
		}
		if b.domain != "microsoft.com" {
			t.Errorf("domain = %q", b.domain)
		}
	})

	for _, board := range []string{
		"apply.careers.microsoft.com",  // no slash → no domain
		"/microsoft.com",               // empty host
		"apply.careers.microsoft.com/", // empty domain
		"",
	} {
		t.Run("rejects "+board, func(t *testing.T) {
			if _, err := parseEightfoldBoard(board); err == nil {
				t.Errorf("parseEightfoldBoard(%q) = nil error, want error", board)
			}
		})
	}
}

// eightfoldList builds a /api/pcsx/search response body with the given count and raw positions.
func eightfoldList(count int, positions ...string) string {
	return `{"data": {"count": ` + strconv.Itoa(count) + `, "positions": [` + strings.Join(positions, ",") + `]}}`
}

// TestEightfoldFetchListsAndFetchesDetail covers the core path: page the position list, fetch
// each position's detail for the description and canonical URL, and map every field. The list
// carries metadata (title, location, postedTs, workLocationOption) while the detail carries the
// description and canonicalPositionUrl.
func TestEightfoldFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("pcsx/search", eightfoldList(2,
			`{"id": 111, "displayJobId": "200001", "name": "Backend Engineer",
			  "locations": ["United States, Washington, Redmond"], "postedTs": 1781691482,
			  "workLocationOption": "onsite", "atsJobId": "200001"}`,
			`{"id": 222, "name": "Data Engineer", "locations": ["Remote"], "postedTs": 0,
			  "workLocationOption": "remote"}`)).
		route("jobs/111", `{"id": 111, "job_description": "<p>Build the backend.</p>",
			"canonicalPositionUrl": "https://apply.careers.microsoft.com/careers/job/111"}`).
		route("jobs/222", `{"id": 222, "job_description": "<p>Crunch data.</p>"}`)

	jobs, err := NewEightfold(fake).Fetch(context.Background(), eightfoldEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["111"]
	if !ok {
		t.Fatal("position 111 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Microsoft" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "United States, Washington, Redmond" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.URL != "https://apply.careers.microsoft.com/careers/job/111" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true, want false for an onsite role")
	}
	if !strings.Contains(j.Description, "Build the backend.") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-06-17" {
		t.Errorf("PostedAt = %v, want 2026-06-17", j.PostedAt)
	}

	data := byID["222"]
	if data.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", data.WorkMode)
	}
	if !data.Remote {
		t.Error("Remote = false, want true for a remote role")
	}
	// canonicalPositionUrl absent → fall back to the public position page.
	if data.URL != "https://apply.careers.microsoft.com/careers/job/222" {
		t.Errorf("URL = %q, want the host/careers/job fallback", data.URL)
	}
	if data.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil for postedTs=0", data.PostedAt)
	}
}

// TestEightfoldPaginatesUntilEmptyPage verifies the lister advances start past the first page
// and stops on an empty page even when count would not be reached.
func TestEightfoldPaginatesUntilEmptyPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("start=0", eightfoldList(99,
			`{"id": 1, "name": "Role 1", "locations": ["Remote"]}`,
			`{"id": 2, "name": "Role 2", "locations": ["Remote"]}`)).
		route("start=2", eightfoldList(99)).
		route("jobs/", `{"job_description": "<p>desc</p>"}`)

	jobs, err := NewEightfold(fake).Fetch(context.Background(), eightfoldEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across the non-empty page", len(jobs))
	}
}

// TestEightfoldDropsFailedDetail verifies one position whose detail request fails is skipped
// while the rest of the board still yields.
func TestEightfoldDropsFailedDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("pcsx/search", eightfoldList(2,
			`{"id": 111, "name": "Kept", "locations": ["Remote"]}`,
			`{"id": 222, "name": "Dropped", "locations": ["Remote"]}`)).
		route("jobs/111", `{"id": 111, "job_description": "<p>kept</p>"}`)
	// no route for jobs/222 → its detail GET errors → that posting is dropped.

	jobs, err := NewEightfold(fake).Fetch(context.Background(), eightfoldEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (failed detail dropped)", len(jobs))
	}
	if jobs[0].ExternalID != "111" {
		t.Errorf("kept job = %q, want 111", jobs[0].ExternalID)
	}
}

// TestEightfoldFetchFallsBackToLegacyV2List covers the old Eightfold generation (e.g.
// Netflix), whose listing lives at /api/apply/v2/jobs (top-level positions/count, t_create
// date) instead of /api/pcsx/search. With no pcsx route the pcsx request errors and the
// adapter falls back to the v2 list; the shared detail endpoint is unchanged.
func TestEightfoldFetchFallsBackToLegacyV2List(t *testing.T) {
	fake := (&routedHTTP{}).
		// note: "apply/v2/jobs?" matches the LIST, "jobs/<id>" matches the DETAIL.
		route("apply/v2/jobs?", `{"count": 2, "positions": [
			{"id": 790, "name": "UX Designer", "location": "Helsinki,Finland",
			 "t_create": 1779926400, "work_location_option": "onsite",
			 "canonicalPositionUrl": "https://explore.jobs.netflix.net/careers/job/790"},
			{"id": 791, "name": "Finance Manager", "location": "Sao Paulo,Brazil", "t_create": 0}
		]}`).
		route("jobs/790", `{"id": 790, "job_description": "<p>Design.</p>",
			"canonicalPositionUrl": "https://explore.jobs.netflix.net/careers/job/790?microsite=netflix.com"}`).
		route("jobs/791", `{"id": 791, "job_description": "<p>Finance.</p>"}`)

	jobs, err := NewEightfold(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Netflix", Provider: "eightfold",
		Board: "explore.jobs.netflix.net/netflix.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["790"]
	if j.Title != "UX Designer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Location != "Helsinki,Finland" {
		t.Errorf("Location = %q (want the v2 single-string location)", j.Location)
	}
	if j.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want a date from t_create")
	}
	// detail's canonicalPositionUrl wins over the list's.
	if j.URL != "https://explore.jobs.netflix.net/careers/job/790?microsite=netflix.com" {
		t.Errorf("URL = %q, want the detail canonical url", j.URL)
	}
	if !strings.Contains(j.Description, "Design.") {
		t.Errorf("Description = %q", j.Description)
	}

	data := byID["791"]
	// detail lacks a canonical url and the list position lacks one → host/careers/job fallback.
	if data.URL != "https://explore.jobs.netflix.net/careers/job/791" {
		t.Errorf("URL = %q, want the host fallback", data.URL)
	}
	if data.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil for t_create=0", data.PostedAt)
	}
}

// TestEightfoldFetchRejectsBadBoard verifies a malformed board fails fast before any request.
func TestEightfoldFetchRejectsBadBoard(t *testing.T) {
	_, err := NewEightfold(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Company: "Microsoft", Provider: "eightfold", Board: "apply.careers.microsoft.com",
	})
	if err == nil {
		t.Fatal("Fetch with a board missing the domain half = nil error, want error")
	}
}
