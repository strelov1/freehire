package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// uberHTTP is a body-aware test JSONPoster: Uber pages its search results by the page
// number in the POST body (the URL is constant), so this fake routes a canned response
// on req.Page. An unknown page returns an empty, successful result set — the natural
// "past the end" response that ends pagination.
type uberHTTP struct {
	pages      map[int]string
	fail       bool
	gotURL     string
	gotPages   []int
	gotHeaders map[string]string
}

func (f *uberHTTP) PostJSONWithHeaders(_ context.Context, url string, headers map[string]string, body, v any) error {
	f.gotURL = url
	f.gotHeaders = headers
	req, ok := body.(uberRequest)
	if !ok {
		return errors.New("uberHTTP: body is not a uberRequest")
	}
	f.gotPages = append(f.gotPages, req.Page)
	if f.fail {
		return errors.New("uberHTTP: boom")
	}
	raw, ok := f.pages[req.Page]
	if !ok {
		raw = `{"status":"success","data":{"results":[],"totalResults":{"low":0}}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestUberProvider(t *testing.T) {
	if got := NewUber(nil).Provider(); got != "uber" {
		t.Errorf("Provider() = %q, want %q", got, "uber")
	}
}

func TestUberFetchPaginatesAndMaps(t *testing.T) {
	fake := &uberHTTP{pages: map[int]string{
		0: `{"status":"success","data":{"totalResults":{"low":3},"results":[
			{"id":153366,"title":"Engineering Manager","description":"**Hybrid** role\n\n<script>alert(1)</script>","location":{"city":"Seattle","region":"Washington","countryName":"United States"},"creationDate":"2026-01-08T22:29:00.000Z"},
			{"id":159903,"title":"Solutions Architect","description":"About the role","location":{"city":"Sao Paulo","region":"Sao Paulo","countryName":"Brazil"},"creationDate":"2026-06-12T17:50:00.000Z"}
		]}}`,
		1: `{"status":"success","data":{"totalResults":{"low":3},"results":[
			{"id":160000,"title":"Data Scientist","description":"Numbers","location":{"city":"","region":"","countryName":"India"},"creationDate":""}
		]}}`,
	}}

	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber", Provider: "uber"})
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

	j := byID["153366"]
	if j.Title != "Engineering Manager" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Uber" {
		t.Errorf("Company = %q, want Uber", j.Company)
	}
	if want := "https://www.uber.com/global/en/careers/list/153366/"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if want := "Seattle, Washington, United States"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
	if !strings.Contains(j.Description, "<strong>Hybrid</strong>") {
		t.Errorf("Description not rendered from Markdown: %q", j.Description)
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 1, 8, 22, 29, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-01-08T22:29:00Z", j.PostedAt)
	}

	// Empty city/region collapse to just the country; empty creationDate -> nil PostedAt.
	jd := byID["160000"]
	if jd.Location != "India" {
		t.Errorf("Location = %q, want India (blank city/region skipped)", jd.Location)
	}
	if jd.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (blank creationDate)", jd.PostedAt)
	}
}

func TestUberStopsAtEmptyPage(t *testing.T) {
	// totalResults says 5 but the source only returns 1 then an empty page: the adapter
	// must stop on the empty page rather than loop forever chasing the count.
	fake := &uberHTTP{pages: map[int]string{
		0: `{"status":"success","data":{"totalResults":{"low":5},"results":[
			{"id":1,"title":"Only One","description":"x","location":{"countryName":"US"},"creationDate":""}
		]}}`,
	}}
	jobs, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (stop at empty page)", len(jobs))
	}
}

func TestUberPostsToSearchURL(t *testing.T) {
	fake := &uberHTTP{pages: map[int]string{}}
	if _, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasPrefix(fake.gotURL, "https://www.uber.com/api/loadSearchJobsResults") {
		t.Errorf("posted to %q, want the loadSearchJobsResults endpoint", fake.gotURL)
	}
}

func TestUberSendsCSRFHeader(t *testing.T) {
	// Uber's search API 403s without an x-csrf-token header (any value is accepted); the
	// adapter must send it or every board fails.
	fake := &uberHTTP{pages: map[int]string{}}
	if _, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.gotHeaders["x-csrf-token"] == "" {
		t.Errorf("missing x-csrf-token header; sent %v", fake.gotHeaders)
	}
}

func TestUberNonSuccessFailsBoard(t *testing.T) {
	// A 200 whose status is not "success" must fail the board, not read as empty.
	fake := &uberHTTP{pages: map[int]string{
		0: `{"status":"error","data":{"results":null,"totalResults":{"low":0}}}`,
	}}
	if _, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"}); err == nil {
		t.Fatal("Fetch: want error on non-success status, got nil")
	}
}

func TestUberTransportErrorFailsBoard(t *testing.T) {
	fake := &uberHTTP{fail: true}
	if _, err := NewUber(fake).Fetch(context.Background(), CompanyEntry{Company: "Uber"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestUberRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["uber"]
	if !ok {
		t.Fatal(`All(nil)["uber"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("uber should be boardless (single company, no board id)")
	}
}
