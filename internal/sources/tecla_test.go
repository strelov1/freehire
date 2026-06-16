package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// teclaHTTP is a page-aware test JSONGetter: tecla's getPublicJobs is paged by a page
// query parameter, so this fake routes a canned response on the page parsed from the URL.
// An unrequested page returns an empty job list — the end-of-list response that stops
// pagination. gotPages records which pages were fetched so a test can assert the adapter
// stops at the API-reported page count instead of over-fetching.
type teclaHTTP struct {
	pages    map[int]string
	fail     bool
	gotPages []int
}

var teclaPageRE = regexp.MustCompile(`page=(\d+)`)

func (f *teclaHTTP) GetJSON(_ context.Context, url string, v any) error {
	if f.fail {
		return errors.New("teclaHTTP: boom")
	}
	page := 1
	if m := teclaPageRE.FindStringSubmatch(url); m != nil {
		page, _ = strconv.Atoi(m[1])
	}
	f.gotPages = append(f.gotPages, page)
	raw, ok := f.pages[page]
	if !ok {
		raw = `{"success":true,"data":{"jobs":[],"pagination":{"countPages":0,"current":1}}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestTeclaProvider(t *testing.T) {
	if got := NewTecla(nil).Provider(); got != "tecla" {
		t.Errorf("Provider() = %q, want %q", got, "tecla")
	}
}

func TestTeclaFetchPaginatesAndMaps(t *testing.T) {
	fake := &teclaHTTP{pages: map[int]string{
		1: `{"success":true,"data":{"jobs":[
			{"id":3301,"name":"Practice / Business Development","company":{"name":"Sliiip"},"createdAt":"2026-05-25T17:02:00.864421","description":"<p>Grow it</p><script>alert(1)</script>"},
			{"id":3308,"name":"Founding Software Engineer","company":{"name":"Psyflo"},"createdAt":"2026-06-09T01:28:17.971898","description":"<p>Own it</p>"}
		],"pagination":{"countPages":2,"current":1}}}`,
		2: `{"success":true,"data":{"jobs":[
			{"id":3400,"name":"Product Designer","company":{"name":"Acme"},"createdAt":"","description":"<p>Design</p>"}
		],"pagination":{"countPages":2,"current":2}}}`,
	}}

	jobs, err := NewTecla(fake).Fetch(context.Background(), CompanyEntry{Company: "Tecla", Provider: "tecla"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (two pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["3301"]
	if j.Title != "Practice / Business Development" {
		t.Errorf("Title = %q", j.Title)
	}
	// The marketplace posting carries its OWN employer — not the configured placeholder.
	if j.Company != "Sliiip" {
		t.Errorf("Company = %q, want Sliiip (per-job, not the entry's Tecla)", j.Company)
	}
	if want := "https://app.tecla.io/job?id=3301"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote (remote-only marketplace)", j.Remote, j.WorkMode)
	}
	if !strings.Contains(j.Description, "<p>Grow it</p>") || strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 5, 25, 17, 2, 0, 864421000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-25T17:02:00.864421 (no-tz layout)", j.PostedAt)
	}

	if byID["3308"].Company != "Psyflo" {
		t.Errorf("Company = %q, want Psyflo", byID["3308"].Company)
	}
	if byID["3400"].PostedAt != nil {
		t.Errorf("blank createdAt should yield nil PostedAt, got %v", byID["3400"].PostedAt)
	}
}

func TestTeclaStopsAtReportedPageCount(t *testing.T) {
	// countPages=1, so the adapter must stop after page 1 and never request page 2,
	// even though a page-2 response exists in the fake.
	fake := &teclaHTTP{pages: map[int]string{
		1: `{"success":true,"data":{"jobs":[{"id":1,"name":"Only","company":{"name":"X"},"createdAt":"","description":"x"}],"pagination":{"countPages":1,"current":1}}}`,
		2: `{"success":true,"data":{"jobs":[{"id":2,"name":"Nope","company":{"name":"Y"},"createdAt":"","description":"y"}],"pagination":{"countPages":1,"current":2}}}`,
	}}
	jobs, err := NewTecla(fake).Fetch(context.Background(), CompanyEntry{Company: "Tecla"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (stop at reported page count)", len(jobs))
	}
	if len(fake.gotPages) != 1 || fake.gotPages[0] != 1 {
		t.Errorf("fetched pages = %v, want only [1]", fake.gotPages)
	}
}

func TestTeclaTransportErrorFailsBoard(t *testing.T) {
	fake := &teclaHTTP{fail: true}
	if _, err := NewTecla(fake).Fetch(context.Background(), CompanyEntry{Company: "Tecla"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestTeclaRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["tecla"]
	if !ok {
		t.Fatal(`All(nil)["tecla"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("tecla should be boardless (one global feed, no board id)")
	}
}
