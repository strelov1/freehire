package sources

import (
	"context"
	"strings"
	"testing"
)

func TestRecruiteeProvider(t *testing.T) {
	if got := NewRecruitee(nil).Provider(); got != "recruitee" {
		t.Errorf("Provider() = %q, want %q", got, "recruitee")
	}
}

func TestRecruiteeFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"offers": [
			{
				"id": 42,
				"title": "Game Director",
				"careers_url": "https://acme.recruitee.com/o/game-director",
				"location": "Warsaw, Poland",
				"created_at": "2024-04-24 10:13:38 UTC",
				"remote": true,
				"description": "<h4>The role</h4><p>Lead the team.</p>",
				"requirements": "<h4>Requirements</h4><ul><li>7+ years</li></ul>"
			},
			{
				"id": 43,
				"title": "Artist",
				"careers_url": "https://acme.recruitee.com/o/artist",
				"location": "Remote",
				"created_at": "2024-04-24 10:13:38 UTC",
				"remote": true,
				"description": "<p>Make art.</p>",
				"requirements": ""
			}
		]
	}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "recruitee", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "acme.recruitee.com") || !strings.Contains(fake.gotURL, "/offers") {
		t.Errorf("requested URL %q should target the board offers endpoint", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "42" {
		t.Errorf("ExternalID = %q, want the id", j.ExternalID)
	}
	if j.URL != "https://acme.recruitee.com/o/game-director" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Warsaw, Poland" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from the remote flag")
	}
	// Description combines description + requirements, sanitized.
	for _, want := range []string{"Lead the team.", "Requirements", "7+ years"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed created_at (2024)", j.PostedAt)
	}

	// Empty requirements must not break assembly.
	if !strings.Contains(jobs[1].Description, "Make art.") {
		t.Errorf("second job description = %q", jobs[1].Description)
	}
}

// Recruitee exposes remote and hybrid as separate booleans, and they are mutually
// exclusive: a hybrid offer carries remote=false, so reading the remote flag alone drops
// the hybrid signal entirely.
func TestRecruiteeFetchHybridOffer(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"offers": [
			{"id": 1, "title": "Designer", "careers_url": "https://acme.recruitee.com/o/designer",
			 "location": "Warszawa", "created_at": "2024-03-01 10:00:00 UTC",
			 "remote": false, "hybrid": true, "description": "<p>Design.</p>", "requirements": ""}
		]
	}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "recruitee", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	if jobs[0].WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from the hybrid flag", jobs[0].WorkMode)
	}
	if jobs[0].Remote {
		t.Error("Remote = true, want false: a hybrid offer is not remote")
	}
}
