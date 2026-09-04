package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The two board uuids the fixtures use. Gusto keys a board by the whole
// "<company-slug>-<company-uuid>" path segment, so the fixture board id carries one too.
const (
	gustoTestBoard   = "acme-robotics-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90"
	gustoTestPosting = "/postings/acme-robotics-senior-go-engineer-dc55dd36-ff48-49d3-9904-9a2c5746286d"
	gustoTestID      = "dc55dd36-ff48-49d3-9904-9a2c5746286d"
)

// gustoListingItemHTML renders one posting row of a board listing exactly as jobs.gusto.com
// does. meta is the row's second paragraph: "<pay> · <employment type>", or the employment type
// alone when the employer states no pay. The pay icon's UNTERMINATED class attribute is
// reproduced verbatim — the board really ships it, and the row's text has to survive it.
func gustoListingItemHTML(href, title, location, meta string) string {
	return `<li>
  <a class="block hover:bg-gray-50" href="` + href + `">
    <div class="px-4 py-4 sm:px-6">
      <h3 class="text-lg">` + title + `</h3>
      <p class="text-gray-500 flex">
        <svg class="h-5 w-5 inline mr-1" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-width="2" d="M17.657 16.657L13.414 20.9z"/>
        </svg>
            ` + location + `
            </br>
      </p>
      <p class="text-gray-500 flex items-center">
          <svg class="h-5 w-5 inline mr-1 width="16" height="12" viewBox="0 0 16 12" fill="none" xmlns="http://www.w3.org/2000/svg">
  <path fill-rule="evenodd" clip-rule="evenodd" d="M0.5 3.31601C0.5 2.58823z" fill="#6C6C72"/>
</svg>

          ` + meta + `
      </p>
    </div>
</a></li>`
}

// gustoBoardHTML renders a board listing page around the given posting rows. With none it
// renders the "no open positions" placeholder the platform serves past the last page — which
// carries an <h3> of its own but no posting anchor, so the walk ends there.
func gustoBoardHTML(items ...string) string {
	body := `<div class="px-4 py-8 sm:p-8 text-center text-gray-500">
      <h3 class="text-lg">There are no open positions currently</h3>
    </div>`
	if len(items) > 0 {
		body = `<ul class="divide-y divide-gray-200">` + strings.Join(items, "\n") + `</ul>`
	}
	return `<html><head><title>Careers at Acme Robotics</title></head><body class="bg-gray-100">
<div id='job-board-header'><h1 class="mt-1 text-4xl">Acme Robotics</h1></div>
<div>` + body + `</div></body></html>`
}

// gustoPostingHTML renders a posting page the way jobs.gusto.com does: a breadcrumb back to the
// board, an <h1> carrying the employer, the title and the "<location> · <employment type>" line,
// then the unheaded summary block and the "About <employer>" / "Description" / "Salary"
// sections. An empty about or salary omits that section, which the platform also does.
func gustoPostingHTML(title, meta, summary, about, description, salary string) string {
	section := func(heading, body string) string {
		if body == "" {
			return ""
		}
		return `<div class="mt-8 text-xl text-gray-600 leading-8">
        <h3 class="mb-4 text-3xl leading-8 font-semibold tracking-tight text-gray-700">` + heading + `</h3>
        <div data-controller="rich-text">
          <div class="rich-text-container" data-rich-text-target="richTextContainer">
            ` + body + `
          </div>
        </div>
      </div>`
	}
	pay := ""
	if salary != "" {
		pay = `<div class="mt-8 text-xl text-gray-600 leading-8">
        <h4 class="mb-4 text-3xl leading-8 font-semibold tracking-tight text-gray-700">Salary</h4>
        <p class="mt-4">` + salary + `</p>
      </div>`
	}
	return `<html><head><title>` + title + `</title></head><body class="bg-gray-100">
<div class="relative py-16" data-controller="referrer">
  <nav class="hidden sm:flex" aria-label="Breadcrumb"><ol><li>
    <a class="text-sm" href="/boards/` + gustoTestBoard + `">Careers at Acme Robotics</a>
  </li></ol></nav>
  <h1>
    <span class="block text-base text-center text-indigo-600 font-semibold">Acme Robotics</span>
    <span class="mt-2 block text-3xl text-center leading-8 font-extrabold">` + title + `</span>
    <span class="mt-2 block text-base text-center text-gray-900">` + meta + `</span>
  </h1>
  <div class="mt-8 text-xl text-gray-800 leading-8">
    <p>` + summary + `</p>
  </div>
  ` + section("About Acme Robotics", about) + `
  ` + section("Description", description) + `
  ` + pay + `
</div></body></html>`
}

// gustoFixture wires a board whose single page lists one posting, plus that posting's page.
func gustoFixture() *routedHTTP {
	return (&routedHTTP{}).
		route("page=1", gustoBoardHTML(gustoListingItemHTML(
			gustoTestPosting, "Senior Go Engineer", "Austin, TX",
			"$120,000 - $150,000 per year\n          &middot;\n        Full time"))).
		route("page=2", gustoBoardHTML()).
		route(gustoTestPosting, gustoPostingHTML(
			"Senior Go Engineer", "Austin, TX &middot; Full time",
			"We are hiring a backend engineer.",
			"Acme Robotics builds warehouse robots.",
			"<p>You will write Go.</p>", "$120,000 - $150,000 per year"))
}

func gustoEntry() CompanyEntry {
	return CompanyEntry{Company: "Acme Robotics", Provider: "gusto", Board: gustoTestBoard}
}

func TestGustoProvider(t *testing.T) {
	if got := NewGusto(nil).Provider(); got != "gusto" {
		t.Errorf("Provider() = %q, want %q", got, "gusto")
	}
}

func TestGustoRegisteredAndFacet(t *testing.T) {
	if _, ok := All(nil)["gusto"]; !ok {
		t.Fatal("gusto not registered in sources.All")
	}
	// One board is one employer, so gusto is board-keyed rather than boardless and belongs
	// in the source facet.
	if !slices.Contains(BoardKeyedProviders(Taxonomy()), "gusto") {
		t.Error("gusto should be board-keyed")
	}
	if !slices.Contains(FilterableProviders(), "gusto") {
		t.Error("gusto should appear in the source facet")
	}
	// The listing carries no body, so the adapter must hydrate only what the catalogue lacks.
	if _, ok := All(nil)["gusto"].(HydratingSource); !ok {
		t.Error("gusto should be a HydratingSource")
	}
}

func TestGustoPostingID(t *testing.T) {
	const id = "f6a50f1f-e924-45b1-86ad-c91287cdf4d9"
	cases := map[string]string{
		"/postings/grupo-ei-el-paso-warehouse-team-lead-" + id:                       id,
		"https://jobs.gusto.com/postings/grupo-ei-el-paso-warehouse-team-lead-" + id: id,
		"/postings/grupo-ei-el-paso-warehouse-team-lead-" + id + "?utm_source=x":     id, // query stripped
		// The apply route hangs off the posting path and is not a posting page.
		"/postings/grupo-ei-el-paso-warehouse-team-lead-" + id + "/applicants/new": "",
		"/boards/grupo-ei-el-paso-7feb4b68-5288-41cc-b169-f56b3ec26120":            "",
		"/postings/no-uuid-at-all": "",
	}
	for loc, want := range cases {
		if got := gustoPostingID(loc); got != want {
			t.Errorf("gustoPostingID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestGustoFetchMapsListingAndDetail(t *testing.T) {
	jobs, err := NewGusto(gustoFixture()).Fetch(context.Background(), gustoEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != gustoTestID {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, gustoTestID)
	}
	if j.URL != "https://jobs.gusto.com"+gustoTestPosting {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Acme Robotics" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Austin, TX" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Remote {
		t.Error("Remote = true, want false for a city location")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt != nil {
		t.Error("PostedAt should stay nil — neither page states a date")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 120000 || j.SalaryMax == nil || *j.SalaryMax != 150000 {
		t.Errorf("salary bounds = %v/%v, want 120000/150000", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "USD" || j.SalaryPeriod != "year" {
		t.Errorf("salary currency/period = %q/%q, want USD/year", j.SalaryCurrency, j.SalaryPeriod)
	}
	// The body is the summary plus the Description section; the "About <employer>" section
	// between them is boilerplate about the company, not the role.
	if !strings.Contains(j.Description, "We are hiring a backend engineer.") {
		t.Errorf("Description missing the summary: %q", j.Description)
	}
	if !strings.Contains(j.Description, "You will write Go.") {
		t.Errorf("Description missing the body: %q", j.Description)
	}
	if strings.Contains(j.Description, "warehouse robots") {
		t.Errorf("Description carries the About-the-employer section: %q", j.Description)
	}
}

func TestGustoFetchRemoteLocation(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", gustoBoardHTML(gustoListingItemHTML(
			gustoTestPosting, "Support Engineer", "Remote", "Part time"))).
		route("page=2", gustoBoardHTML()).
		route(gustoTestPosting, gustoPostingHTML("Support Engineer", "Remote &middot; Part time",
			"Help our users.", "", "<p>Answer tickets.</p>", ""))

	jobs, err := NewGusto(fake).Fetch(context.Background(), gustoEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if !j.Remote {
		t.Error("Remote = false, want true for a Remote location")
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time", j.EmploymentType)
	}
	// A row with no pay line must not read its employment type as a salary.
	if j.SalaryMin != nil || j.SalaryCurrency != "" {
		t.Errorf("unexpected salary %v %q", j.SalaryMin, j.SalaryCurrency)
	}
	// A posting page without an "About <employer>" section still yields the summary and body.
	if !strings.Contains(j.Description, "Help our users.") ||
		!strings.Contains(j.Description, "Answer tickets.") {
		t.Errorf("Description = %q", j.Description)
	}
}

func TestGustoFetchPaginatesUntilAnEmptyPage(t *testing.T) {
	second := "/postings/acme-robotics-platform-engineer-49dbe1c8-53cc-49db-8ebf-158f754d4284"
	fake := (&routedHTTP{}).
		route("page=1", gustoBoardHTML(gustoListingItemHTML(
			gustoTestPosting, "Senior Go Engineer", "Austin, TX", "Full time"))).
		route("page=2", gustoBoardHTML(gustoListingItemHTML(
			second, "Platform Engineer", "Austin, TX", "Full time"))).
		route("page=3", gustoBoardHTML()).
		route(gustoTestPosting, gustoPostingHTML("Senior Go Engineer", "Austin, TX &middot; Full time",
			"a", "", "<p>b</p>", "")).
		route(second, gustoPostingHTML("Platform Engineer", "Austin, TX &middot; Full time",
			"c", "", "<p>d</p>", ""))

	jobs, err := NewGusto(fake).Fetch(context.Background(), gustoEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (both pages)", len(jobs))
	}
}

func TestGustoFetchFirstPageFailureFailsTheBoard(t *testing.T) {
	// No route matches, so every page errors: the FIRST one failing is a board-level error.
	_, err := NewGusto(&routedHTTP{}).Fetch(context.Background(), gustoEntry())
	if err == nil {
		t.Fatal("expected a board-level error when the first listing page fails")
	}
	if !strings.Contains(err.Error(), gustoTestBoard) {
		t.Errorf("error should name the board, got %v", err)
	}
}

func TestGustoFetchLaterPageFailureKeepsWhatWasGathered(t *testing.T) {
	// Page 2 has no route and errors; page 1's posting still ingests.
	fake := (&routedHTTP{}).
		route("page=1", gustoBoardHTML(gustoListingItemHTML(
			gustoTestPosting, "Senior Go Engineer", "Austin, TX", "Full time"))).
		route(gustoTestPosting, gustoPostingHTML("Senior Go Engineer", "Austin, TX &middot; Full time",
			"a", "", "<p>b</p>", ""))

	jobs, err := NewGusto(fake).Fetch(context.Background(), gustoEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
}

func TestGustoFetchDropsAPostingWithNoBody(t *testing.T) {
	// A posting page whose Description section is missing yields nothing to index, so the
	// posting is deferred to the next crawl rather than stored body-less and never retried.
	fake := (&routedHTTP{}).
		route("page=1", gustoBoardHTML(gustoListingItemHTML(
			gustoTestPosting, "Senior Go Engineer", "Austin, TX", "Full time"))).
		route("page=2", gustoBoardHTML()).
		route(gustoTestPosting, `<html><body><h1><span>Senior Go Engineer</span></h1></body></html>`)

	jobs, err := NewGusto(fake).Fetch(context.Background(), gustoEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestGustoFetchNewSkipsDetailForASeenPosting(t *testing.T) {
	fake := gustoFixture()
	jobs, err := NewGusto(fake).(HydratingSource).FetchNew(context.Background(), gustoEntry(),
		func(id string) bool { return id == gustoTestID })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if !j.SeenRefresh {
		t.Error("a seen posting should be flagged SeenRefresh")
	}
	if j.Description != "" {
		t.Errorf("a refresh must carry no body, got %q", j.Description)
	}
	// The title travels with the refresh: the pipeline re-applies the catalogue filter to it.
	if j.Title != "Senior Go Engineer" || j.ExternalID != gustoTestID {
		t.Errorf("refresh identity = %q/%q", j.Title, j.ExternalID)
	}
	// Two listing pages and no posting page.
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (the listing only)", fake.calls)
	}
}

func TestGustoFetchNewHydratesAnUnseenPosting(t *testing.T) {
	jobs, err := NewGusto(gustoFixture()).(HydratingSource).FetchNew(context.Background(), gustoEntry(),
		func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].SeenRefresh {
		t.Error("an unseen posting must be hydrated, not refreshed")
	}
	if !strings.Contains(jobs[0].Description, "You will write Go.") {
		t.Errorf("Description = %q", jobs[0].Description)
	}
}

func TestGustoEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full time": "full_time", "Part time": "part_time",
		"Contractor": "contract", "Intern": "internship",
		"": "", "Seasonal": "", "full time": "",
	}
	for in, want := range cases {
		if got := gustoEmploymentType(in); got != want {
			t.Errorf("gustoEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGustoMetaLine(t *testing.T) {
	cases := []struct{ line, salary, employment string }{
		{"$70,000 - $90,000 per year · Full time", "$70,000 - $90,000 per year", "Full time"},
		{"$15 - $18 per hour · Part time", "$15 - $18 per hour", "Part time"},
		{"Contractor", "", "Contractor"}, // no pay stated
		{"", "", ""},
	}
	for _, c := range cases {
		salary, employment := gustoMetaLine(c.line)
		if salary != c.salary || employment != c.employment {
			t.Errorf("gustoMetaLine(%q) = %q/%q, want %q/%q",
				c.line, salary, employment, c.salary, c.employment)
		}
	}
}

func TestGustoApplySalary(t *testing.T) {
	cases := []struct {
		line     string
		min, max int
		period   string
	}{
		{"$70,000 - $90,000 per year", 70000, 90000, "year"},
		{"$15 - $18 per hour", 15, 18, "hour"},
		{"$18.75 - $22.40 per hour", 19, 22, "hour"}, // fractional bounds round
		{"$1,250 - $2,000 per month", 1250, 2000, "month"},
		{"$350 - $500 per day", 350, 500, "day"},
		// freehire has no weekly period, so a weekly range yields nothing rather than being
		// filed under the wrong one.
		{"$350 - $1,250 per week", 0, 0, ""},
		{"Competitive pay", 0, 0, ""},
		{"$70,000 per year", 0, 0, ""}, // one-sided: not the rendered shape
		{"", 0, 0, ""},
	}
	for _, c := range cases {
		var j Job
		gustoPosting{salary: c.line}.applySalary(&j)
		if c.period == "" {
			if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" || j.SalaryPeriod != "" {
				t.Errorf("applySalary(%q) set a salary, want none", c.line)
			}
			continue
		}
		if j.SalaryMin == nil || *j.SalaryMin != c.min || j.SalaryMax == nil || *j.SalaryMax != c.max {
			t.Errorf("applySalary(%q) bounds = %v/%v, want %d/%d", c.line, j.SalaryMin, j.SalaryMax, c.min, c.max)
			continue
		}
		if j.SalaryPeriod != c.period || j.SalaryCurrency != "USD" {
			t.Errorf("applySalary(%q) = %q/%q, want %q/USD",
				c.line, j.SalaryCurrency, j.SalaryPeriod, c.period)
		}
	}
}

func TestGustoBoardIdentity(t *testing.T) {
	const uuid = "aacf81cf-0249-436a-a514-3014fea74892"
	cases := map[string]string{
		// The rename twins: one employer, two slugs, the same uuid, both answering 200.
		"affordable-massage-company-" + uuid: uuid,
		"affordable-massage-studios-" + uuid: uuid,
		// The board route ignores a format suffix, so this is a third spelling of the same one.
		"affordable-massage-studios-" + uuid + ".json": uuid,
		// Nothing to fold on: kept verbatim rather than quietly merged into another board.
		"no-uuid-here": "no-uuid-here",
		"":             "",
	}
	for board, want := range cases {
		if got := gustoBoardIdentity(board); got != want {
			t.Errorf("gustoBoardIdentity(%q) = %q, want %q", board, got, want)
		}
	}
}

// Two spellings of one board in a board file must collapse to one crawl target: external ids
// are namespaced by board, so crawling both would store every posting of that employer twice.
func TestGustoBoardDedupeKeyFoldsASecondSpellingOfOneBoard(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Company: "Affordable Massage Studios", Provider: "gusto",
		Board: "affordable-massage-studios-aacf81cf-0249-436a-a514-3014fea74892"})
	b, _ := BoardDedupeKey(CompanyEntry{Company: "Affordable Massage Company", Provider: "gusto",
		Board: "affordable-massage-company-aacf81cf-0249-436a-a514-3014fea74892"})
	if a != b {
		t.Errorf("two spellings of one Gusto board should share a key, got %q and %q", a, b)
	}
	other, _ := BoardDedupeKey(CompanyEntry{Company: "Acme Robotics", Provider: "gusto", Board: gustoTestBoard})
	if other == a {
		t.Error("a different Gusto board must not fold onto this one")
	}
}

func TestGustoBoardURL(t *testing.T) {
	want := "https://jobs.gusto.com/boards/" + gustoTestBoard + "?page=2"
	if got := gustoBoardURL(gustoTestBoard, 2); got != want {
		t.Errorf("gustoBoardURL = %q, want %q", got, want)
	}
}
