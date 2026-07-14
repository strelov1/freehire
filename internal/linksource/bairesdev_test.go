package linksource

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

// bairesDevPostingJSON mirrors the public JobPosting bridge response: plain schema.org
// JSON-LD returned directly (not embedded in HTML), keyed by the numeric job id. The
// timestamp carries no timezone and the description is untrusted third-party text.
const bairesDevPostingJSON = `{"@context":"https://schema.org","@type":"JobPosting",` +
	`"title":"Sales Director (Retail Background) - Remote Work",` +
	`"description":"<p>Own the deal.</p><script>evil()</script>",` +
	`"identifier":{"@type":"PropertyValue","name":"Job ID","value":284579},` +
	`"datePosted":"2025-06-02T10:23:21.503","validThrough":"2026-10-12T12:40:14.313",` +
	`"jobLocationType":"TELECOMMUTE","employmentType":"FULL_TIME",` +
	`"hiringOrganization":{"@type":"Organization","name":"BairesDev","sameAs":"https://www.bairesdev.com"}}`

// bairesDevJobJSON is the apply-flow endpoint's payload: the FULL HTML description (the JobPosting
// block above carries only a teaser).
const bairesDevJobJSON = `{"jobResults":[{"title":"Sales Director (Retail Background) - Remote Work",` +
	`"description":"<h3>Own the deal.</h3><p>The full role responsibilities.</p><script>evil()</script>"}]}`

func TestBairesDevResolvesFromApplyLink(t *testing.T) {
	// A real apply link carries a career-site id, the job id, and tracking query params.
	const link = "https://applicants.bairesdev.com/job/97/284579/apply?_gl=1*2opsly&lang=en"
	c := (&fakeClient{}).
		route("JobPostingId=284579", bairesDevPostingJSON, "").
		route("JobOfferId=284579", bairesDevJobJSON, "")

	job, ok, err := NewBairesDev(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	// External id is the job id alone (career-site id is just the link's entry point), so
	// the same posting dedups regardless of which career link brought it in.
	if job.ExternalID != "284579" {
		t.Errorf("ExternalID = %q, want the bare job id", job.ExternalID)
	}
	// The stored URL is the canonical apply link with tracking params stripped.
	if job.URL != "https://applicants.bairesdev.com/job/97/284579/apply" {
		t.Errorf("URL = %q, want canonical apply link without tracking", job.URL)
	}
	if job.Title != "Sales Director (Retail Background) - Remote Work" {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "BairesDev" {
		t.Errorf("Company = %q, want BairesDev", job.Company)
	}
	if !job.Remote {
		t.Error("Remote = false, want true for TELECOMMUTE")
	}
	// The FULL description (Job endpoint) is used and sanitized, not the JobPosting teaser.
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "The full role responsibilities.") {
		t.Errorf("Description not the sanitized full text: %q", job.Description)
	}
	// datePosted has no timezone, so it falls back to the date alone (posted_at is approximate).
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-06-02 (date-only fallback)", job.PostedAt)
	}
}

func TestBairesDevMatch(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://applicants.bairesdev.com/job/97/284579/apply", true},
		{"https://applicants.bairesdev.com/job/111/176816", true}, // no /apply suffix
		{"https://applicants.bairesdev.com/job/97/284579/apply?lang=en", true},
		{"https://applicants.bairesdev.com/openings", false},     // listing, not a job
		{"https://applicants.bairesdev.com/job/97/apply", false}, // missing job id
		{"https://talent.bairesdev.com/pt/vagas/arquiteto-node-remote", false},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.raw, err)
		}
		if got := (bairesdev{}).Match(u); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestBairesDevSkipsWhenPostingGone(t *testing.T) {
	// The bridge answers 200 with an empty body for a stale/invalid id — a matched link
	// that is no longer a live vacancy, so skip (ok=false) rather than error.
	const link = "https://applicants.bairesdev.com/job/97/999999/apply"
	c := (&fakeClient{}).route("JobPostingId=999999", `{}`, "")

	_, ok, err := NewBairesDev(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ok {
		t.Error("ok=true, want skip for a posting no longer live")
	}
}
