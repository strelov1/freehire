package sources

import (
	"context"
	"strings"
	"testing"
)

// apploiListJSON is an api.apploi.com/v1/jobs?employer=<id> page: one live posting plus an
// archived one (which the adapter must drop), with the description inline (no detail fetch).
const apploiListJSON = `{"data":[
{"id":"1882468","name":"Car Rental Cleaner","description":"<p>Clean cars.</p><script>x()<\/script>","city":"Koloa","state":"Hawaii","country":"United States","published_date":"2026-06-18T17:23:00+00:00","brand_name_with_company_only":"OnTray","published":true,"archived":false,"private":false},
{"id":"9999","name":"Archived Role","published":true,"archived":true,"private":false}
],"limit":100,"offset":0}`

func TestApploiProvider(t *testing.T) {
	if got := NewApploi(nil).Provider(); got != "apploi" {
		t.Errorf("Provider() = %q, want %q", got, "apploi")
	}
}

func TestApploiFetch(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/jobs", apploiListJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (archived posting must be dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1882468" {
		t.Errorf("external_id = %q", j.ExternalID)
	}
	if j.Title != "Car Rental Cleaner" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Company != "OnTray" {
		t.Errorf("company = %q", j.Company)
	}
	if j.Location != "Koloa, Hawaii, United States" {
		t.Errorf("location = %q", j.Location)
	}
	if j.URL != "https://jobs.apploi.com/view/1882468" {
		t.Errorf("url = %q", j.URL)
	}
	if j.PostedAt == nil {
		t.Error("posted_at not parsed")
	}
	if !strings.Contains(j.Description, "Clean cars") || strings.Contains(j.Description, "x()") {
		t.Errorf("description not sanitized: %q", j.Description)
	}
}

// An employer with no live openings returns an empty data array, yielding zero jobs and no
// error (not a board-level failure).
func TestApploiFetchEmpty(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/jobs", `{"data":[],"limit":100,"offset":0}`)
	jobs, err := NewApploi(fake).Fetch(context.Background(), CompanyEntry{Board: "1"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}
