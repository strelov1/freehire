package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestVentionFetchListAndMaps(t *testing.T) {
	page0 := `{"meta":{"total_count":1},"items":[{
		"id":1652,"title":"Python Intern",
		"meta":{"slug":"internship-python-krakow","html_url":"https://join.ventionteams.com/job-openings/internship-python-krakow","first_published_at":"2026-06-14T18:39:35+00:00"},
		"city":{"name":"Krakow","country":{"name":"Poland"}},
		"work_format":"remote",
		"responsibilities":"<p>Do <b>work</b>.</p>","required_skills":"<p>Python</p>","benefits":"<p>perks</p>","about":"<script>x</script><p>us</p>"
	}]}`
	fake := (&routedHTTP{}).route("offset=0", page0)

	jobs, err := NewVention(fake).Fetch(context.Background(), CompanyEntry{Company: "Vention", Board: "join.ventionteams.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1652" {
		t.Errorf("ExternalID = %q, want 1652", j.ExternalID)
	}
	if j.Title != "Python Intern" || j.Company != "Vention" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.URL != "https://join.ventionteams.com/job-openings/internship-python-krakow" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Krakow, Poland" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote", j.Remote, j.WorkMode)
	}
	// All four HTML blocks concatenated + sanitized (script stripped).
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<b>work</b>") || !strings.Contains(j.Description, "Python") ||
		!strings.Contains(j.Description, "perks") || !strings.Contains(j.Description, "us") {
		t.Errorf("Description incomplete/unsanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 14, 18, 39, 35, 0, time.UTC)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestVentionProvider(t *testing.T) {
	if got := NewVention(nil).Provider(); got != "vention" {
		t.Errorf("Provider() = %q, want vention", got)
	}
}

func TestVentionRegisteredInAll(t *testing.T) {
	if s, ok := All(nil)["vention"]; !ok || s.Provider() != "vention" {
		t.Fatal("All() missing provider vention")
	}
}
