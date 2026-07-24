package sources

import (
	"context"
	"strings"
	"testing"
)

func TestAshbyProvider(t *testing.T) {
	if got := NewAshby(nil).Provider(); got != "ashby" {
		t.Errorf("Provider() = %q, want %q", got, "ashby")
	}
}

func TestAshbyFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": "job-uuid",
				"title": "Platform Engineer",
				"location": "San Francisco",
				"jobUrl": "https://jobs.ashbyhq.com/ashby/job-uuid",
				"publishedAt": "2024-01-15T10:00:00.000Z",
				"descriptionPlain": "Run the platform.",
				"descriptionHtml": "<p>Run the <strong>platform</strong>.</p><script>x()</script>",
				"isRemote": true
			}
		]
	}`}

	jobs, err := NewAshby(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Ashby", Provider: "ashby", Board: "ashby",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(fake.gotURL, "ashby") {
		t.Errorf("requested URL %q should target the board", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "job-uuid" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "job-uuid")
	}
	if j.Title != "Platform Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.ashbyhq.com/ashby/job-uuid" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Ashby" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "San Francisco" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "<strong>platform</strong>") {
		t.Errorf("Description should be the sanitized descriptionHtml, got %q", j.Description)
	}
	if strings.Contains(j.Description, "<script") {
		t.Errorf("Description retained a script tag, got %q", j.Description)
	}
	// Remote unions Ashby's explicit isRemote flag with the location heuristic (see
	// MapAshbyPosting); here the flag alone sets it.
	if !j.Remote {
		t.Error("Remote = false, want true from the explicit isRemote field")
	}
	// The explicit isRemote flag also yields a structured work mode.
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote from the explicit isRemote field", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want parsed publishedAt with milliseconds")
	}
}
