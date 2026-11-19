package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestDevGlobalJobsProvider(t *testing.T) {
	if got := NewDevGlobalJobs(nil).Provider(); got != "devglobaljobs" {
		t.Fatalf("Provider() = %q, want %q", got, "devglobaljobs")
	}
}

func TestDevGlobalJobsMarkers(t *testing.T) {
	s := NewDevGlobalJobs(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("devglobaljobs should implement boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("devglobaljobs should implement aggregator")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("devglobaljobs should implement CoverageGated")
	}
	if gs, ok := s.(sweepGrace); !ok || gs.SweepGrace() != 30*24*time.Hour {
		t.Errorf("devglobaljobs sweepGrace() = %v, want 30 days", gs.SweepGrace())
	}
	if !slices.Contains(FilterableProviders(), "devglobaljobs") {
		t.Error("FilterableProviders() should include devglobaljobs")
	}
}

func TestDevGlobalJobsFetch(t *testing.T) {
	listJSON := `{
		"jobs": [
			{
				"id": 165923983,
				"title": "Senior Software Engineer, Storage",
				"organization": "Airbnb",
				"location": "San Francisco, CA",
				"country": "US",
				"category": "technology",
				"jobType": "Full-time",
				"descriptionExcerpt": "Short excerpt here",
				"postedAt": "2026-10-20T05:41:25.987Z",
				"url": "https://devglobaljobs.com/jobs/detail/165923983"
			},
			{
				"id": 165916167,
				"title": "Full Stack Developer",
				"organization": "RemoteCo",
				"location": "Remote",
				"country": "GB",
				"category": "technology",
				"jobType": "Contract",
				"descriptionExcerpt": "Remote work excerpt",
				"postedAt": "2026-10-20T05:11:28.119Z",
				"url": "https://devglobaljobs.com/jobs/detail/165916167"
			}
		]
	}`

	detailJSON1 := `{
		"id": 165923983,
		"description": "<p>Full detailed description from API</p>"
	}`
	detailJSON2 := `{
		"id": 165916167,
		"description": "<p>Remote work only full description</p>"
	}`

	routed := (&routedHTTP{}).
		route("offset=0", listJSON).
		route("/api/v1/jobs/165923983", detailJSON1).
		route("/api/v1/jobs/165916167", detailJSON2)

	jobs, err := NewDevGlobalJobs(routed).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	// First job
	j1 := jobs[0]
	if j1.ExternalID != "165923983" {
		t.Errorf("ExternalID = %q, want 165923983", j1.ExternalID)
	}
	if j1.Title != "Senior Software Engineer, Storage" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "Airbnb" {
		t.Errorf("Company = %q", j1.Company)
	}
	if j1.Location != "San Francisco, CA" {
		t.Errorf("Location = %q", j1.Location)
	}
	if !reflect.DeepEqual(j1.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us]", j1.Countries)
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j1.EmploymentType)
	}
	if j1.Description != "<p>Full detailed description from API</p>" {
		t.Errorf("Description = %q", j1.Description)
	}
	if j1.PostedAt == nil {
		t.Error("PostedAt is nil")
	}

	// Second job
	j2 := jobs[1]
	if j2.ExternalID != "165916167" {
		t.Errorf("ExternalID = %q, want 165916167", j2.ExternalID)
	}
	if !j2.Remote || j2.WorkMode != "remote" {
		t.Errorf("Remote=%v, WorkMode=%q, want remote", j2.Remote, j2.WorkMode)
	}
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j2.EmploymentType)
	}
	if !reflect.DeepEqual(j2.Countries, []string{"gb"}) {
		t.Errorf("Countries = %v, want [gb]", j2.Countries)
	}
	if j2.Description != "<p>Remote work only full description</p>" {
		t.Errorf("Description = %q", j2.Description)
	}
}

func TestDevGlobalJobsRemoteParsing(t *testing.T) {
	listJSON := `{
		"jobs": [
			{
				"id": 1,
				"title": "Engineer",
				"organization": "Acme",
				"location": "Berlin, Germany",
				"category": "remote",
				"url": "https://devglobaljobs.com/jobs/detail/1"
			},
			{
				"id": 2,
				"title": "Engineer",
				"organization": "Acme",
				"location": "Fully Remote - US",
				"category": "engineering",
				"url": "https://devglobaljobs.com/jobs/detail/2"
			},
			{
				"id": 3,
				"title": "Engineer",
				"organization": "Acme",
				"location": "Hybrid - London, UK",
				"category": "engineering",
				"url": "https://devglobaljobs.com/jobs/detail/3"
			}
		]
	}`

	routed := (&routedHTTP{}).
		route("offset=0", listJSON).
		route("/api/v1/jobs/1", `{"id":1,"description":"desc"}`).
		route("/api/v1/jobs/2", `{"id":2,"description":"desc"}`).
		route("/api/v1/jobs/3", `{"id":3,"description":"desc"}`)

	jobs, err := NewDevGlobalJobs(routed).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}

	// category: "remote" with location: "Berlin, Germany" must NOT be marked remote
	if jobs[0].Remote || jobs[0].WorkMode == "remote" {
		t.Errorf("job 1 with location 'Berlin, Germany' and category 'remote' should not be remote: %+v", jobs[0])
	}

	// location: "Fully Remote - US"
	if !jobs[1].Remote || jobs[1].WorkMode != "remote" {
		t.Errorf("job 2 with location 'Fully Remote - US' should be remote: %+v", jobs[1])
	}

	// location: "Hybrid - London, UK"
	if jobs[2].Remote || jobs[2].WorkMode != "hybrid" {
		t.Errorf("job 3 with location 'Hybrid - London, UK' should be hybrid: %+v", jobs[2])
	}
}

func TestDevGlobalJobsDetailFailure(t *testing.T) {
	listJSON := `{
		"jobs": [
			{
				"id": 101,
				"title": "Software Engineer",
				"organization": "CompanyA",
				"url": "https://devglobaljobs.com/jobs/detail/101"
			},
			{
				"id": 102,
				"title": "Backend Dev",
				"organization": "CompanyB",
				"url": "https://devglobaljobs.com/jobs/detail/102"
			},
			{
				"id": 103,
				"title": "Frontend Dev",
				"organization": "CompanyC",
				"url": "https://devglobaljobs.com/jobs/detail/103"
			}
		]
	}`

	routed := (&routedHTTP{}).
		route("offset=0", listJSON).
		// 101 fails with 500: transport/server failure -> Unreadable marker
		routeErr("/api/v1/jobs/101", &StatusError{Method: "GET", Code: http.StatusInternalServerError, URL: "https://devglobaljobs.com/api/v1/jobs/101"}).
		// 102 fails with 404: gone -> dropped (not returned)
		routeErr("/api/v1/jobs/102", &StatusError{Method: "GET", Code: http.StatusNotFound, URL: "https://devglobaljobs.com/api/v1/jobs/102"}).
		// 103 answers 200 with empty description -> Unreadable marker
		route("/api/v1/jobs/103", `{"id":103,"description":"  "}`)

	jobs, err := NewDevGlobalJobs(routed).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 102 was dropped on 404; 101 and 103 yield Unreadable markers
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (102 dropped on 404)", len(jobs))
	}

	j101 := jobs[0]
	if !j101.Unreadable {
		t.Errorf("job 101 should be marked Unreadable: %+v", j101)
	}
	if j101.ExternalID != "101" || j101.Company != "CompanyA" {
		t.Errorf("unexpected identity for unreadable job 101: %+v", j101)
	}

	j103 := jobs[1]
	if !j103.Unreadable {
		t.Errorf("job 103 should be marked Unreadable: %+v", j103)
	}
	if j103.ExternalID != "103" || j103.Company != "CompanyC" {
		t.Errorf("unexpected identity for unreadable job 103: %+v", j103)
	}
}

func TestDevGlobalJobsPagination(t *testing.T) {
	// Build a full 100-job page 0
	page0JSON := `{"meta":{"hasMore":true},"jobs":[`
	for i := 0; i < 100; i++ {
		if i > 0 {
			page0JSON += ","
		}
		page0JSON += fmt.Sprintf(`{"id":%d,"title":"Job %d","organization":"Org %d","url":"https://devglobaljobs.com/jobs/detail/%d"}`, i+1, i+1, i+1, i+1)
	}
	page0JSON += `]}`

	// Page 1 with 2 jobs and hasMore: false
	page1JSON := `{
		"meta": {"hasMore": false},
		"jobs": [
			{"id": 101, "title": "Job 101", "organization": "Org 101", "url": "https://devglobaljobs.com/jobs/detail/101"},
			{"id": 102, "title": "Job 102", "organization": "Org 102", "url": "https://devglobaljobs.com/jobs/detail/102"}
		]
	}`

	routed := (&routedHTTP{}).
		route("offset=0", page0JSON).
		route("offset=100", page1JSON)

	for i := 1; i <= 102; i++ {
		routed.route(fmt.Sprintf("/api/v1/jobs/%d", i), fmt.Sprintf(`{"id":%d,"description":"desc"}`, i))
	}

	jobs, err := NewDevGlobalJobs(routed).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(jobs) != 102 {
		t.Fatalf("len(jobs) = %d, want 102 across two pages", len(jobs))
	}
	if jobs[0].ExternalID != "1" || jobs[101].ExternalID != "102" {
		t.Errorf("unexpected external IDs: first=%s, last=%s", jobs[0].ExternalID, jobs[101].ExternalID)
	}
}

func TestDevGlobalJobsFetchNewSeen(t *testing.T) {
	listJSON := `{
		"jobs": [
			{
				"id": 12345,
				"title": "Backend Engineer",
				"organization": "Stripe",
				"url": "https://devglobaljobs.com/jobs/detail/12345"
			}
		]
	}`

	routed := (&routedHTTP{}).route("offset=0", listJSON)

	// Mark 12345 as seen: should emit SeenRefresh with zero detail calls
	seenMap := map[string]bool{"12345": true}
	jobs, err := NewDevGlobalJobs(routed).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool {
		return seenMap[id]
	})
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !jobs[0].SeenRefresh {
		t.Errorf("expected SeenRefresh to be true, got false")
	}
	if jobs[0].ExternalID != "12345" || jobs[0].Company != "Stripe" {
		t.Errorf("unexpected job: %+v", jobs[0])
	}
}

func TestDevGlobalJobsCoverageGated(t *testing.T) {
	listJSON := `{
		"jobs": [
			{
				"id": 101,
				"title": "Software Engineer",
				"organization": "OpenAI",
				"descriptionExcerpt": "OpenAI excerpt",
				"url": "https://devglobaljobs.com/jobs/detail/101"
			},
			{
				"id": 102,
				"title": "Chef",
				"organization": "Local Bistro",
				"descriptionExcerpt": "Bistro excerpt",
				"url": "https://devglobaljobs.com/jobs/detail/102"
			},
			{
				"id": 103,
				"title": "Go Developer",
				"organization": "UncoveredStartup",
				"descriptionExcerpt": "Startup excerpt",
				"url": "https://devglobaljobs.com/jobs/detail/103"
			}
		]
	}`

	detailJSON := `{
		"id": 103,
		"description": "<p>Hydrated full body for uncovered startup</p>"
	}`

	routed := (&routedHTTP{}).
		route("offset=0", listJSON).
		route("/api/v1/jobs/103", detailJSON)

	gate := DetailGate{
		Covered: func(companies []string) map[string]bool {
			return map[string]bool{
				"OpenAI":       true,
				"Local Bistro": true,
			}
		},
	}

	jobs, err := NewDevGlobalJobs(routed).(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}

	// 101 (OpenAI) was covered: yielded body-less (uses excerpt, no detail fetch called)
	if jobs[0].Company != "OpenAI" || jobs[0].Description != "OpenAI excerpt" {
		t.Errorf("unexpected job 0: %+v", jobs[0])
	}

	// 102 (Local Bistro) was covered too: yielded body-less (uses excerpt, no detail fetch called)
	if jobs[1].Company != "Local Bistro" || jobs[1].Description != "Bistro excerpt" {
		t.Errorf("unexpected job 1: %+v", jobs[1])
	}

	// 103 (UncoveredStartup) was not covered: detail fetch was called!
	if jobs[2].Company != "UncoveredStartup" || jobs[2].Description != "<p>Hydrated full body for uncovered startup</p>" {
		t.Errorf("unexpected job 2: %+v", jobs[2])
	}
}

func TestDevGlobalJobsListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("network down")}
	_, err := NewDevGlobalJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDevGlobalJobsPaced(t *testing.T) {
	g, ok := pacedDevGlobalJobsGetter(&recordingJSONGetter{}).(rateLimitedJSONGetter)
	if !ok {
		t.Fatal("pacedDevGlobalJobsGetter should wrap getter in rateLimitedJSONGetter")
	}
	if g.limiter == nil {
		t.Fatal("expected limiter to be set")
	}
}
