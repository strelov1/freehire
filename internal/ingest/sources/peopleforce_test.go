package sources

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
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

// peopleforceDetailHTMLTailwind mirrors a tenant already migrated to PeopleForce's newer
// theme, which renders the same layout with Tailwind-prefixed classes ("tw-col-lg-8"
// instead of "col-lg-8") — seen live on boards mid-rollout (e.g. vyriy, unitedsoftware).
func peopleforceDetailHTMLTailwind(location string) string {
	return `<html><body>
<h1>Work at Acme</h1>
<div class="tw-row">
  <div class="tw-col-lg-8 tw-col-12">
    <h2>About the role</h2><p>Build things.</p>
  </div>
  <div class="tw-col-lg-4 tw-col-12"><dl><dt>Location</dt><dd>` + location + `</dd></dl></div>
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

func TestPeopleForceFetchReadsDescriptionFromTailwindThemedDetailPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML([2]string{"229080-marketing-ops", "Marketing Operations Manager"})).
		route("?page=2", emptyPeopleforceListingHTML).
		route("/careers/v/229080-marketing-ops", peopleforceDetailHTMLTailwind("Kyiv"))

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
	if j.Location != "Kyiv" {
		t.Errorf("Location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "About the role") || !strings.Contains(j.Description, "Build things") {
		t.Errorf("Description empty or lost content on the Tailwind-themed layout: %q", j.Description)
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

// PeopleForce's detail page is its ONLY source for a posting and is re-fetched on every run (no
// HydratingSource), so a dropped one leaves a live vacancy missing from a crawl that reported no
// failure — which the sweep would read as the posting having gone. Now that peopleforce carries
// the fullBoardListing marker, that silent drop is exactly what the marker's promise forbids: a
// failed-but-not-gone detail request must yield an unreadableDetail marker instead.
func TestPeopleForceUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML(
			[2]string{"100-brand-leader", "Brand Leader"},
			[2]string{"200-ops-manager", "Ops Manager"})).
		route("?page=2", emptyPeopleforceListingHTML).
		routeErr("/careers/v/200-ops-manager", errors.New("connection reset by peer")).
		route("/careers/v/100-brand-leader", peopleforceDetailHTML("Full-time", "Remote"))

	jobs, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "peopleforce", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one unreadable detail: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != "100" {
		t.Fatalf("read = %v, want only the posting whose detail answered", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "200" {
		t.Fatalf("unreadable markers = %v, want one for the posting whose detail did not", markers)
	}
	if markers[0].Company != "Acme" {
		t.Errorf("marker Company = %q, want the ENTRY's employer", markers[0].Company)
	}
}

// The other half of the distinction: 404 is the platform's own answer that the posting is gone,
// so the crawl drops it rather than marking it unreadable.
func TestPeopleForceGoneDetailDropsThePosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML(
			[2]string{"100-brand-leader", "Brand Leader"},
			[2]string{"200-ops-manager", "Ops Manager"})).
		route("?page=2", emptyPeopleforceListingHTML).
		routeErr("/careers/v/200-ops-manager", &StatusError{Method: "GET", Code: 404, URL: "https://acme.peopleforce.io/careers/v/200-ops-manager"}).
		route("/careers/v/100-brand-leader", peopleforceDetailHTML("Full-time", "Remote"))

	jobs, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "peopleforce", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "100" {
		t.Fatalf("got %v, want only the posting whose detail answered — the 404'd one dropped, not marked", jobs)
	}
}

// A page whose every card is already-listed (e.g. a re-served page) must not end the walk early:
// only a genuinely empty page proves the board's end. If the walk stopped on "no NEWLY-KEPT
// cards" rather than "the raw page has no cards", it would end at page 2 and never reach page 3.
func TestPeopleForceFetchReachesAPostingPastADuplicateOnlyPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", peopleforceListingHTML([2]string{"100-brand-leader", "Brand Leader"})).
		route("?page=2", peopleforceListingHTML([2]string{"100-brand-leader", "Brand Leader"})). // duplicate
		route("?page=3", peopleforceListingHTML([2]string{"200-ops-manager", "Ops Manager"})).   // new
		route("?page=4", emptyPeopleforceListingHTML).
		route("/careers/v/100-brand-leader", peopleforceDetailHTML("Full-time", "Remote")).
		route("/careers/v/200-ops-manager", peopleforceDetailHTML("Full-time", "Remote"))

	jobs, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "peopleforce", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	if !ids["100"] || !ids["200"] {
		t.Errorf("got job ids %v, want both 100 and 200 (the walk must not stop at the duplicate-only page 2)", ids)
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

func TestPeopleForceRegisteredAsFullBoardListing(t *testing.T) {
	if _, ok := NewPeopleForce(nil).(fullBoardListing); !ok {
		t.Error("peopleforce should implement the fullBoardListing marker")
	}
	if !FullBoardListingProviders(All(nil))["peopleforce"] {
		t.Error("FullBoardListingProviders(All(nil)) should include peopleforce")
	}
}

// peopleforceEndlessFake serves a fresh job card on every page, so it never yields a genuinely
// empty page — used to prove the page-cap ceiling fails loudly rather than succeeding partially.
// Mirrors taleoEndlessFake / gustoEndlessFake.
type peopleforceEndlessFake struct{ calls int }

func (f *peopleforceEndlessFake) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	f.calls++
	id := fmt.Sprintf("%d-endless-role", f.calls)
	return html.Parse(strings.NewReader(peopleforceListingHTML([2]string{id, "Role"})))
}

func TestPeopleForceFetchFailsWhenListingExceedsThePageCap(t *testing.T) {
	fake := &peopleforceEndlessFake{}
	_, err := NewPeopleForce(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err == nil {
		t.Fatal("expected reaching the page cap to fail the Fetch")
	}
	if fake.calls != peopleforceMaxPages {
		t.Errorf("got %d listing calls, want exactly %d (the cap, no more)", fake.calls, peopleforceMaxPages)
	}
}
