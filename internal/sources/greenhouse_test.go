package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGreenhouseProvider(t *testing.T) {
	if got := NewGreenhouse(nil).Provider(); got != "greenhouse" {
		t.Errorf("Provider() = %q, want %q", got, "greenhouse")
	}
}

func TestGreenhouseFetchDecodesAndSanitizesContent(t *testing.T) {
	// Greenhouse delivers `content` as entity-encoded HTML.
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 1,
				"title": "Data Engineer",
				"absolute_url": "https://boards.greenhouse.io/dropbox/jobs/1",
				"location": {"name": "Remote"},
				"content": "&lt;h2&gt;Role&lt;/h2&gt;&lt;p&gt;Build pipelines&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;"
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dropbox", Provider: "greenhouse", Board: "dropbox",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got := jobs[0].Description
	if !strings.Contains(got, "<h2>Role</h2>") || !strings.Contains(got, "<p>Build pipelines</p>") {
		t.Errorf("Description should be decoded HTML, got: %s", got)
	}
	if strings.Contains(got, "&lt;") {
		t.Errorf("Description still entity-encoded, got: %s", got)
	}
	if strings.Contains(got, "<script") {
		t.Errorf("Description retained a script tag, got: %s", got)
	}
}

func TestGreenhouseFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 123,
				"title": "Senior Go Developer",
				"absolute_url": "https://boards.greenhouse.io/gitlab/jobs/123",
				"updated_at": "2024-01-15T10:00:00Z",
				"location": {"name": "Remote - US"},
				"content": "<p>Build things</p>"
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "GitLab", Provider: "greenhouse", Board: "gitlab",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(fake.gotURL, "gitlab") || !strings.Contains(fake.gotURL, "content=true") {
		t.Errorf("requested URL %q should target the board with content=true", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "123" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "123")
	}
	if j.Title != "Senior Go Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://boards.greenhouse.io/gitlab/jobs/123" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "GitLab" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Remote - US" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "<p>Build things</p>" {
		t.Errorf("Description = %q", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true for a Remote location")
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want parsed updated_at")
	}
}

// Live-verified shapes (2026-08-13): 1800contacts/solidpower/carrotfertility all
// carry an "Employment Type" metadata field; coherentsolutions carries one with an
// empty-array value (unset); most boards (e.g. 1910genetics) carry metadata: null.
func TestGreenhouseMetadataEmploymentType(t *testing.T) {
	cases := []struct {
		name     string
		metadata []GreenhouseMetadataField
		want     string
	}{
		{"no metadata field", nil, ""},
		{
			"unrelated field only",
			[]GreenhouseMetadataField{{Name: "Headcount #", Value: json.RawMessage(`null`)}},
			"",
		},
		{
			"Full-time value",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`"Full-time"`)}},
			"full_time",
		},
		{
			"case/whitespace-insensitive field name",
			[]GreenhouseMetadataField{{Name: " employment type ", Value: json.RawMessage(`"Part-Time"`)}},
			"part_time",
		},
		{
			"empty-array value (unset multi-select) reads as absent",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`[]`)}},
			"",
		},
		{
			"unrecognized value maps to empty, not a guess",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`"Variable Hour"`)}},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := greenhouseMetadataEmploymentType(c.metadata); got != c.want {
				t.Errorf("greenhouseMetadataEmploymentType() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGreenhouseFetchReadsEmploymentTypeMetadata(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 1,
				"title": "Accountant",
				"location": {"name": "Remote"},
				"content": "",
				"metadata": [
					{"id": 1, "name": "Employment Type", "value": "Full-time", "value_type": "single_select"}
				]
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "greenhouse", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := jobs[0].EmploymentType; got != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", got)
	}
}
