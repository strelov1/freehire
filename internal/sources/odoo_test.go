package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// odooListingHTML is an Odoo /jobs listing: server-rendered cards linking to
// /jobs/detail/<slug>-<id> pages, plus a non-posting nav link that the id predicate rejects.
func odooListingHTML(links ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="o_job_index"><a href="/jobs">All positions</a>`)
	for _, l := range links {
		b.WriteString(`<a href="/jobs/detail/` + l + `">A Role</a>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// odooDetailHTML is an Odoo job detail page: a schema.org JobPosting as microdata. The title
// is the page <h1>; the description is itemprop="description"; datePosted is a content attr.
func odooDetailHTML(title string) string {
	return `<html><body itemscope itemtype="https://schema.org/JobPosting">
<h1 itemprop="title">` + title + `</h1>
<span itemprop="datePosted" content="2026-07-10">Jul 10</span>
<span itemprop="addressLocality">Kyiv</span><span itemprop="addressCountry">Ukraine</span>
<div itemprop="description"><p>Build things.</p><script>alert(1)</script></div>
</body></html>`
}

func TestOdooProvider(t *testing.T) {
	if got := NewOdoo(nil).Provider(); got != "odoo" {
		t.Errorf("Provider() = %q, want odoo", got)
	}
}

func TestOdooJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.acme.com/jobs/detail/recruiter-157":         "157",
		"/en_US/jobs/detail/full-stack-python-developer-1":        "1",
		"https://acme.com/jobs/analista-de-facilities-2621":       "2621",
		"https://acme.com/jobs/detail/senior-ios-developer-2?x=1": "2",
		"https://acme.com/jobs":                                   "",
	}
	for loc, want := range cases {
		if got := odooJobID(loc); got != want {
			t.Errorf("odooJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestOdooFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/detail/recruiter-157", odooDetailHTML("Recruiter")).
		route("/jobs", odooListingHTML("recruiter-157"))

	jobs, err := NewOdoo(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Ameware", Provider: "odoo", Board: "jobs.amewaregroup.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "157" {
		t.Errorf("ExternalID = %q, want 157", j.ExternalID)
	}
	if j.URL != "https://jobs.amewaregroup.com/jobs/detail/recruiter-157" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Recruiter" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Ameware" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Kyiv, Ukraine" {
		t.Errorf("Location = %q, want %q", j.Location, "Kyiv, Ukraine")
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-10", j.PostedAt)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build things") {
		t.Errorf("Description lost body: %q", j.Description)
	}
}

func TestOdooDropsDetailWithoutJobPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/detail/good-1", odooDetailHTML("Good")).
		route("/jobs/detail/bare-2", `<html><body>no microdata</body></html>`).
		route("/jobs", odooListingHTML("good-1", "bare-2"))

	jobs, err := NewOdoo(fake).Fetch(context.Background(), CompanyEntry{Board: "acme.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1" {
		t.Fatalf("got %v, want only the posting with a JobPosting description", jobs)
	}
}

func TestOdooListingErrorIsBoardError(t *testing.T) {
	if _, err := NewOdoo(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "acme.com"}); err == nil {
		t.Fatal("want a board-level error when the listing fails")
	}
}

func TestOdooRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["odoo"]
	if !ok {
		t.Fatal("All() missing provider odoo")
	}
	if s.Provider() != "odoo" {
		t.Errorf("Provider() = %q", s.Provider())
	}
}
