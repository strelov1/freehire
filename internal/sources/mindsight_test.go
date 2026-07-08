package sources

import (
	"context"
	"strings"
	"testing"
)

func TestMindsightPostedDateFallsBackWhenStartAtUnusable(t *testing.T) {
	// external_publication_start_at is a future instant (a scheduled publication), which
	// NotFuture drops to nil; the posted date must fall back to created_at rather than
	// leaving the job undated.
	listing := `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"publicJobPostings":[
		{"id":7,"name":"Scheduled Role","status":"IN_PROGRESS","work_model":"REMOTE",
		 "country":"BR","state":"SP","city":"São Paulo",
		 "external_publication_start_at":"2099-01-01T00:00:00Z","created_at":"2026-06-10T00:00:00Z"}
	]}}}</script></html>`
	detail := `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"jobPosting":{"id":7,"description":"<p>x</p>"}}}}</script></html>`

	fake := (&routedHTTP{}).route("/acme/7", detail).route("/acme", listing)
	jobs, err := NewMindsight(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "mindsight", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].PostedAt == nil || jobs[0].PostedAt.Year() != 2026 || jobs[0].PostedAt.Month() != 6 {
		t.Errorf("PostedAt = %v, want created_at 2026-06 (future start_at dropped)", jobs[0].PostedAt)
	}
}

func TestMindsightProvider(t *testing.T) {
	if got := NewMindsight(nil).Provider(); got != "mindsight" {
		t.Errorf("Provider() = %q, want %q", got, "mindsight")
	}
}

func TestMindsightFetchListsAndEnrichesDetail(t *testing.T) {
	// The listing embeds publicJobPostings in __NEXT_DATA__ (structured fields, no body);
	// each posting's detail page embeds jobPosting.description. Job 16 is open and enriched;
	// job 99 is not IN_PROGRESS and is dropped.
	listing := `<html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"publicJobPostings":[
		{"id":16,"name":"  Backend Engineer  ","status":"IN_PROGRESS","work_model":"REMOTE",
		 "country":"BR","state":"SP","city":"Rio Claro",
		 "external_publication_start_at":"2026-06-16T00:00:00Z","created_at":"2026-06-10T00:00:00Z"},
		{"id":99,"name":"Closed Role","status":"FINISHED","work_model":"IN_PERSON",
		 "country":"BR","state":"SP","city":"São Paulo","created_at":"2026-06-01T00:00:00Z"}
	]}}}</script></body></html>`

	detail16 := `<html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"jobPosting":{"id":16,
		"description":"<p>Build APIs</p><script>evil()</script>"}}}}</script></body></html>`

	fake := (&routedHTTP{}).
		route("/grupomngt/16", detail16).
		route("/grupomngt", listing)

	jobs, err := NewMindsight(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Grupo MNGT", Provider: "mindsight", Board: "grupomngt",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (closed job dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "16" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "16")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want trimmed", j.Title)
	}
	if j.Company != "Grupo MNGT" {
		t.Errorf("Company = %q, want %q", j.Company, "Grupo MNGT")
	}
	if j.URL != "https://oportunidades.mindsight.com.br/grupomngt/16" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Rio Claro, SP, BR" {
		t.Errorf("Location = %q, want %q", j.Location, "Rio Claro, SP, BR")
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote", j.Remote, j.WorkMode)
	}
	if !strings.Contains(j.Description, "Build APIs") {
		t.Errorf("Description missing detail body: %q", j.Description)
	}
	if strings.Contains(j.Description, "evil") || strings.Contains(j.Description, "<script") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	// external_publication_start_at is preferred over created_at for the posted date.
	if j.PostedAt == nil || j.PostedAt.Month() != 6 || j.PostedAt.Day() != 16 {
		t.Errorf("PostedAt = %v, want 2026-06-16 (start_at, not created_at)", j.PostedAt)
	}
}
