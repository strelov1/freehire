package sources

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The board and posting the fixtures use. Workstream keys a board by the eight hex characters
// its canonical career-site URL opens with, and a posting by the eight the URL ends with.
const (
	workstreamTestBoard   = "965a796b"
	workstreamTestPosting = "/j/965a796b/moxies/pickering-79247/line-cook-09eacfc8"
	workstreamTestID      = "09eacfc8"
)

// workstreamCardHTML renders one listing card the way www.workstream.us does: the posting link
// under an onclick twin, the schedule tags, the store's street address, the truncated blurb the
// adapter ignores, and the pay line beside its icon. tags and pay may be empty — an employer
// states neither by default.
func workstreamCardHTML(href, title, address, pay string, tags ...string) string {
	tagHTML := ""
	for _, t := range tags {
		tagHTML += `<span class="tag tag-small ml8px bg-flat-blue">` + t + `</span>`
	}
	payHTML := ""
	if pay != "" {
		payHTML = `<div class="flex align-items-center mt16px mr18px">` +
			`<img class="image-icons" src="/j/images/icon-rate-of-pay.svg" alt="#{t('Rate of pay')}" data-icon="rate-of-pay"/>` +
			`<span class="mute fz13px">` + pay + `</span></div>`
	}
	return `<div class="position-card mb16px pointer" onclick="location.href='` + href + `?locale=en'">
  <div class="flex justify-content-space-between"><span class="align-items-center">
    <a class="no-underline b fz16px black" href="` + href + `?locale=en">` + title + `</a>` + tagHTML + `
  </span></div>
  <div class="flex mt8px"><div class="flex-col" style="flex: 7"><div>
    <div class="position-address mute fz13px">` + address + `</div>
    <div class="position-short-desc fz13px">Our people are the heart and soul of our busine...</div>
  </div><div class="flex">` + payHTML + `</div></div>
  <div class="flex justify-content-center align-items-center" style="flex: 1">
    <a class="view-position-btn" href="` + href + `?locale=en" data-testid="view-position-btn">
      <img class="image-icons" src="/j/images/icon-right-arrow.svg" data-icon="right-arrow"/>
    </a>
  </div></div>
</div>`
}

// workstreamListingHTML renders a positions listing page around the given cards. searchBase is
// the URL the page states its own positions listing lives at, and totalPages the page count it
// publishes — the two inline variables the crawl steers by.
func workstreamListingHTML(searchBase string, totalPages int, cards ...string) string {
	return `<html><head><title>FineCasual Careers and Jobs</title></head><body>
<div class="body-wrapper"><div class="card-header mt40px">` + strings.Join(cards, "\n") + `</div>
<nav class="rounded-pagination"><ul class="pagination" id="pagination"></ul></nav></div>
<script>
var query = {"locale":"en"};
var currentPage = 1;
var totalPages = ` + strconv.Itoa(totalPages) + `;
var isNewUrl = true;
var searchBaseUrl = '` + searchBase + `';
</script></body></html>`
}

// workstreamPostingHTML renders a posting page: the breadcrumb, the schedule tags, and the one
// rich-text block that holds the body. body may be empty — the page a posting with no body
// renders carries no rich-text block at all.
func workstreamPostingHTML(title, body string) string {
	rich := ""
	if body != "" {
		rich = `<div class="position-rich-text-content mt18px">` + body + `</div>`
	}
	return `<html><head><title>` + title + `</title></head><body>
<ol class="breadcrumbs"><li class="breadcrumb-item"><span itemprop="name">Moxies Careers</span></li></ol>
<h1 class="company-name fz22px b black inline">` + title + `</h1>
<div class="flex mt16px position-metadata-tags"><span class="tag tag-small bg-flat-green">Full-time</span></div>
` + rich + `
<div class="border-top"><h2 class="fz19px black">Moxies - Pickering</h2></div>
</body></html>`
}

func workstreamEntry() CompanyEntry {
	return CompanyEntry{Provider: "workstream", Company: "FineCasual", Board: workstreamTestBoard}
}

// workstreamFixture is the multi-brand shape: "/j/<board>/positions" answers the employer-wide
// listing itself, so the page fetched to read searchBaseUrl IS page 1.
func workstreamFixture() *routedHTTP {
	listing := workstreamListingHTML(
		"https://www.workstream.us/j/"+workstreamTestBoard+"/positions", 1,
		workstreamCardHTML(workstreamTestPosting, "Line Cook",
			"1355 Kingston Rd, Pickering, ON L1V 1B8, Canada", "$17.00 - 20.00 per hour", "Full-time"))
	return (&routedHTTP{}).
		route("page=2", workstreamListingHTML(
			"https://www.workstream.us/j/"+workstreamTestBoard+"/positions", 1)).
		route(workstreamTestPosting, workstreamPostingHTML("Line Cook", "<p>You will cook.</p>")).
		route("/positions", listing)
}

func TestWorkstreamProvider(t *testing.T) {
	if got := NewWorkstream(nil).Provider(); got != "workstream" {
		t.Errorf("Provider() = %q, want %q", got, "workstream")
	}
}

func TestWorkstreamRegisteredAndFacet(t *testing.T) {
	if _, ok := All(nil)["workstream"]; !ok {
		t.Fatal("workstream not registered in sources.All")
	}
	// One board is one employer account, so workstream is board-keyed rather than boardless and
	// belongs in the source facet.
	if !slices.Contains(BoardKeyedProviders(Taxonomy()), "workstream") {
		t.Error("workstream should be board-keyed")
	}
	if !slices.Contains(FilterableProviders(), "workstream") {
		t.Error("workstream should appear in the source facet")
	}
	// A listing card carries no body, so the adapter must hydrate only what the catalogue lacks.
	if _, ok := All(nil)["workstream"].(HydratingSource); !ok {
		t.Error("workstream should be a HydratingSource")
	}
}

func TestWorkstreamPostingID(t *testing.T) {
	cases := map[string]string{
		workstreamTestPosting:                                   workstreamTestID,
		"https://www.workstream.us" + workstreamTestPosting:     workstreamTestID,
		workstreamTestPosting + "?locale=en":                    workstreamTestID, // query stripped
		workstreamTestPosting + "/apply":                        "",               // the apply route is not a posting
		"/j/965a796b/moxies/pickering-79247":                    "",               // a store page
		"/j/965a796b/moxies":                                    "",               // a brand page
		"/j/965a796b/positions":                                 "",               // the listing itself
		"/j/965a796b/moxies/pickering-79247/line-cook-09EACFC8": "",               // ids are lower-case hex
		"/j/965a796b/moxies/pickering-79247/line-cook":          "",
	}
	for loc, want := range cases {
		if got := workstreamPostingID(loc); got != want {
			t.Errorf("workstreamPostingID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestWorkstreamFetchMapsListingAndDetail(t *testing.T) {
	jobs, err := NewWorkstream(workstreamFixture()).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != workstreamTestID {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, workstreamTestID)
	}
	// The stored URL is the posting's own, without the locale the crawl asked for.
	if j.URL != "https://www.workstream.us"+workstreamTestPosting {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Line Cook" {
		t.Errorf("Title = %q", j.Title)
	}
	// The employer is the board's account, not the brand the posting trades under.
	if j.Company != "FineCasual" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "1355 Kingston Rd, Pickering, ON L1V 1B8, Canada" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Remote {
		t.Error("Remote = true, want false for a street address")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt != nil {
		t.Error("PostedAt should stay nil — neither page states a date")
	}
	if !strings.Contains(j.Description, "You will cook.") {
		t.Errorf("Description missing the body: %q", j.Description)
	}
	// The pay line is stated, never parsed: the "$" is unqualified and this posting is Canadian.
	if !strings.Contains(j.Description, "Pay: $17.00 - 20.00 per hour") {
		t.Errorf("Description missing the pay line: %q", j.Description)
	}
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" || j.SalaryPeriod != "" {
		t.Errorf("salary fields should stay empty, got %v/%v %q/%q",
			j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
}

func TestWorkstreamFetchFollowsTheListingURLThePageStates(t *testing.T) {
	// The single-brand shape: "/j/<board>/positions" is redirected to the brand root, whose
	// searchBaseUrl names the brand's own positions listing.
	const brandListing = "https://www.workstream.us/j/" + workstreamTestBoard + "/moxies/positions"
	fake := (&routedHTTP{}).
		route("page=2", workstreamListingHTML(brandListing, 1)).
		route(workstreamTestPosting, workstreamPostingHTML("Line Cook", "<p>You will cook.</p>")).
		route("/moxies/positions", workstreamListingHTML(brandListing, 1,
			workstreamCardHTML(workstreamTestPosting, "Line Cook", "Pickering, ON", "", "Full-time"))).
		// The brand root: a locations listing that carries no cards, and states where the
		// positions are.
		route("/positions", workstreamListingHTML(brandListing, 1))

	jobs, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 from the listing the page pointed at", len(jobs))
	}
}

func TestWorkstreamFetchFailsABoardThatStatesNoListing(t *testing.T) {
	fake := (&routedHTTP{}).route("/positions", `<html><body>Not a career site</body></html>`)
	if _, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry()); err == nil {
		t.Fatal("Fetch should fail when the page states no positions listing")
	}
}

func TestWorkstreamFetchWalksEveryPageThePageCountStates(t *testing.T) {
	const base = "https://www.workstream.us/j/" + workstreamTestBoard + "/positions"
	card := func(id, title string) string {
		return workstreamCardHTML("/j/965a796b/moxies/pickering-79247/"+title+"-"+id,
			title, "Pickering, ON", "")
	}
	fake := (&routedHTTP{}).
		route("page=2", workstreamListingHTML(base, 2, card("0000000b", "server"))).
		// Nothing routes page 3: totalPages says there are two, so the walk must not ask.
		route("/positions", workstreamListingHTML(base, 2, card("0000000a", "cook"))).
		route("/j/965a796b/moxies/", workstreamPostingHTML("Role", "<p>Body.</p>"))

	jobs, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (one per page)", len(jobs))
	}
}

func TestWorkstreamFetchFirstPageFailureFailsTheBoard(t *testing.T) {
	// No routes at all: the listing fetch errors, which is a board-level failure.
	if _, err := NewWorkstream(&routedHTTP{}).Fetch(context.Background(), workstreamEntry()); err == nil {
		t.Fatal("Fetch should fail when the first listing page cannot be fetched")
	}
}

func TestWorkstreamFetchLaterPageFailureKeepsWhatWasGathered(t *testing.T) {
	const base = "https://www.workstream.us/j/" + workstreamTestBoard + "/positions"
	fake := (&routedHTTP{}).
		// page=2 is unrouted, so it errors — the walk ends with page 1's posting.
		route("/positions", workstreamListingHTML(base, 3,
			workstreamCardHTML(workstreamTestPosting, "Line Cook", "Pickering, ON", ""))).
		route(workstreamTestPosting, workstreamPostingHTML("Line Cook", "<p>You will cook.</p>"))

	jobs, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch should not fail on a later page: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want the 1 gathered before the failure", len(jobs))
	}
}

func TestWorkstreamFetchDropsAPostingWithNoBody(t *testing.T) {
	const base = "https://www.workstream.us/j/" + workstreamTestBoard + "/positions"
	fake := (&routedHTTP{}).
		route("page=2", workstreamListingHTML(base, 1)).
		route(workstreamTestPosting, workstreamPostingHTML("Line Cook", "")).
		route("/positions", workstreamListingHTML(base, 1,
			workstreamCardHTML(workstreamTestPosting, "Line Cook", "Pickering, ON", "")))

	jobs, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Storing it body-less would make it `seen` and so never hydrated again; deferring it by one
	// crawl is recoverable.
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want the body-less posting dropped", len(jobs))
	}
}

func TestWorkstreamFetchNewSkipsDetailForASeenPosting(t *testing.T) {
	fake := workstreamFixture()
	jobs, err := NewWorkstream(fake).(HydratingSource).
		FetchNew(context.Background(), workstreamEntry(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !jobs[0].SeenRefresh {
		t.Error("a seen posting should be flagged SeenRefresh")
	}
	if jobs[0].Description != "" {
		t.Errorf("a refresh must carry no content, got %q", jobs[0].Description)
	}
	// The title travels so the refresh path can re-apply the catalogue filter to it.
	if jobs[0].Title != "Line Cook" {
		t.Errorf("Title = %q", jobs[0].Title)
	}
	if fake.calls != 1 {
		t.Errorf("fetched %d pages, want only the listing", fake.calls)
	}
}

func TestWorkstreamFetchNewHydratesAnUnseenPosting(t *testing.T) {
	jobs, err := NewWorkstream(workstreamFixture()).(HydratingSource).
		FetchNew(context.Background(), workstreamEntry(), func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].SeenRefresh {
		t.Error("an unseen posting should not be flagged SeenRefresh")
	}
	if !strings.Contains(jobs[0].Description, "You will cook.") {
		t.Errorf("Description = %q", jobs[0].Description)
	}
}

func TestWorkstreamCardWithSeveralSchedulesStatesNoEmploymentType(t *testing.T) {
	const base = "https://www.workstream.us/j/" + workstreamTestBoard + "/positions"
	fake := (&routedHTTP{}).
		route("page=2", workstreamListingHTML(base, 1)).
		route(workstreamTestPosting, workstreamPostingHTML("Line Cook", "<p>You will cook.</p>")).
		route("/positions", workstreamListingHTML(base, 1,
			workstreamCardHTML(workstreamTestPosting, "Line Cook", "Pickering, ON", "",
				"Full-time", "Part-time")))

	jobs, err := NewWorkstream(fake).Fetch(context.Background(), workstreamEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	// Two schedules state no single type, so the description parser decides.
	if jobs[0].EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want empty for a posting offering both", jobs[0].EmploymentType)
	}
}

func TestWorkstreamJobStatesNoPayLineWhenTheEmployerStatesNone(t *testing.T) {
	p := workstreamPosting{id: workstreamTestID, title: "Line Cook"}
	if got := p.job(workstreamEntry(), "<p>Body.</p>").Description; got != "<p>Body.</p>" {
		t.Errorf("Description = %q, want the body alone", got)
	}
}

func TestWorkstreamEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full-time": "full_time",
		"Part-time": "part_time",
		"":          "",
		"Seasonal":  "",
	}
	for tag, want := range cases {
		if got := workstreamEmploymentType(tag); got != want {
			t.Errorf("workstreamEmploymentType(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestWorkstreamPositionsURL(t *testing.T) {
	const want = "https://www.workstream.us/j/965a796b/positions"
	if got := workstreamPositionsURL(workstreamTestBoard); got != want {
		t.Errorf("workstreamPositionsURL = %q, want %q", got, want)
	}
}

func TestWorkstreamPageURL(t *testing.T) {
	cases := map[string]string{
		"https://www.workstream.us/j/965a796b/positions": "https://www.workstream.us/j/965a796b/positions?page=3",
		// A listing URL that ever states a query of its own keeps it, and the page parameter is
		// SET rather than appended so it cannot arrive twice.
		"https://www.workstream.us/j/965a796b/positions?locale=en": "https://www.workstream.us/j/965a796b/positions?locale=en&page=3",
		"https://www.workstream.us/j/965a796b/positions?page=1":    "https://www.workstream.us/j/965a796b/positions?page=3",
	}
	for base, want := range cases {
		u, err := url.Parse(base)
		if err != nil {
			t.Fatalf("parse %q: %v", base, err)
		}
		if got := workstreamPageURL(u, 3); got != want {
			t.Errorf("workstreamPageURL(%q, 3) = %q, want %q", base, got, want)
		}
	}
}
