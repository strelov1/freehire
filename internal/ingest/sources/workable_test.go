package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkableProvider(t *testing.T) {
	if got := NewWorkable(nil).Provider(); got != "workable" {
		t.Errorf("Provider() = %q, want %q", got, "workable")
	}
}

// Workable earns fullBoardListing because Fetch is a single unpaginated request that returns
// the board's whole jobs array in one response — there is no loop that could stop early, so
// any listing failure aborts the whole Fetch rather than returning a partial result.
func TestWorkableMarkers(t *testing.T) {
	s := NewWorkable(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("workable should implement the fullBoardListing marker")
	}
}

func TestWorkableRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["workable"] {
		t.Error("FullBoardListingProviders(All(nil)) should include workable")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestWorkableMarkers' fullBoardListing claim rests on.
func TestWorkableFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewWorkable(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestWorkableFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"title": "Backend Engineer",
				"shortcode": "ABC123",
				"url": "https://apply.workable.com/j/ABC123",
				"published_on": "2024-01-15",
				"city": "Berlin",
				"state": "",
				"country": "Germany",
				"telecommuting": true,
				"description": "<p>Build <strong>things</strong>.</p><script>x()</script>"
			}
		]
	}`}

	jobs, err := NewWorkable(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Hugging Face", Provider: "workable", Board: "huggingface",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "huggingface") || !strings.Contains(fake.gotURL, "details=true") {
		t.Errorf("requested URL %q should target the board with details=true", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "ABC123" {
		t.Errorf("ExternalID = %q, want the shortcode", j.ExternalID)
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://apply.workable.com/j/ABC123" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Hugging Face" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q, want non-empty city/country joined", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from telecommuting")
	}
	if !strings.Contains(j.Description, "<strong>things</strong>") {
		t.Errorf("Description should be sanitized HTML, got %q", j.Description)
	}
	if strings.Contains(j.Description, "<script") {
		t.Errorf("Description retained a script tag, got %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed published_on (2024)", j.PostedAt)
	}
}
