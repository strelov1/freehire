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

// entityEncodedJobHTML mirrors a Teamtailor detail page whose ld+json description is
// HTML-entity-encoded (`&lt;p&gt;…`) — including an entity-encoded <script>. The resolver
// must decode it before sanitizing, so the stored markup is real HTML (not literal tags
// that render broken under {@html}) and the decoded script is still stripped.
const entityEncodedJobHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Senior Backend Engineer","hiringOrganization":{"name":"Vairix"},
 "description":"&lt;p&gt;We build &lt;strong&gt;distributed systems&lt;/strong&gt;.&lt;/p&gt;&lt;script&gt;evil()&lt;/script&gt;"}
</script></head><body></body></html>`

func TestGenericDecodesEntityEncodedDescription(t *testing.T) {
	c := (&fakeClient{}).route("/jobs/1", entityEncodedJobHTML, "")
	job, ok, err := NewGeneric(c).Resolve(context.Background(), "https://careers.vairix.com/jobs/1-x")
	if err != nil || !ok {
		t.Fatalf("Resolve: ok=%v err=%v", ok, err)
	}
	if strings.Contains(job.Description, "&lt;") {
		t.Errorf("description kept literal entities (not decoded): %q", job.Description)
	}
	if !strings.Contains(job.Description, "<strong>distributed systems</strong>") {
		t.Errorf("description lost its real markup after decode+sanitize: %q", job.Description)
	}
	if strings.Contains(job.Description, "evil") {
		t.Errorf("decoded <script> was not stripped: %q", job.Description)
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
