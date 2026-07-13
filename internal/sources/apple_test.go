package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

// appleFake is a test transport for the Apple adapter: the listing is a POST whose page
// number drives the canned search response, and each posting's detail is a GET keyed by the
// positionId in the URL. An unrequested page returns an empty result set (the natural
// end-of-list response that stops pagination); an unknown detail id returns an empty body.
type appleFake struct {
	pages     map[int]string    // search page -> /api/v1/search response JSON
	details   map[string]string // positionId -> /api/v1/jobDetails response JSON
	postFail  bool
	postCalls int
	getURLs   []string
}

var appleDetailRE = regexp.MustCompile(`jobDetails/([^?]+)`)

func (f *appleFake) PostJSON(_ context.Context, _ string, body, v any) error {
	f.postCalls++
	if f.postFail {
		return errors.New("appleFake: boom")
	}
	page, _ := body.(map[string]any)["page"].(int)
	raw, ok := f.pages[page]
	if !ok {
		raw = `{"res":{"searchResults":[],"totalRecords":0}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *appleFake) GetJSON(_ context.Context, url string, v any) error {
	f.getURLs = append(f.getURLs, url)
	id := ""
	if m := appleDetailRE.FindStringSubmatch(url); m != nil {
		id = m[1]
	}
	raw, ok := f.details[id]
	if !ok {
		raw = `{"res":{}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestAppleProvider(t *testing.T) {
	if got := NewApple(nil).Provider(); got != "apple" {
		t.Errorf("Provider() = %q, want %q", got, "apple")
	}
}

func TestAppleDescriptionBuildsSanitizedHTML(t *testing.T) {
	// Apple serves the summary + description as plain-text paragraphs (\n\n) and the
	// qualifications as newline-separated bullet lines. The stored description must be
	// sanitized HTML (the {@html} consumer), so paragraphs become <p> and each
	// qualification line a <li> under an <h2> section header.
	got := appleDescription(
		"Apple Retail is where the best of Apple comes together.\n\nAs a Specialist, you build brand loyalty.",
		"Deliver excellent service.\n\nStay up to date on Apple products.",
		"Availability to work nights and weekends.\nProficient in the local language.",
		"You can:\nDemonstrate knowledge of Apple products.\nPersonalize solutions.",
	)
	for _, want := range []string{
		"<p>Apple Retail is where the best of Apple comes together.</p>",
		"<p>As a Specialist, you build brand loyalty.</p>",
		"<p>Deliver excellent service.</p>",
		"<h2>Minimum Qualifications</h2>",
		"<li>Availability to work nights and weekends.</li>",
		"<h2>Preferred Qualifications</h2>",
		"<li>Personalize solutions.</li>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("appleDescription() missing %q\ngot: %q", want, got)
		}
	}
}

func TestAppleDescriptionOmitsEmptySections(t *testing.T) {
	got := appleDescription("", "Just a description.", "", "")
	if !strings.Contains(got, "<p>Just a description.</p>") {
		t.Errorf("want the description paragraph, got %q", got)
	}
	if strings.Contains(got, "Qualifications") {
		t.Errorf("empty qualification sections must be omitted, got %q", got)
	}
}

func TestAppleFetchPaginatesDedupsAndMaps(t *testing.T) {
	fake := &appleFake{
		pages: map[int]string{
			1: `{"res":{"totalRecords":3,"searchResults":[
				{"positionId":"200600664","postingTitle":"Software Engineer, Watch Software","transformedPostingTitle":"software-engineer-watch-software","postDateInGMT":"2025-12-02T08:30:00.000Z","homeOffice":false,"locations":[{"name":"San Diego"}]},
				{"positionId":"200600664","postingTitle":"Software Engineer, Watch Software","transformedPostingTitle":"software-engineer-watch-software","postDateInGMT":"2025-12-02T08:30:00.000Z","homeOffice":false,"locations":[{"name":"Cupertino"}]}
			]}}`,
			2: `{"res":{"totalRecords":3,"searchResults":[
				{"positionId":"200659431","postingTitle":"Site Reliability Engineer","transformedPostingTitle":"site-reliability-engineer","postDateInGMT":"2026-04-22T14:44:56.003755186Z","homeOffice":true,"locations":[{"name":"Seattle"}]}
			]}}`,
		},
		details: map[string]string{
			"200600664": `{"res":{"jobSummary":"Join the Watch team.","description":"Build the Watch software.","minimumQualifications":"5+ years experience.","preferredQualifications":"Swift expertise."}}`,
			"200659431": `{"res":{"description":"Keep services running.","minimumQualifications":"","preferredQualifications":""}}`,
		},
	}

	jobs, err := NewApple(fake).Fetch(context.Background(), CompanyEntry{Company: "Apple", Provider: "apple"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Three listing rows but only two distinct positionIds: the multi-location role is
	// deduped to one job and its detail fetched once.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (deduped by positionId)", len(jobs))
	}
	if n := len(fake.getURLs); n != 2 {
		t.Errorf("detail fetched %d times, want 2 (one per distinct position)", n)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["200600664"]
	if j.Title != "Software Engineer, Watch Software" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Apple" {
		t.Errorf("Company = %q, want Apple", j.Company)
	}
	if want := "https://jobs.apple.com/en-us/details/200600664/software-engineer-watch-software"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if want := "San Diego"; j.Location != want {
		t.Errorf("Location = %q, want first-seen %q", j.Location, want)
	}
	for _, part := range []string{"Join the Watch team.", "Build the Watch software.", "5+ years experience.", "Swift expertise."} {
		if !strings.Contains(j.Description, part) {
			t.Errorf("Description missing %q: %q", part, j.Description)
		}
	}
	if !strings.Contains(j.Description, "<p>") {
		t.Errorf("Description should be sanitized HTML, got plain text: %q", j.Description)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("homeOffice=false should be onsite-unknown, got Remote=%v WorkMode=%q", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2025, 12, 2, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-12-02T08:30:00Z", j.PostedAt)
	}

	r := byID["200659431"]
	if !r.Remote || r.WorkMode != "remote" {
		t.Errorf("homeOffice=true should map to remote, got Remote=%v WorkMode=%q", r.Remote, r.WorkMode)
	}
	if r.PostedAt == nil || !r.PostedAt.Equal(time.Date(2026, 4, 22, 14, 44, 56, 3755186, time.UTC)) {
		t.Errorf("PostedAt = %v, want the nanosecond RFC3339 value parsed", r.PostedAt)
	}
}

func TestAppleStopsAtEmptyPage(t *testing.T) {
	// totalRecords claims 9 but page 2 is empty: the adapter must stop on the empty page
	// rather than loop forever chasing the count.
	fake := &appleFake{
		pages: map[int]string{
			1: `{"res":{"totalRecords":9,"searchResults":[
				{"positionId":"1","postingTitle":"Only","transformedPostingTitle":"only","postDateInGMT":"","homeOffice":false,"locations":[{"name":"Remote"}]}
			]}}`,
		},
		details: map[string]string{"1": `{"res":{"description":"x"}}`},
	}
	jobs, err := NewApple(fake).Fetch(context.Background(), CompanyEntry{Company: "Apple"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (stop at empty page)", len(jobs))
	}
}

func TestAppleSkipsPostingWithoutDescription(t *testing.T) {
	// A posting whose detail carries no description is dropped (ok=false), not emitted blank.
	fake := &appleFake{
		pages: map[int]string{
			1: `{"res":{"totalRecords":1,"searchResults":[
				{"positionId":"42","postingTitle":"Ghost","transformedPostingTitle":"ghost","postDateInGMT":"","homeOffice":false,"locations":[{"name":"Cupertino"}]}
			]}}`,
		},
		details: map[string]string{"42": `{"res":{"description":""}}`},
	}
	jobs, err := NewApple(fake).Fetch(context.Background(), CompanyEntry{Company: "Apple"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (no description -> dropped)", len(jobs))
	}
}

func TestAppleTransportErrorFailsBoard(t *testing.T) {
	fake := &appleFake{postFail: true}
	if _, err := NewApple(fake).Fetch(context.Background(), CompanyEntry{Company: "Apple"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestAppleRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["apple"]
	if !ok {
		t.Fatal(`All(nil)["apple"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("apple should be boardless (single company, no board id)")
	}
}
