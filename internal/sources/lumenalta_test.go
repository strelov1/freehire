package sources

import (
	"context"
	"strings"
	"testing"
)

func TestLumenaltaProvider(t *testing.T) {
	if got := NewLumenalta(nil).Provider(); got != "lumenalta" {
		t.Errorf("Provider() = %q, want %q", got, "lumenalta")
	}
}

func TestLumenaltaIsBoardless(t *testing.T) {
	if _, ok := NewLumenalta(nil).(boardless); !ok {
		t.Error("Lumenalta is a single-company source and must be boardless")
	}
}

func TestLumenaltaFetch(t *testing.T) {
	// One page whose meta.total matches its data length, so the pager stops after it.
	fake := &fakeHTTP{body: `{
		"data": [
			{
				"_id": "68dee35f8c9481db144ea376",
				"slug": "ai-engineer-ai-engineer-551",
				"name": "Senior AI Engineer",
				"description": "  At Lumenalta, we build technology that scales.  "
			}
		],
		"meta": {"page": 1, "limit": 100, "total": 1}
	}`}

	jobs, err := NewLumenalta(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Lumenalta", Provider: "lumenalta",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "lumenalta.com/api/jobs") {
		t.Errorf("requested URL %q should target the jobs API", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "68dee35f8c9481db144ea376" {
		t.Errorf("ExternalID = %q, want the Mongo _id", j.ExternalID)
	}
	if j.URL != "https://lumenalta.com/careers/ai-engineer-ai-engineer-551" {
		t.Errorf("URL = %q, want the careers page built from slug", j.URL)
	}
	if j.Title != "Senior AI Engineer" {
		t.Errorf("Title = %q, want name", j.Title)
	}
	if j.Company != "Lumenalta" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	// The plain-text description is rebuilt into structural HTML for the {@html} consumer.
	if j.Description != "<p>At Lumenalta, we build technology that scales.</p>" {
		t.Errorf("Description = %q, want the plain text wrapped as a paragraph", j.Description)
	}
	// Lumenalta is a remote-only consultancy; the API carries no location field.
	if j.Location != "Remote" || !j.Remote {
		t.Errorf("Location/Remote = %q/%v, want Remote/true", j.Location, j.Remote)
	}
}

func TestLumenaltaDescriptionRebuiltAsSafeHTML(t *testing.T) {
	// The list description is plain text with blank-line blocks and bullet lines. It must be
	// rebuilt into structural HTML (paragraphs + list) so {@html} renders it, not one collapsed
	// line, AND sanitized so a stray tag in the plain text cannot inject active content.
	fake := &fakeHTTP{body: `{
		"data": [{
			"_id": "1",
			"slug": "role",
			"name": "Engineer",
			"description": "We build things.\n\n- Ship code\n- Break <script>alert(1)</script>"
		}],
		"meta": {"total": 1}
	}`}

	jobs, err := NewLumenalta(fake).Fetch(context.Background(), CompanyEntry{Company: "Lumenalta", Provider: "lumenalta"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	d := jobs[0].Description
	if !strings.Contains(d, "<p>We build things.</p>") {
		t.Errorf("Description = %q, want the first block as a <p> paragraph", d)
	}
	if !strings.Contains(d, "<li>Ship code</li>") {
		t.Errorf("Description = %q, want bullet lines rebuilt as <li>", d)
	}
	if strings.Contains(d, "<script>") {
		t.Errorf("Description = %q, must not carry active <script> content", d)
	}
}
