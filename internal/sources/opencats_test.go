package sources

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// opencatsStockListingHTML is the listing an install running the shipped OpenCATS template
// serves: an XHTML table whose rows link each posting by the careers route, with the title as
// the anchor text. Columns are department, title, location.
func opencatsStockListingHTML(rows ...[3]string) string { // row = {id, title, location}
	var b strings.Builder
	b.WriteString(`<html><body><table class="careerPortalListing"><tr>` +
		`<th>Department</th><th>Job Title</th><th>Location</th></tr>`)
	for _, r := range rows {
		b.WriteString(`<tr class="careerPortalListItem">` +
			`<td>Engineering</td>` +
			`<td><a href="index.php?m=careers&amp;p=showJob&amp;ID=` + r[0] + `">` + r[1] + `</a></td>` +
			`<td>` + r[2] + `</td></tr>`)
	}
	b.WriteString(`</table></body></html>`)
	return b.String()
}

// opencatsRewrittenListingHTML is the same listing from an install that replaced the template
// wholesale (as G4S has): different element types, different classes, extra columns in a
// different order, and a second link to the same posting for the apply action. Only the
// careers route and the anchor text survive the rewrite — which is exactly what the adapter
// is allowed to depend on.
func opencatsRewrittenListingHTML(rows ...[3]string) string { // row = {id, title, location}
	var b strings.Builder
	b.WriteString(`<html><body><div class="jobs-grid">`)
	for _, r := range rows {
		b.WriteString(`<div class="job-card" data-kind="posting">` +
			`<span class="loc">` + r[2] + `</span>` +
			`<span class="entity">Acme Holdings Ltd.</span>` +
			`<h3><a class="ttl" href="/index.php?m=careers&amp;p=showJob&amp;ID=` + r[0] + `">` + r[1] + `</a></h3>` +
			`<a class="btn" href="/index.php?m=careers&amp;p=showJob&amp;ID=` + r[0] + `">APPLY NOW</a>` +
			`</div>`)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// opencatsStockDetailHTML is a posting page from the shipped template: the title follows a
// "Position Details:" prefix in the h1, the labelled fields sit in #detailsTable, and the body
// is #descriptive. The body carries a character entity (est&aacute;) because real portals
// serve accented prose that way — the tags themselves are live, so the shared sanitiser must
// leave readable text, not a re-encoded entity. The embedded script must not survive.
func opencatsStockDetailHTML() string {
	return `<html><body><div id="careerContent">
<h1>Position Details: MuleSoft Platform Support Engineer (L2)</h1>
<table id="detailsTable">
<tr><td class="detailsHeader"><strong>Location:</strong></td><td>Lisboa, Lisboa</td></tr>
<tr><td class="detailsHeader"><strong>Openings:</strong></td><td>1</td></tr>
<tr><td class="detailsHeader"><strong>Work Model:</strong></td><td>Hybrid</td></tr>
</table>
<div id="descriptive"><p><strong>Description:</strong></p>
<p>A equipa est&aacute; a crescer.</p><ul><li>Suporte L2.</li></ul><script>alert(1)</script></div>
</div></body></html>`
}

// opencatsLocalisedDetailHTML is a posting page from an install serving a non-English portal
// (opencats.gorgany.com serves Ukrainian): the shipped template's row order survives, but the
// field labels are translated, so an English label match finds nothing.
func opencatsLocalisedDetailHTML() string {
	return `<html><body><div id="careerContent">
<h1>Position Details: Адміністратор магазину</h1>
<table id="detailsTable">
<tr><td class="detailsHeader"><strong>Місцезнаходження:</strong></td><td>Київ, Україна</td></tr>
<tr><td class="detailsHeader"><strong>Openings:</strong></td><td>2</td></tr>
<tr><td class="detailsHeader"><strong>Зарплата:</strong></td><td></td></tr>
</table>
<div id="descriptive"><p>Робота в магазині.</p></div>
</div></body></html>`
}

// opencatsRenamedBodyDetailHTML is a posting page from an install that renamed the body
// container — careers.crewlogix.com ships it as "job-decription", typo and all — and dropped
// the table id. The labels survive, so only the body lookup has to cope.
func opencatsRenamedBodyDetailHTML() string {
	return `<html><body><div id="careerContent">
<h1>Position Details: Sr. PHP Developer</h1>
<table>
<tr><td class="detailsHeader"><strong>Location:</strong></td><td>Gulberg 3, Lahore</td></tr>
<tr><td class="detailsHeader"><strong>Openings:</strong></td><td>1</td></tr>
</table>
<div class="job-decription"><p>Build and maintain PHP services.</p></div>
</div></body></html>`
}

// TestOpencatsDetailReadsLocalisedLocation covers the 33 postings on the Ukrainian install that
// a label-only lookup silently dropped: the label is translated, but the shipped template's row
// order is not, so the first row of the details table is still the location.
func TestOpencatsDetailReadsLocalisedLocation(t *testing.T) {
	fake := (&routedHTTP{}).
		route("p=showJob&ID=406", opencatsLocalisedDetailHTML()).
		route("p=showAll", opencatsStockListingHTML([3]string{"406", "Адміністратор магазину", "Київ"}))

	jobs, err := NewOpencats(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Gorgany", Board: "opencats.gorgany.com/careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Location != "Київ, Україна" {
		t.Errorf("Location = %q, want the first details row when the label is not English", jobs[0].Location)
	}
}

// TestOpencatsDetailReadsRenamedBody covers the 8 postings that came back with an empty
// description because the install renamed the body container.
func TestOpencatsDetailReadsRenamedBody(t *testing.T) {
	fake := (&routedHTTP{}).
		route("p=showJob&ID=114", opencatsRenamedBodyDetailHTML()).
		route("p=showAll", opencatsStockListingHTML([3]string{"114", "Sr. PHP Developer", "Lahore"}))

	jobs, err := NewOpencats(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Crewlogix Technologies", Board: "careers.crewlogix.com/careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "Build and maintain PHP services") {
		t.Errorf("Description = %q, want the renamed body container's text", jobs[0].Description)
	}
	if jobs[0].Location != "Gulberg 3, Lahore" {
		t.Errorf("Location = %q", jobs[0].Location)
	}
}

func TestOpencatsFetchMapsListingAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("p=showJob&ID=51", opencatsStockDetailHTML()).
		route("p=showAll", opencatsStockListingHTML(
			[3]string{"51", "MuleSoft Platform Support Engineer (L2)", "Lisbon"}))

	jobs, err := NewOpencats(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Boomit", Provider: "opencats", Board: "careers.boomit.pt/careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "51" {
		t.Errorf("ExternalID = %q, want the native posting id 51", j.ExternalID)
	}
	if j.Title != "MuleSoft Platform Support Engineer (L2)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Boomit" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if want := "https://careers.boomit.pt/careers/index.php?m=careers&p=showJob&ID=51"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Lisboa, Lisboa" {
		t.Errorf("Location = %q, want the detail page's Location field", j.Location)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Suporte L2") {
		t.Errorf("Description lost the body: %q", j.Description)
	}
	if !strings.Contains(j.Description, "está") {
		t.Errorf("Description should carry readable accented text, got %q", j.Description)
	}
}

// TestOpencatsFetchResolvesRootMountedBoard covers the other mount shape: a portal served from
// the web root rather than under a /careers prefix.
func TestOpencatsFetchResolvesRootMountedBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		route("p=showJob&ID=30156", opencatsStockDetailHTML()).
		route("https://atscareers.g4s.com/index.php?m=careers&p=showAll",
			opencatsStockListingHTML([3]string{"30156", "Security Analyst", "Chennai"}))

	jobs, err := NewOpencats(fake).Fetch(context.Background(), CompanyEntry{
		Company: "G4S", Provider: "opencats", Board: "atscareers.g4s.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 — the listing URL was probably built wrong", len(jobs))
	}
}

// TestOpencatsListingsSkipGeneralApplication guards a real posting seen on careers.boomit.pt:
// installs park a "Can't find what you're looking for? Apply here" entry in the portal, which
// uses the posting route but is a talent-pool form, not an open position.
func TestOpencatsListingsSkipGeneralApplication(t *testing.T) {
	base, err := url.Parse("https://careers.boomit.pt/careers/")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	for _, title := range []string{
		"Can’t find what you’re looking for? Apply here",
		"Can't find what you're looking for? Apply here",
		"General Application",
		"Open Application",
		"Spontaneous application",
	} {
		got := opencatsListings(base, parseHTML(t, opencatsStockListingHTML([3]string{"24", title, "Various"})))
		if len(got) != 0 {
			t.Errorf("title %q: got %d postings, want it excluded as a general-application entry", title, len(got))
		}
	}
}

// TestOpencatsListingsKeepRealPostingsThatMentionApplications is the other half of the filter:
// the exclusion must not swallow genuine roles whose titles happen to contain the same words.
func TestOpencatsListingsKeepRealPostingsThatMentionApplications(t *testing.T) {
	base, err := url.Parse("https://careers.boomit.pt/careers/")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	for _, title := range []string{
		"Senior Application Security Engineer",
		"Application Support Analyst",
		"General Manager, Engineering",
	} {
		got := opencatsListings(base, parseHTML(t, opencatsStockListingHTML([3]string{"77", title, "Lisbon"})))
		if len(got) != 1 {
			t.Errorf("title %q: got %d postings, want it kept as a real posting", title, len(got))
		}
	}
}

func TestOpencatsListingErrorIsBoardError(t *testing.T) {
	if _, err := NewOpencats(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Board: "careers.boomit.pt/careers",
	}); err == nil {
		t.Fatal("want a board-level error when the listing fails, not an empty success")
	}
}

// TestOpencatsFailedDetailSkipsOnlyThatPosting keeps one broken posting from costing the board:
// self-hosted installs are individually flaky, so isolation is the difference between losing a
// job and losing an employer.
func TestOpencatsFailedDetailSkipsOnlyThatPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("p=showJob&ID=51", opencatsStockDetailHTML()). // ID=50 has no route: its detail fails
		route("p=showAll", opencatsStockListingHTML(
			[3]string{"51", "MuleSoft Platform Support Engineer (L2)", "Lisbon"},
			[3]string{"50", "Data Engineer (Fabric + Power BI)", "Porto"}))

	jobs, err := NewOpencats(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Boomit", Board: "careers.boomit.pt/careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "51" {
		t.Fatalf("got %+v, want only posting 51 — a failed detail must not drop the board", jobs)
	}
}

func TestOpencatsRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["opencats"]
	if !ok {
		t.Fatal("All() missing provider opencats")
	}
	if s.Provider() != "opencats" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if slices.Contains(SelfClosingProviders(All(nil)), "opencats") {
		t.Error("opencats must not be self-closing: the portal gives no removal signal")
	}
}

// TestOpencatsListingsReadRoutingNotMarkup is the adapter's central contract: installs
// customise the portal template freely, so two listings with nothing in common but the
// careers route must yield the same postings.
func TestOpencatsListingsReadRoutingNotMarkup(t *testing.T) {
	rows := [][3]string{
		{"51", "MuleSoft Platform Support Engineer (L2)", "Lisbon"},
		{"50", "Data Engineer (Fabric + Power BI)", "Remote"},
	}
	base, err := url.Parse("https://careers.boomit.pt/careers/")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	for name, doc := range map[string]string{
		"stock template":     opencatsStockListingHTML(rows...),
		"rewritten template": opencatsRewrittenListingHTML(rows...),
	} {
		t.Run(name, func(t *testing.T) {
			got := opencatsListings(base, parseHTML(t, doc))
			if len(got) != 2 {
				t.Fatalf("got %d postings, want 2: %+v", len(got), got)
			}
			for i, want := range rows {
				if got[i].ID != want[0] {
					t.Errorf("posting %d: ID = %q, want %q", i, got[i].ID, want[0])
				}
				if got[i].Title != want[1] {
					t.Errorf("posting %d: Title = %q, want the anchor text %q", i, got[i].Title, want[1])
				}
			}
		})
	}
}

// TestOpencatsListingsCollapseRepeatedLinks guards the rewritten-template case where a
// posting is linked twice (title plus an apply button): the portal offers one position, so
// the adapter must offer one job.
func TestOpencatsListingsCollapseRepeatedLinks(t *testing.T) {
	base, err := url.Parse("https://atscareers.g4s.com/")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	doc := opencatsRewrittenListingHTML([3]string{"30156", "Security Analyst", "Chennai, India"})

	got := opencatsListings(base, parseHTML(t, doc))
	if len(got) != 1 {
		t.Fatalf("got %d postings, want 1 (the title link and the apply link are one posting): %+v", len(got), got)
	}
	if got[0].Title != "Security Analyst" {
		t.Errorf("Title = %q, want the title anchor's text, not the apply button's", got[0].Title)
	}
	if want := "https://atscareers.g4s.com/index.php?m=careers&p=showJob&ID=30156"; got[0].URL != want {
		t.Errorf("URL = %q, want %q", got[0].URL, want)
	}
}

// opencatsOnclickListingHTML is a listing from an install that makes the whole table row
// clickable instead of linking the title — rms.adgonline.ca (Vancouver Police Department)
// ships this. The careers route is intact, but it rides an onclick handler, so there is no
// anchor and therefore no anchor text: the title is the row's first cell.
func opencatsOnclickListingHTML(rows ...[3]string) string { // row = {id, title, location}
	var b strings.Builder
	b.WriteString(`<html><body><table><tr><th>Job Title</th><th>Location</th></tr>`)
	for _, r := range rows {
		b.WriteString(`<tr class="oddTableRow" style="cursor: pointer;" ` +
			`onclick="window.location.href='index.php?m=careers&amp;p=showJob&amp;ID=` + r[0] + `';">` +
			`<td>` + r[1] + `</td><td>` + r[2] + `</td></tr>`)
	}
	b.WriteString(`</table></body></html>`)
	return b.String()
}

// TestOpencatsListingsReadClickableRows covers a portal that carries the posting route on a
// row handler rather than an anchor. The routing invariant still holds, so the postings must
// still be found — an anchor is one carrier of the route, not the definition of a posting.
func TestOpencatsListingsReadClickableRows(t *testing.T) {
	base, err := url.Parse("https://rms.adgonline.ca/careers/")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	rows := [][3]string{
		{"11", "Home Guard Recruitment 2024", "Vancouver, Vancouver"},
		{"12", "Police Constable", "Vancouver, Vancouver"},
	}

	got := opencatsListings(base, parseHTML(t, opencatsOnclickListingHTML(rows...)))
	if len(got) != 2 {
		t.Fatalf("got %d postings, want 2: %+v", len(got), got)
	}
	for i, want := range rows {
		if got[i].ID != want[0] {
			t.Errorf("posting %d: ID = %q, want %q", i, got[i].ID, want[0])
		}
		if got[i].Title != want[1] {
			t.Errorf("posting %d: Title = %q, want the row's first cell %q", i, got[i].Title, want[1])
		}
		wantURL := "https://rms.adgonline.ca/careers/index.php?m=careers&p=showJob&ID=" + want[0]
		if got[i].URL != wantURL {
			t.Errorf("posting %d: URL = %q, want %q", i, got[i].URL, wantURL)
		}
	}
}
