package linksource

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// genericJobHTML is a top-level JobPosting ld+json block as Teamtailor/Breezy detail
// pages server-render it: a clean title/company, a TELECOMMUTE location type, and an
// HTML description with an entity and an embedded script to strip.
const genericJobHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Senior Backend Engineer (Java / Go)",
 "datePosted":"2026-07-01",
 "jobLocationType":"TELECOMMUTE",
 "description":"<p>Build &amp; scale.</p><script>evil()<\/script>",
 "hiringOrganization":{"@type":"Organization","name":"Vairix"}}
</script></head><body></body></html>`

// a listing/search page with no JobPosting block.
const genericListingHTML = `<html><head>
<script type="application/ld+json">{"@type":"WebSite","name":"Careers"}</script>
</head><body></body></html>`

func TestGenericMatch(t *testing.T) {
	g := NewGeneric(nil)
	for _, raw := range []string{"https://careers.vairix.com/jobs/605143-x", "http://tekton-labs.breezy.hr/p/abc"} {
		u, _ := url.Parse(raw)
		if !g.Match(u) {
			t.Errorf("Match(%s) = false, want true", raw)
		}
	}
	for _, raw := range []string{"ftp://x/y", "mailto:a@b.c", "/relative/path"} {
		u, _ := url.Parse(raw)
		if g.Match(u) {
			t.Errorf("Match(%s) = true, want false", raw)
		}
	}
}

func TestGenericResolvesJobPosting(t *testing.T) {
	const link = "https://careers.vairix.com/jobs/605143-senior-backend?utm_source=x#apply"
	c := (&fakeClient{}).route("/jobs/605143", genericJobHTML, "")

	job, ok, err := NewGeneric(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	// The canonical id/URL drops the query and fragment so a tracking-tagged copy dedups.
	const want = "https://careers.vairix.com/jobs/605143-senior-backend"
	if job.ExternalID != want || job.URL != want {
		t.Errorf("id/url = %q / %q, want %q", job.ExternalID, job.URL, want)
	}
	if !strings.Contains(job.Title, "Senior Backend Engineer") {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "Vairix" {
		t.Errorf("Company = %q, want Vairix", job.Company)
	}
	if !job.Remote {
		t.Error("Remote = false, want true (TELECOMMUTE)")
	}
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "Build &amp; scale.") {
		t.Errorf("Description not sanitized/decoded: %q", job.Description)
	}
}

func TestGenericSkipsNonVacancy(t *testing.T) {
	c := (&fakeClient{}).route("/jobs", genericListingHTML, "")
	_, ok, err := NewGeneric(c).Resolve(context.Background(), "https://acme.com/jobs")
	if err != nil {
		t.Fatalf("Resolve: unexpected error %v", err)
	}
	if ok {
		t.Error("ok=true, want false for a page with no JobPosting block")
	}
}
