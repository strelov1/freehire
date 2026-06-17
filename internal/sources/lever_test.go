package sources

import (
	"context"
	"strings"
	"testing"
)

func TestLeverProvider(t *testing.T) {
	if got := NewLever(nil).Provider(); got != "lever" {
		t.Errorf("Provider() = %q, want %q", got, "lever")
	}
}

func TestLeverFetch(t *testing.T) {
	fake := &fakeHTTP{body: `[
		{
			"id": "abc-123",
			"text": "Backend Engineer",
			"hostedUrl": "https://jobs.lever.co/lever/abc-123",
			"createdAt": 1705312800000,
			"categories": {"location": "Remote"},
			"workplaceType": "hybrid",
			"description": "<p>Write Go.</p>"
		}
	]`}

	jobs, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Lever", Provider: "lever", Board: "lever",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(fake.gotURL, "lever") {
		t.Errorf("requested URL %q should target the board", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from Lever's workplaceType", j.WorkMode)
	}
	if j.ExternalID != "abc-123" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "abc-123")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.lever.co/lever/abc-123" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Lever" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Remote" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "<p>Write Go.</p>") {
		t.Errorf("Description = %q, want the opening HTML", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true")
	}
	if j.PostedAt == nil {
		t.Fatal("PostedAt = nil, want parsed createdAt")
	}
	if got := j.PostedAt.UTC().Year(); got != 2024 {
		t.Errorf("PostedAt year = %d, want 2024", got)
	}
}

func TestLeverFetchAssemblesBodyFromAllFields(t *testing.T) {
	// Lever splits the body across description + lists + additional. The plain mirror
	// is often empty even when the HTML fields carry the real content.
	fake := &fakeHTTP{body: `[
		{
			"id": "p1",
			"text": "Partner Marketing Manager",
			"hostedUrl": "https://jobs.lever.co/spotify/p1",
			"categories": {"location": "Mumbai"},
			"descriptionPlain": "",
			"description": "<div><p>Spotify is looking for a manager.</p></div>",
			"lists": [
				{"text": "What You'll Do", "content": "<li>Lead partnerships</li>"},
				{"text": "Who You Are", "content": "<li>5+ years experience</li>"}
			],
			"additional": "<p>Spotify is an equal opportunity employer.</p>"
		}
	]`}

	jobs, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Spotify", Provider: "lever", Board: "spotify",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got := jobs[0].Description
	for _, want := range []string{
		"Spotify is looking for a manager.",
		"<h3>", // list headings are wrapped as headings
		"Who You Are",
		"Lead partnerships",
		"5+ years experience",
		"equal opportunity employer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("assembled description missing %q\ngot: %s", want, got)
		}
	}
}

func TestLeverFetchHandlesEmptyHeadingsAndFields(t *testing.T) {
	// Only lists carry content (the bug case: empty description/additional), and one
	// list has a blank heading.
	fake := &fakeHTTP{body: `[
		{
			"id": "p2",
			"text": "Engineer",
			"hostedUrl": "https://jobs.lever.co/acme/p2",
			"categories": {"location": "Remote"},
			"description": "",
			"additional": "",
			"lists": [
				{"text": "", "content": "<li>Build things</li>"}
			]
		}
	]`}

	jobs, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "lever", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got := jobs[0].Description
	if !strings.Contains(got, "Build things") {
		t.Errorf("content from a headingless list was dropped\ngot: %s", got)
	}
	if strings.Contains(got, "<h3></h3>") {
		t.Errorf("emitted an empty heading for a blank list title\ngot: %s", got)
	}
}

func TestLeverFetchDefaultsToGlobalHost(t *testing.T) {
	// No region set: the board lives on Lever's default (US) API host.
	fake := &fakeHTTP{body: `[]`}
	if _, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Smile.io", Provider: "lever", Board: "Smile.io",
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasPrefix(fake.gotURL, "https://api.lever.co/v0/postings/") {
		t.Errorf("default board should hit api.lever.co, got %q", fake.gotURL)
	}
}

func TestLeverFetchEURegionUsesEUHost(t *testing.T) {
	// region: eu selects Lever's EU data-residency host (e.g. XM, Silverfin live there;
	// their boards 404 on the default host).
	fake := &fakeHTTP{body: `[]`}
	if _, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{
		Company: "XM", Provider: "lever", Board: "xm", Region: "eu",
	}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasPrefix(fake.gotURL, "https://api.eu.lever.co/v0/postings/xm") {
		t.Errorf("eu-region board should hit api.eu.lever.co, got %q", fake.gotURL)
	}
}
