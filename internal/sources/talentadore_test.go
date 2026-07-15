package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestTalentAdoreProvider(t *testing.T) {
	if got := NewTalentAdore(nil).Provider(); got != "talentadore" {
		t.Errorf("Provider() = %q, want %q", got, "talentadore")
	}
}

func TestTalentAdoreFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"company": "Acme",
		"jobs": [
			{
				"id": "yjMOo",
				"job_token": "mWXEgr",
				"name": "AI Engineer",
				"link": "https://ats.talentadore.com/apply/ai-engineer/mWXEgr",
				"description_html": "<p>Build models.</p><script>alert(1)</script>",
				"description_text": "Build models.",
				"start_date": "2026-07-10T12:16:20Z",
				"updated": "2026-07-11T09:00:00Z",
				"city": "Helsinki",
				"county": "",
				"country": "Finland",
				"location": "Pasilankatu 2 A"
			},
			{
				"id": "RyGxO",
				"job_token": "m5n9e1",
				"name": "Remote Designer",
				"link": "https://ats.talentadore.com/apply/remote-designer/m5n9e1",
				"description_html": "",
				"description_text": "Design things.",
				"start_date": "",
				"updated": "2026-07-09T08:00:00Z",
				"city": "",
				"county": "",
				"country": "Remote"
			}
		]
	}`}

	jobs, err := NewTalentAdore(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "talentadore", Board: "9wmfASE",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "/positions/9wmfASE/json") {
		t.Errorf("requested URL %q should target the board positions feed", fake.gotURL)
	}
	// v=2 is load-bearing: it makes the feed emit RFC3339 (Z) dates the bare endpoint omits.
	if !strings.Contains(fake.gotURL, "v=2") {
		t.Errorf("requested URL %q must carry v=2 for RFC3339 dates", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "mWXEgr" {
		t.Errorf("ExternalID = %q, want the job_token", j.ExternalID)
	}
	if j.URL != "https://ats.talentadore.com/apply/ai-engineer/mWXEgr" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "AI Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Helsinki, Finland" {
		t.Errorf("Location = %q, want city+country (street omitted)", j.Location)
	}
	if j.Remote {
		t.Error("Remote = true, want false for a Helsinki posting")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build models.") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Day() != 10 {
		t.Errorf("PostedAt = %v, want start_date (2026-07-10)", j.PostedAt)
	}

	// Second posting: empty description_html falls back to description_text; a "Remote"
	// country flags remote; a missing start_date falls back to updated.
	r := jobs[1]
	if !strings.Contains(r.Description, "Design things.") {
		t.Errorf("Description = %q, want description_text fallback", r.Description)
	}
	if !r.Remote {
		t.Error("Remote = false, want true from the Remote location")
	}
	if r.PostedAt == nil || r.PostedAt.UTC().Day() != 9 {
		t.Errorf("PostedAt = %v, want updated fallback (2026-07-09)", r.PostedAt)
	}
}

func TestTalentAdoreSkipsMissingToken(t *testing.T) {
	fake := &fakeHTTP{body: `{"jobs":[
		{"job_token":"","name":"No Token","link":"x"},
		{"job_token":"keep","name":"Kept","link":"y"}
	]}`}

	jobs, err := NewTalentAdore(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "keep" {
		t.Fatalf("got %v, want only the posting with a job_token", jobs)
	}
}

func TestTalentAdoreRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["talentadore"]
	if !ok {
		t.Fatal("All() missing provider talentadore")
	}
	if s.Provider() != "talentadore" {
		t.Errorf("All()[talentadore].Provider() = %q", s.Provider())
	}
	// Board-based (per-employer feed token): stays in the source facet.
	if !slices.Contains(FilterableProviders(), "talentadore") {
		t.Error("FilterableProviders() should include board-based talentadore")
	}
}
