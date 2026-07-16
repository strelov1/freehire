package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// peopleforceListingHTML is a PeopleForce careers listing page: server-rendered job cards, each
// an <h4><a href="/careers/v/<id>-<slug>">Title</a></h4>, plus a ?page=2 pagination anchor. The
// title is read from the anchor text (the detail page's <h1> is the generic "Work at <Company>").
func peopleforceListingHTML(cards ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="row">`)
	for _, c := range cards { // c = {id-slug, title}
		b.WriteString(`<div class="col-12"><h4><a class="stretched-link" data-turbo-frame="_top" ` +
			`href="/careers/v/` + c[0] + `">` + c[1] + `</a></h4></div>`)
	}
	b.WriteString(`<nav><a href="?page=2">Next</a></nav></div></body></html>`)
	return b.String()
}

// emptyPeopleforceListingHTML is a listing past the last page: no job cards, so the pagination
// walk stops when it yields no new links.
const emptyPeopleforceListingHTML = `<html><body><div class="row"></div></body></html>`

// peopleforceDetailHTML is a PeopleForce job detail page: the description lives in the Bootstrap
// col-lg-8 column, and a <dl> sidebar carries Work type / Department / Location. The description
// embeds a <script> that sanitizeHTML must strip.
func peopleforceDetailHTML(workType, location string) string {
	wt := ""
	if workType != "" {
		wt = `<dt>Work type</dt><dd>` + workType + `</dd>`
	}
	return `<html><head><meta property="og:title" content="Acme - A Role"></head><body>
<h1>Work at Acme</h1>
<div class="row">
  <div class="col-lg-8 col-12">
    <h2>About the role</h2><p>Build things.</p><script>alert(1)</script>
    <ul><li>Ship</li></ul>
  </div>
  <div class="col-lg-4 col-12"><dl>` + wt + `<dt>Department</dt><dd>Sales</dd>
    <dt>Location</dt><dd>` + location + `</dd></dl></div>
</div></body></html>`
}

func TestPeopleForceProvider(t *testing.T) {
	if got := NewPeopleForce(nil).Provider(); got != "peopleforce" {
		t.Errorf("Provider() = %q, want %q", got, "peopleforce")
	}
}

func TestPeopleForceJobID(t *testing.T) {
	cases := map[string]string{
		"https://acme.peopleforce.io/careers/v/222906-brand-leader": "222906",
		"/careers/v/145060-brand-leader":                            "145060",
		"https://acme.peopleforce.io/careers/v/145060?x=1":          "145060",
		"https://acme.peopleforce.io/careers":                       "",
		"/careers/v/":                                               "",
	}
	for loc, want := range cases {
		if got := peopleforceJobID(loc); got != want {
			t.Errorf("peopleforceJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestPeopleForceFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML([2]string{"222906-brand-leader", "Brand Leader"})).
		route("?page=2", emptyPeopleforceListingHTML).
		route("/careers/v/222906-brand-leader", peopleforceDetailHTML("Full-time", "Philippines/Metro Manila/Manila"))

	jobs, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "peopleforce", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "222906" {
		t.Errorf("ExternalID = %q, want 222906", j.ExternalID)
	}
	if j.URL != "https://acme.peopleforce.io/careers/v/222906-brand-leader" {
		t.Errorf("URL = %q, want canonical detail URL", j.URL)
	}
	if j.Title != "Brand Leader" {
		t.Errorf("Title = %q, want anchor text", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want configured company", j.Company)
	}
	if j.Location != "Philippines/Metro Manila/Manila" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "About the role") || !strings.Contains(j.Description, "Build things") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
}

func TestPeopleForcePaginatesAcrossPages(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML([2]string{"1-a", "Role 1"}, [2]string{"2-b", "Role 2"})).
		route("?page=2", peopleforceListingHTML([2]string{"3-c", "Role 3"})).
		route("?page=3", emptyPeopleforceListingHTML).
		route("/careers/v/1-a", peopleforceDetailHTML("Part-time", "Kyiv")).
		route("/careers/v/2-b", peopleforceDetailHTML("", "Kyiv")).
		route("/careers/v/3-c", peopleforceDetailHTML("Full-time", "Remote"))

	jobs, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	ids := []string{jobs[0].ExternalID, jobs[1].ExternalID, jobs[2].ExternalID}
	for _, want := range []string{"1", "2", "3"} {
		if !slices.Contains(ids, want) {
			t.Errorf("missing job %q in %v", want, ids)
		}
	}
}

func TestPeopleForceListingErrorIsBoardError(t *testing.T) {
	fake := &routedHTTP{} // no routes → first listing GET fails
	if _, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("want a board-level error when the first listing page fails")
	}
}

func TestPeopleForceRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["peopleforce"]
	if !ok {
		t.Fatal("All() missing provider peopleforce")
	}
	if s.Provider() != "peopleforce" {
		t.Errorf("All()[peopleforce].Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "peopleforce") {
		t.Error("FilterableProviders() should include peopleforce (board-based)")
	}
}
