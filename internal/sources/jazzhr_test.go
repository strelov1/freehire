package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// jazzhrDetailHTML is a JazzHR job page: server-rendered HTML whose only payload we read is
// the schema.org JobPosting ld+json. jobLocation is a single Place (not an array). The
// description embeds a <script> (written as <\/script>) that sanitizeHTML must strip.
const jazzhrDetailHTML = `<html><head></head><body>
<script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting",
"url":"https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer",
"title":"Field Service Engineer",
"description":"<p>Fix machines.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-16",
"hiringOrganization":{"@type":"Organization","name":"ProAutomated"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Dover","addressRegion":"DE"}}}
</script>
</body></html>`

// jazzhrListingHTML is the /apply listing: anchors to /apply/<token>/<slug> job pages,
// plus noise links the adapter must ignore.
const jazzhrListingHTML = `<html><body>
<a href="/about">About</a>
<a href="/apply/nfHu9c2Sxz/Field-Service-Engineer">Field Service Engineer</a>
<a href="/apply/nfHu9c2Sxz/Field-Service-Engineer">Apply</a>
</body></html>`

func TestJazzHRProvider(t *testing.T) {
	if got := NewJazzHR(nil).Provider(); got != "jazzhr" {
		t.Errorf("Provider() = %q, want %q", got, "jazzhr")
	}
}

func TestJazzHRJobID(t *testing.T) {
	cases := map[string]string{
		"https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer": "nfHu9c2Sxz",
		"/apply/bhCE7nHkv6/Some-Role":               "bhCE7nHkv6",
		"https://proautomated.applytojob.com/about": "",
	}
	for u, want := range cases {
		if got := jazzhrJobID(u); got != want {
			t.Errorf("jazzhrJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestJazzHRFetchListingThenDetailAndMaps(t *testing.T) {
	job := "https://proautomated.applytojob.com/apply/nfHu9c2Sxz/Field-Service-Engineer"
	// The listing emits a relative href, so the test fails unless the adapter resolves it
	// against the board host before fetching the detail.
	fake := (&routedHTTP{}).
		route("/apply/nfHu9c2Sxz/Field-Service-Engineer", jazzhrDetailHTML).
		route("/apply", jazzhrListingHTML)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "ProAutomated", Provider: "jazzhr", Board: "proautomated",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the title+apply anchors de-duped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "nfHu9c2Sxz" {
		t.Errorf("ExternalID = %q, want nfHu9c2Sxz", j.ExternalID)
	}
	if j.URL != job {
		t.Errorf("URL = %q, want %q", j.URL, job)
	}
	if j.Title != "Field Service Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "ProAutomated" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Dover, DE" {
		t.Errorf("Location = %q, want %q", j.Location, "Dover, DE")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Fix machines") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-16", j.PostedAt)
	}
}

func TestJazzHRCompanyFallsBackToEntry(t *testing.T) {
	detail := `<html><body><script type="application/ld+json">
{"@type":"JobPosting","title":"Role","datePosted":"2026-06-16",
"hiringOrganization":{"name":""},
"jobLocation":{"address":{"addressLocality":"Austin","addressCountry":"US"}}}
</script></body></html>`
	fake := (&routedHTTP{}).
		route("/apply/abc123XYZ0/role", detail).
		route("/apply", `<html><body><a href="/apply/abc123XYZ0/role">Role</a></body></html>`)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme Corp", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme Corp" {
		t.Fatalf("Company = %q, want fallback %q", jobs[0].Company, "Acme Corp")
	}
}

func TestJazzHRFailedDetailDropsOnlyThatPosting(t *testing.T) {
	listing := `<html><body>
<a href="/apply/keptKept11/kept">kept</a>
<a href="/apply/dropDrop22/dropped">dropped</a>
</body></html>`
	// No route for /apply/dropDrop22/... → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("/apply/keptKept11/kept", jazzhrDetailHTML).
		route("/apply", listing)

	jobs, err := NewJazzHR(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "keptKept11" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}
