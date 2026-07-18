package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// startupandvcDetailHTML is a startupandvc.com vacancy detail page: the schema.org JobPosting
// ld+json (whose description is only a one-line stub), the rich w-richtext body carrying the real
// description (with a <script> sanitizeHTML must strip), and the external "Apply now" button whose
// href is the destination with startupandvc's tracking params appended. datePosted is the site's
// "Jul 07, 2026" format.
func startupandvcDetailHTML(title, company, locality, country, applyHref string) string {
	return `<html><head></head><body>
<h1>` + title + `</h1>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
"title":"` + title + `",
"description":"<p>` + company + ` is looking for a ` + title + `.</p>",
"datePosted":"Jul 07, 2026",
"hiringOrganization":{"@type":"Organization","name":"` + company + `"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}}
</script>
<div class="w-richtext"><h2>POSITION SUMMARY</h2><p>Real body with detail.</p>
<script>alert(1)</script></div>
<a href="` + applyHref + `" target="_blank" class="button-large fw w-button">Apply now</a>
</body></html>`
}

// startupandvcListingHTML is a listing page linking to each given slug. Like the real markup each
// card renders several anchors to the same detail page (title, company, logo), exercising the
// job-link dedup; a non-vacancy nav anchor and the bare listing self-link must be ignored.
func startupandvcListingHTML(slugs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="w-dyn-list">`)
	for _, slug := range slugs {
		b.WriteString(`<a href="/venture-capital-jobs/` + slug + `">A Role</a>`)
		b.WriteString(`<a href="/venture-capital-jobs/` + slug + `">A Company</a>`)
	}
	b.WriteString(`<a href="/venture-capital-jobs">All jobs</a>`) // listing self-link: ignored
	b.WriteString(`<a href="/about-us">About</a>`)                // nav: ignored
	b.WriteString(`</div></body></html>`)
	return b.String()
}

func TestStartupAndVCProvider(t *testing.T) {
	if got := NewStartupAndVC(nil).Provider(); got != "startupandvc" {
		t.Errorf("Provider() = %q, want %q", got, "startupandvc")
	}
}

func TestStartupAndVCSlug(t *testing.T) {
	cases := map[string]string{
		"https://www.startupandvc.com/venture-capital-jobs/associate-9a2fb":       "associate-9a2fb",
		"/venture-capital-jobs/analyst-39b3f":                                     "analyst-39b3f",
		"https://www.startupandvc.com/venture-capital-jobs/associate-9a2fb?utm=1": "associate-9a2fb",
		"https://www.startupandvc.com/venture-capital-jobs":                       "", // listing root
		"https://www.startupandvc.com/venture-capital-jobs/":                      "", // empty slug
		"https://www.startupandvc.com/venture-capital-jobs/cat/x":                 "", // deeper path
		"https://www.startupandvc.com/about-us":                                   "",
	}
	for loc, want := range cases {
		if got := startupandvcSlug(loc); got != want {
			t.Errorf("startupandvcSlug(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestStartupAndVCFetchListingThenDetailAndMaps(t *testing.T) {
	slug := "associate-9a2fb"
	apply := "https://www.linkedin.com/jobs/view/4417182287/?eBP=xyz&trk=abc?utm_source=startupandvc"
	fake := (&routedHTTP{}).
		// Detail route first: its slug substring never matches the bare listing URL, and the
		// listing URL is a substring of the detail URL, so the more specific route must win.
		route("/venture-capital-jobs/"+slug, startupandvcDetailHTML("Associate", "B Capital Group", "New York", "USA", apply)).
		route("/venture-capital-jobs", startupandvcListingHTML(slug))

	jobs, err := NewStartupAndVC(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Startup & VC", Provider: "startupandvc",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != slug {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, slug)
	}
	if j.Title != "Associate" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "B Capital Group" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "New York, USA" {
		t.Errorf("Location = %q, want %q", j.Location, "New York, USA")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Real body with detail") {
		t.Errorf("Description should come from the w-richtext body, got: %q", j.Description)
	}
	if strings.Contains(j.Description, "is looking for a") {
		t.Errorf("Description should prefer the rich body over the ld+json stub, got: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-07", j.PostedAt)
	}
}

// TestStartupAndVCApplyURL covers the outbound-link decision: the job URL points at the real
// posting the "Apply now" button links to, and falls back to the stable landing page when the
// detail page has no external button.
func TestStartupAndVCApplyURL(t *testing.T) {
	slug := "associate-9a2fb"
	detail := "https://www.startupandvc.com/venture-capital-jobs/" + slug

	// With an external apply button, URL is the destination (tracking suffix stripped).
	apply := "https://www.linkedin.com/jobs/view/4417182287/?eBP=xyz&trk=abc?utm_source=startupandvc"
	fake := (&routedHTTP{}).
		route("/venture-capital-jobs/"+slug, startupandvcDetailHTML("Associate", "Acme", "New York", "USA", apply)).
		route("/venture-capital-jobs", startupandvcListingHTML(slug))
	jobs, err := NewStartupAndVC(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if want := "https://www.linkedin.com/jobs/view/4417182287/"; jobs[0].URL != want {
		t.Errorf("URL = %q, want the cleaned external apply target %q", jobs[0].URL, want)
	}

	// Without an apply button, URL falls back to the startupandvc landing page.
	noButton := `<html><body>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting","title":"Associate",
"description":"stub","datePosted":"Jul 07, 2026",
"hiringOrganization":{"@type":"Organization","name":"Acme"}}
</script></body></html>`
	fake2 := (&routedHTTP{}).
		route("/venture-capital-jobs/"+slug, noButton).
		route("/venture-capital-jobs", startupandvcListingHTML(slug))
	jobs2, err := NewStartupAndVC(fake2).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch (no button): %v", err)
	}
	if len(jobs2) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs2))
	}
	if jobs2[0].URL != detail {
		t.Errorf("URL = %q, want the landing-page fallback %q", jobs2[0].URL, detail)
	}
}

func TestStartupAndVCDropsDetailWithoutJobPosting(t *testing.T) {
	good := "associate-9a2fb"
	bad := "analyst-39b3f"
	fake := (&routedHTTP{}).
		route("/venture-capital-jobs/"+good, startupandvcDetailHTML("Associate", "Acme", "New York", "USA", "https://x/apply")).
		route("/venture-capital-jobs/"+bad, `<html><body>no ld+json here</body></html>`).
		route("/venture-capital-jobs", startupandvcListingHTML(good, bad))

	jobs, err := NewStartupAndVC(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != good {
		t.Fatalf("got %v, want only the posting with a JobPosting block", jobs)
	}
}

func TestStartupAndVCRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["startupandvc"]
	if !ok {
		t.Fatal("All() missing provider startupandvc")
	}
	if s.Provider() != "startupandvc" {
		t.Errorf("All()[startupandvc].Provider() = %q", s.Provider())
	}
	// Aggregator (boardless but many companies) — it stays in the source facet.
	if !slices.Contains(FilterableProviders(), "startupandvc") {
		t.Error("FilterableProviders() should include startupandvc (aggregator)")
	}
}
