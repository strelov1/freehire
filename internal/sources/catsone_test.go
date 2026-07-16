package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// catsoneListingHTML is a CATS portal page: a server-rendered table whose rows are
// <a class="table-row"> anchors carrying the job's title (.title-cell) and location
// (data-label="Location") in cells, linking to /careers/<portalId>/jobs/<id> detail pages.
func catsoneListingHTML(rows ...[3]string) string { // row = {idSlug, title, location}
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs-table"><div class="table-header">` +
		`<div class="header-cell">Job Title</div></div>`)
	for _, r := range rows {
		b.WriteString(`<a class="table-row" href="/careers/90438/jobs/` + r[0] + `">` +
			`<div class="data-cell title-cell">` + r[1] + `</div>` +
			`<div class="data-cell" data-label="Category">General</div>` +
			`<div class="data-cell" data-label="Location">` + r[2] + `</div></a>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// catsoneDetailHTML is a CATS job detail page: the description body is the .job-description
// element inside .job-description-container (which also carries a "View all jobs" link and a
// repeated header). The description embeds a <script> that sanitizeHTML must strip.
func catsoneDetailHTML() string {
	return `<html><body><div class="job-description-container">
<a class="view-all-jobs" href="/careers/90438">View all jobs</a>
<div><div class="job-header"><h2>Ignored Header</h2><div class="job-tags"><div>Brazil, Remote</div></div></div>
<hr/><div class="job-description"><p><strong>About</strong></p><p>Sell things.</p><script>alert(1)</script></div></div>
</div></body></html>`
}

func TestCatsoneProvider(t *testing.T) {
	if got := NewCatsone(nil).Provider(); got != "catsone" {
		t.Errorf("Provider() = %q, want catsone", got)
	}
}

func TestCatsoneJobID(t *testing.T) {
	cases := map[string]string{
		"https://acme.catsone.com/careers/90438/jobs/16831828-sdr": "16831828",
		"/careers/2531/jobs/8124207":                               "8124207",
		"https://acme.catsone.com/careers/90438":                   "",
		"/careers/90438/jobs/":                                     "",
	}
	for loc, want := range cases {
		if got := catsoneJobID(loc); got != want {
			t.Errorf("catsoneJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestCatsoneFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/16831828", catsoneDetailHTML()). // detail routes first (Contains-matching)
		route("/jobs/16823910", catsoneDetailHTML()).
		route("/careers", catsoneListingHTML(
			[3]string{"16831828-sdr", "Sales Development Representative (SDR)", "Brazil, Remote"},
			[3]string{"16823910-ai", "AI Engineer", "Kyiv"}))

	jobs, err := NewCatsone(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Sphere", Provider: "catsone", Board: "sphereinc.catsone.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["16831828"]
	if !ok {
		t.Fatalf("missing job 16831828 in %v", byID)
	}
	if j.URL != "https://sphereinc.catsone.com/careers/90438/jobs/16831828-sdr" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Sales Development Representative (SDR)" {
		t.Errorf("Title = %q, want listing title-cell", j.Title)
	}
	if j.Company != "Sphere" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Brazil, Remote" {
		t.Errorf("Location = %q, want listing Location cell", j.Location)
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true for %q", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Sell things") {
		t.Errorf("Description lost body: %q", j.Description)
	}
	if strings.Contains(j.Description, "View all jobs") || strings.Contains(j.Description, "Ignored Header") {
		t.Errorf("Description leaked chrome (header/view-all): %q", j.Description)
	}
}

func TestCatsoneListingErrorIsBoardError(t *testing.T) {
	if _, err := NewCatsone(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "acme.catsone.com"}); err == nil {
		t.Fatal("want a board-level error when the listing fails")
	}
}

func TestCatsoneRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["catsone"]
	if !ok {
		t.Fatal("All() missing provider catsone")
	}
	if s.Provider() != "catsone" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "catsone") {
		t.Error("FilterableProviders() should include catsone")
	}
}
