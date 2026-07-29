package linksource

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ashbyBoardJSON mirrors the per-board posting API (no company name; the linked job is
// found by its UUID among the board's jobs).
const ashbyBoardJSON = `{"apiVersion":"1","jobs":[
 {"id":"00000000-0000-0000-0000-000000000000","title":"Other Role","location":"NY","jobUrl":"https://jobs.ashbyhq.com/ruby-labs/00000000-0000-0000-0000-000000000000","publishedAt":"2025-01-01T00:00:00.000+00:00","descriptionHtml":"<p>x</p>","isRemote":false},
 {"id":"62661b07-ac6b-4283-ae38-6c3255c47bd4","title":"Head of User Acquisition","location":"Ukraine","jobUrl":"https://jobs.ashbyhq.com/ruby-labs/62661b07-ac6b-4283-ae38-6c3255c47bd4","publishedAt":"2025-05-05T14:02:49.272+00:00","descriptionHtml":"<p>Lead it.</p><script>evil()</script>","isRemote":true}
]}`

func TestAshbyResolvesAlignedIdentity(t *testing.T) {
	const link = "https://jobs.ashbyhq.com/ruby-labs/62661b07-ac6b-4283-ae38-6c3255c47bd4?utm_source=telegram"
	c := (&fakeClient{}).route("api.ashbyhq.com/posting-api/job-board/ruby-labs", ashbyBoardJSON, "")

	job, ok, err := NewAshby(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	if job.ExternalID != "ruby-labs:62661b07-ac6b-4283-ae38-6c3255c47bd4" {
		t.Errorf("ExternalID = %q, want board-namespaced uuid", job.ExternalID)
	}
	if job.Title != "Head of User Acquisition" {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "Ruby Labs" {
		t.Errorf("Company = %q, want Ruby Labs (humanized board slug)", job.Company)
	}
	if !job.Remote {
		t.Error("Remote = false, want true")
	}
	if job.Location != "Ukraine" {
		t.Errorf("Location = %q", job.Location)
	}
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "Lead it.") {
		t.Errorf("Description not sanitized: %q", job.Description)
	}
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2025, 5, 5, 14, 2, 49, 272000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-05-05T14:02:49.272Z (fractional seconds)", job.PostedAt)
	}
}

func TestAshbySkipsWhenJobNotOnBoard(t *testing.T) {
	// A delisted job: matched host, board fetch succeeds, but the UUID is gone → skip.
	const link = "https://jobs.ashbyhq.com/ruby-labs/ffffffff-ffff-ffff-ffff-ffffffffffff"
	c := (&fakeClient{}).route("api.ashbyhq.com/posting-api/job-board/ruby-labs", ashbyBoardJSON, "")

	_, ok, err := NewAshby(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ok {
		t.Error("ok=true, want skip for a job no longer on the board")
	}
}

// Link resolution shares MapAshbyPosting with the ingest adapter, so a hybrid posting
// resolves to the same facets on both paths.
func TestAshbyResolvesHybridPostingAsNotRemote(t *testing.T) {
	const board = `{"apiVersion":"1","jobs":[
 {"id":"d675ca96-5678-4ed9-be94-01fa1e641f28","title":"Senior Web Designer","location":"Vilnius","jobUrl":"https://jobs.ashbyhq.com/surfshark/d675ca96-5678-4ed9-be94-01fa1e641f28","publishedAt":"2026-07-28T09:35:42.943+00:00","descriptionHtml":"<p>Design it.</p>","isRemote":true,"workplaceType":"Hybrid"}
]}`
	const link = "https://jobs.ashbyhq.com/surfshark/d675ca96-5678-4ed9-be94-01fa1e641f28"
	c := (&fakeClient{}).route("api.ashbyhq.com/posting-api/job-board/surfshark", board, "")

	job, ok, err := NewAshby(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	if job.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from workplaceType", job.WorkMode)
	}
	if job.Remote {
		t.Error("Remote = true, want false: a hybrid role is not remote")
	}
}
