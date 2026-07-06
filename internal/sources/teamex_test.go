package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// teamexHTTP is a route-aware test JSONPoster. teamex has a single POST list endpoint
// (/api/jts/global/filter?pageIndex=), so this fake routes each request by the pageIndex
// query parameter and records the pages fetched, letting a test assert pagination stops at
// paging.totalCount.
type teamexHTTP struct {
	pages     map[int]string // list body keyed by requested pageIndex
	failFirst bool           // pageIndex 1 fails
	failPage  map[int]bool   // a specific pageIndex fails
	gotPages  []int
}

var teamexPageRE = regexp.MustCompile(`pageIndex=(\d+)`)

func (f *teamexHTTP) PostJSON(_ context.Context, url string, _, v any) error {
	page := 1
	if m := teamexPageRE.FindStringSubmatch(url); m != nil {
		page, _ = strconv.Atoi(m[1])
	}
	f.gotPages = append(f.gotPages, page)
	if (page == 1 && f.failFirst) || f.failPage[page] {
		return errors.New("teamexHTTP: list boom")
	}
	raw, ok := f.pages[page]
	if !ok {
		raw = `{"data":{"paging":{"totalCount":0},"data":[]}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestTeamexProvider(t *testing.T) {
	if got := NewTeamex(nil).Provider(); got != "teamex" {
		t.Errorf("Provider() = %q, want %q", got, "teamex")
	}
}

func TestTeamexRegisteredInAllBoardlessAggregator(t *testing.T) {
	s, ok := All(nil)["teamex"]
	if !ok {
		t.Fatal(`All(nil)["teamex"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("teamex should be boardless (one global feed, no board id)")
	}
	if _, isAggregator := s.(aggregator); !isAggregator {
		t.Error("teamex should be an aggregator (many employers behind one marketplace)")
	}
	if !slices.Contains(FilterableProviders(), "teamex") {
		t.Error("FilterableProviders() should include the teamex aggregator")
	}
}

func TestTeamexFetchMapsJob(t *testing.T) {
	fake := &teamexHTTP{pages: map[int]string{1: `{"data":{"paging":{"totalCount":1},"data":[
		{"sequenceId":1506,"title":"Senior Full-Stack Engineer","isActive":true,"isAnonymous":false,
		 "publishedDate":"2026-07-06T13:52:49.765Z","yearsOfExperienceMin":6,
		 "descriptions":[{"title":"Overview","content":"<p>Build systems.</p><script>x()</script>"}],
		 "countries":[{"name":"Argentina","code":"AR"},{"name":"Costa Rica","code":"CR"}],
		 "requiredSkills":[{"skill":{"displayName":"Docker"}},{"skill":{"displayName":"React"}}],
		 "company":{"name":"Acme Health"}}
	]}}`}}

	jobs, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{Company: "teamex", Provider: "teamex"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1506" {
		t.Errorf("ExternalID = %q, want 1506", j.ExternalID)
	}
	if want := "https://teamex.io/job/1506"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Title != "Senior Full-Stack Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme Health" {
		t.Errorf("Company = %q, want the posting's own employer", j.Company)
	}
	if want := "Argentina, Costa Rica"; j.Location != want {
		t.Errorf("Location = %q, want %q (country names joined)", j.Location, want)
	}
	if !strings.Contains(j.Description, "Build systems") || strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not composed+sanitized: %q", j.Description)
	}
	if !slices.Equal(j.Skills, []string{"docker", "react"}) {
		t.Errorf("Skills = %v, want [docker react] (canonical, sorted)", j.Skills)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 6 {
		t.Errorf("ExperienceYearsMin = %v, want 6", j.ExperienceYearsMin)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 6, 13, 52, 49, 765000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-06T13:52:49.765Z", j.PostedAt)
	}
}

func TestTeamexSkipsInactive(t *testing.T) {
	fake := &teamexHTTP{pages: map[int]string{1: `{"data":{"paging":{"totalCount":2},"data":[
		{"sequenceId":1,"title":"Open","isActive":true,"company":{"name":"A"},"countries":[]},
		{"sequenceId":2,"title":"Closed","isActive":false,"company":{"name":"B"},"countries":[]}
	]}}`}}
	jobs, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("got %v, want only the active job (isActive:false excluded)", jobs)
	}
}

func TestTeamexAnonymousCompanyFallback(t *testing.T) {
	// An anonymous posting hides its employer (no company.name), so it falls back to the
	// marketplace label rather than an empty company.
	fake := &teamexHTTP{pages: map[int]string{1: `{"data":{"paging":{"totalCount":1},"data":[
		{"sequenceId":9,"title":"Hidden Employer","isActive":true,"isAnonymous":true,
		 "company":{},"countries":[]}
	]}}`}}
	jobs, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "TeamEx" {
		t.Fatalf("anonymous posting Company = %q, want TeamEx fallback", jobs[0].Company)
	}
}

func TestTeamexAnonymousHidesNamedEmployer(t *testing.T) {
	// When the API flags a posting anonymous but STILL carries the employer name, the flag
	// wins — the marketplace anonymized it on purpose, so the name must not leak.
	fake := &teamexHTTP{pages: map[int]string{1: `{"data":{"paging":{"totalCount":1},"data":[
		{"sequenceId":9,"title":"Hidden Employer","isActive":true,"isAnonymous":true,
		 "company":{"name":"Acme Corp"},"countries":[]}
	]}}`}}
	jobs, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "TeamEx" {
		t.Fatalf("anonymous posting Company = %q, want TeamEx (name must not leak)", jobs[0].Company)
	}
}

func TestTeamexPaginatesAndStopsAtTotal(t *testing.T) {
	// totalCount=51 with a 50 page size: pages 1 and 2 are fetched, then 2*50>=51 stops.
	page := func(id int) string {
		return `{"data":{"paging":{"totalCount":51},"data":[
			{"sequenceId":` + strconv.Itoa(id) + `,"title":"P","isActive":true,"company":{"name":"C"},"countries":[]}]}}`
	}
	fake := &teamexHTTP{pages: map[int]string{1: page(1), 2: page(2)}}
	jobs, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (two pages)", len(jobs))
	}
	if !slices.Equal(fake.gotPages, []int{1, 2}) {
		t.Errorf("fetched pages = %v, want [1 2] (stop at totalCount)", fake.gotPages)
	}
}

func TestTeamexFirstPageErrorFailsBoard(t *testing.T) {
	fake := &teamexHTTP{failFirst: true}
	if _, err := NewTeamex(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("Fetch: want first-page error, got nil")
	}
}
