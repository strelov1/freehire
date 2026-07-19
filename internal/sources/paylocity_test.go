package sources

import (
	"context"
	"strings"
	"testing"
)

// paylocityListingHTML is a recruiting.paylocity.com /Recruiting/Jobs/All/<guid> page: the
// job list rides in the window.pageData blob's Jobs[] array (id/title/location/date/remote).
const paylocityListingHTML = `<html><body>
<script>window.pageData = {"CookieBannerScriptSource":"x","Departments":["All Departments"],"IsLeadJoinEnabled":true,"Jobs":[
{"JobId":4026535,"JobTitle":"Technician Aide","LocationName":"CBAH - Airport Rd.","PublishedDate":"2026-06-29T11:55:56-05:00","IsRemote":false},
{"JobId":3836522,"JobTitle":"Associate Veterinarian","LocationName":"Remote","PublishedDate":"2026-03-19T16:53:34-05:00","IsRemote":true}
]};</script>
</body></html>`

// paylocityDetailHTML is a /Recruiting/Jobs/Details/<id> page. Paylocity dropped the
// schema.org ld+json (the page is now a client-rendered shell), but the description still
// renders server-side as the <div> following the "Description" section header. A trailing
// <script> in that block must be stripped by sanitizeHTML; the sibling "Job Type" section
// proves the selector picks the Description block, not the first header.
const paylocityDetailHTML = `<html><body>
<div class="job-preview-details">
  <div class="vertical-padding">
    <div class="job-listing-header">Job Type</div>
    <div>Full-time</div>
  </div>
  <div class="job-listing-header">Description</div>
  <div><h2>About</h2><p>Care for animals.</p><script>alert(1)</script></div>
</div>
</body></html>`

func TestPaylocityProvider(t *testing.T) {
	if got := NewPaylocity(nil).Provider(); got != "paylocity" {
		t.Errorf("Provider() = %q, want %q", got, "paylocity")
	}
}

func TestPaylocityFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Recruiting/Jobs/All/", paylocityListingHTML).
		route("/Recruiting/Jobs/Details/", paylocityDetailHTML)

	jobs, err := NewPaylocity(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Clover Basin Animal Hospital", Board: "1a06dc72-45ee-4c90-a268-fe881bbeb577"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}

	// fetchDetails fans out concurrently, so index is not stable — key by ExternalID.
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	tech, ok := byID["4026535"]
	if !ok {
		t.Fatalf("missing job 4026535; got %v", byID)
	}
	if tech.Title != "Technician Aide" {
		t.Errorf("title = %q", tech.Title)
	}
	if tech.Location != "CBAH - Airport Rd." {
		t.Errorf("location = %q", tech.Location)
	}
	if tech.Company != "Clover Basin Animal Hospital" {
		t.Errorf("company = %q", tech.Company)
	}
	if tech.PostedAt == nil {
		t.Error("posted_at not parsed")
	}
	if tech.Remote {
		t.Error("job wrongly flagged remote")
	}
	if !strings.Contains(tech.Description, "Care for animals") || strings.Contains(tech.Description, "alert(1)") {
		t.Errorf("description not sanitized ld+json body: %q", tech.Description)
	}
	if tech.URL != "https://recruiting.paylocity.com/Recruiting/Jobs/Details/4026535" {
		t.Errorf("url = %q", tech.URL)
	}

	vet := byID["3836522"]
	if !vet.Remote {
		t.Error("IsRemote job not flagged remote")
	}
}

// A board with no openings renders an empty Jobs[] array, which must yield zero jobs
// without an error (not a board-level failure).
func TestPaylocityFetchEmpty(t *testing.T) {
	empty := `<html><body><script>window.pageData = {"Jobs":[]};</script></body></html>`
	fake := (&routedHTTP{}).route("/Recruiting/Jobs/All/", empty)
	jobs, err := NewPaylocity(fake).Fetch(context.Background(), CompanyEntry{Board: "guid"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}
