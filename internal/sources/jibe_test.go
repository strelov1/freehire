package sources

import (
	"context"
	"strings"
	"testing"
)

func TestJibeProvider(t *testing.T) {
	if got := NewJibe(nil).Provider(); got != "jibe" {
		t.Errorf("Provider() = %q, want %q", got, "jibe")
	}
}

// jibeJob renders one /api/jobs "data" object with the fields the adapter reads.
func jibeJob(slug, title, locName, locType, org, posted string) string {
	return `{"data": {
		"slug": "` + slug + `",
		"req_id": "` + slug + `",
		"title": "` + title + `",
		"description": "<strong>About</strong><br>` + title + ` does the work.",
		"location_name": "` + locName + `",
		"full_location": "United States",
		"location_type": "` + locType + `",
		"hiring_organization": "` + org + `",
		"posted_date": "` + posted + `"
	}}`
}

func TestJibeFetchPaginatesAndMaps(t *testing.T) {
	// totalCount 3 forces a second page: page 1 returns 2, page 2 the last.
	fake := (&routedHTTP{}).
		route("page=1", `{"totalCount": 3, "jobs": [`+
			jibeJob("5441", "Software Engineer III", "US Remote", "ANY", "GitHub, Inc.", "2026-06-16T20:41:00+0000")+`,`+
			jibeJob("5500", "Hybrid Role", "Berlin", "HYBRID", "GitHub, Inc.", "2026-06-15T10:00:00+0000")+
			`]}`).
		route("page=2", `{"totalCount": 3, "jobs": [`+
			jibeJob("5600", "Onsite Role", "London", "REMOTE", "", "2026-06-14T09:30:00+0000")+
			`]}`)

	jobs, err := NewJibe(fake).Fetch(context.Background(), CompanyEntry{
		Company: "GitHub", Provider: "jibe", Board: "www.github.careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["5441"]
	if j.Title != "Software Engineer III" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://www.github.careers/careers-home/jobs/5441" {
		t.Errorf("URL = %q, want the public job page built from board+slug", j.URL)
	}
	if j.Company != "GitHub, Inc." {
		t.Errorf("Company = %q, want the posting's hiring_organization", j.Company)
	}
	if j.Location != "US Remote" {
		t.Errorf("Location = %q, want location_name", j.Location)
	}
	// location_type "ANY" gives no structured mode, but the "US Remote" string flags remote.
	if j.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty for location_type ANY", j.WorkMode)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from the location string heuristic")
	}
	if !strings.Contains(j.Description, "does the work") {
		t.Errorf("Description = %q, want the posting body", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed posted_date (2026)", j.PostedAt)
	}

	// HYBRID/REMOTE location_type map to the structured work mode.
	if got := byID["5500"].WorkMode; got != "hybrid" {
		t.Errorf("WorkMode(5500) = %q, want hybrid", got)
	}
	if got := byID["5600"].WorkMode; got != "remote" {
		t.Errorf("WorkMode(5600) = %q, want remote", got)
	}

	// hiring_organization empty -> fall back to the configured company.
	if got := byID["5600"].Company; got != "GitHub" {
		t.Errorf("Company(5600) = %q, want configured company fallback", got)
	}
}

func TestJibeDropsPostingWithoutID(t *testing.T) {
	// A posting with neither slug nor req_id has no dedup key; emitting it would give an
	// empty ExternalID and a bare ".../jobs/" URL that collides with every other id-less
	// posting. It must be dropped, like every sibling adapter does.
	fake := (&routedHTTP{}).
		route("page=1", `{"totalCount": 2, "jobs": [`+
			jibeJob("5441", "Valid Role", "US Remote", "REMOTE", "GitHub, Inc.", "2026-06-16T20:41:00+0000")+`,`+
			jibeJob("", "No ID Role", "Berlin", "ONSITE", "GitHub, Inc.", "2026-06-15T10:00:00+0000")+
			`]}`).
		route("page=2", `{"totalCount": 2, "jobs": []}`)

	jobs, err := NewJibe(fake).Fetch(context.Background(), CompanyEntry{
		Company: "GitHub", Provider: "jibe", Board: "www.github.careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (the id-less posting must drop)", len(jobs))
	}
	if jobs[0].ExternalID != "5441" {
		t.Errorf("ExternalID = %q, want the only posting with an id", jobs[0].ExternalID)
	}
}
