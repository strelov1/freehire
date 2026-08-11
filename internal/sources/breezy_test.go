package sources

import (
	"context"
	"strings"
	"testing"
)

func TestBreezyProvider(t *testing.T) {
	if got := NewBreezy(nil).Provider(); got != "breezy" {
		t.Errorf("Provider() = %q, want %q", got, "breezy")
	}
}

// jobPostingHTML is a position page carrying the schema.org JobPosting ld+json block
// Breezy server-renders; the adapter reads only the description from it.
func jobPostingHTML(title, desc string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting","title":"` + title +
		`","description":"` + desc + `","datePosted":"2025-01-14"}` +
		`</script></head><body>page</body></html>`
}

func TestBreezyFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/p/52-senior-designer", jobPostingHTML("Senior Designer", "<p>Do the design.</p>")).
		route("/p/53-remote-engineer", jobPostingHTML("Remote Engineer", "<p>Work anywhere.</p>")).
		route("/json", `[
			{"id": "52", "name": "Senior Designer",
			 "url": "https://acme.breezy.hr/p/52-senior-designer",
			 "location": {"city": "Yerevan", "country": {"name": "Armenia", "id": "AM"}, "is_remote": false},
			 "type": {"id": "fullTime", "name": "Full-Time"},
			 "published_date": "2025-01-14T00:20:37.073Z"},
			{"id": "53", "name": "Remote Engineer",
			 "url": "https://acme.breezy.hr/p/53-remote-engineer",
			 "location": {"city": "", "country": {"name": "", "id": ""}, "is_remote": true},
			 "type": {"name": "Full-Time"},
			 "published_date": "2025-02-01T00:00:00.000Z"}
		]`)

	jobs, err := NewBreezy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "breezy", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["52"]
	if !ok {
		t.Fatal("job 52 missing")
	}
	if j.Title != "Senior Designer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://acme.breezy.hr/p/52-senior-designer" {
		t.Errorf("URL = %q, want the position page url from the list", j.URL)
	}
	if j.Location != "Yerevan, Armenia" {
		t.Errorf("Location = %q, want city/country joined from the list", j.Location)
	}
	if !strings.Contains(j.Description, "Do the design.") {
		t.Errorf("Description = %q, want the JSON-LD description", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true, want false from list is_remote")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2025 {
		t.Errorf("PostedAt = %v, want parsed published_date (2025)", j.PostedAt)
	}

	r := byID["53"]
	if !r.Remote {
		t.Error("job 53 is_remote=true should set Remote = true")
	}
	if r.Location != "" {
		t.Errorf("job 53 Location = %q, want empty (no city/country)", r.Location)
	}
}

func TestBreezyRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["breezy"]
	if !ok {
		t.Fatal("All() missing provider breezy")
	}
	if s.Provider() != "breezy" {
		t.Errorf("All()[breezy].Provider() = %q", s.Provider())
	}
}

func TestBreezyFetchSkipsPositionWithoutDescription(t *testing.T) {
	// id 53's page carries no JobPosting ld+json, so its detail fetch yields no
	// description and the posting is dropped (a description-less job is useless to
	// enrichment).
	fake := (&routedHTTP{}).
		route("/p/52-designer", jobPostingHTML("Designer", "<p>Real work.</p>")).
		route("/p/53-broken", `<html><head></head><body>no job posting here</body></html>`).
		route("/json", `[
			{"id": "52", "name": "Designer", "url": "https://acme.breezy.hr/p/52-designer",
			 "location": {"city": "Berlin", "country": {"name": "Germany"}, "is_remote": false},
			 "published_date": "2025-01-14T00:00:00.000Z"},
			{"id": "53", "name": "Broken", "url": "https://acme.breezy.hr/p/53-broken",
			 "location": {"city": "Berlin", "country": {"name": "Germany"}, "is_remote": false},
			 "published_date": "2025-01-14T00:00:00.000Z"}
		]`)

	jobs, err := NewBreezy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "breezy", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one description-less detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "52" {
		t.Fatalf("want only 52 to survive, got %d jobs", len(jobs))
	}
}
