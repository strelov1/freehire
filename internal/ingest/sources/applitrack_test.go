package sources

import (
	"context"
	"strings"
	"testing"
)

// applitrackWrite wraps markup the way Output.asp serves it: a document.write of a single-quoted
// JavaScript string, with every apostrophe in the markup backslash-escaped. Writing the fixtures
// through it keeps them readable as the markup they are.
func applitrackWrite(markup string) string {
	return "document.write('" + applitrackEscape(markup) + "')\n"
}

// applitrackEscape is the JavaScript string escaping applitrackWrite applies: the platform's
// markup quotes its attributes with the same apostrophe that terminates the string it is written
// through, so every one of them arrives escaped.
func applitrackEscape(markup string) string { return strings.ReplaceAll(markup, "'", `\'`) }

// applitrackMenuBody is the unfiltered listing: the district's own category menu and no postings.
// Its vocabulary is the shape the live fleet publishes — one plainly technical name, one plainly
// not, and two of the look-alikes the allowlist exists to turn away (Maine's word for a
// paraprofessional, and a teaching subject).
var applitrackMenuBody = "var VacanciesAreOnThisPage = false\n" +
	applitrackWrite(`<div id='AppliTrackSearchAdvancedContainer'>`+
		`<select name='AppliTrackSearchCategory' id='AppliTrackSearchCategory'>`+
		`<option style='color:#666;'>Select Category</option>`+
		`<option value='{id:"Food Service",vals:[""]}'>Food Service</option>`+
		`<option value='{id:"Ed Tech",vals:[""]}'>Ed Tech</option>`+
		`<option value='{id:"Career and Technical Education",vals:[""]}'>Career and Technical Education</option>`+
		`<option value='{id:"Technology",vals:["Network/Server Support"]}'>Technology</option>`+
		`</select></div>`)

// applitrackTechListingBody is the Technology category's slice. Posting 950 is listed twice, as a
// consortium lists one requisition once per participating district; both rows carry the one id.
var applitrackTechListingBody = "var VacanciesAreOnThisPage = true\n" +
	applitrackWrite(`<div id='AppliTrackListContent'><table class='AppliTrackPostingTable'>`+
		`<thead><tr><th>Description</th><th>Posted On</th></tr></thead><tbody>`+
		applitrackRow("739", "Network Engineer", "Central Office")+
		applitrackRow("812", "Desktop Technician", "Transportation")+
		applitrackRow("900", "Systems Analyst", "Central Office")+
		applitrackRow("950", "Help Desk Technician", "District Wide")+
		applitrackRow("950", "Help Desk Technician", "Second Campus")+
		`</tbody></table></div>`) +
	// The analytics tag is document.written too, but as an unescape() call rather than a string
	// literal, so nothing of it may reach the markup.
	`var gaJsHost = "https://ssl."; document.write(unescape("%3Cscript src='ga.js'%3E%3C/script%3E"));`

// applitrackMarkup unwraps a fixture's document.write calls, failing the test if the fixture
// itself is malformed rather than letting a half-read one look like a passing case.
func applitrackMarkup(t *testing.T, body string) string {
	t.Helper()
	markup, complete := applitrackWritten(body)
	if !complete {
		t.Fatalf("fixture has an unterminated document.write: %q", body)
	}
	return markup
}

// applitrackRow renders one posting's pair of listing rows.
func applitrackRow(id, title, site string) string {
	return `<tr valign='top' class='even'><td><span class='title'>` + title +
		` <a href='javascript:updateHrefFromCurrentWindowLocation("ApplitrackHardcodedURL%3F1%3D1` +
		`%26AppliTrackJobId%3D` + id + `%26AppliTrackLayoutMode%3Ddetail%26AppliTrackViewPosting%3D1")'` +
		` title='Click to view details of this posting'>view</a></span></td><td>9/1/2026</td></tr>` +
		`<tr valign='top' class='even'><td colspan='5'><span class='label'>ID:</span> ` + id +
		`&nbsp;&nbsp;<span class='label'>Location:</span> ` + site + `&nbsp;&nbsp;</td></tr>`
}

// applitrackDetail739 is a posting page carrying every field row the platform renders. The label
// spans are quoted inconsistently and padded with stray spaces exactly as the platform emits
// them, the body's own bullet list must not be mistaken for a field row, and the \x96 byte is
// windows-1252's en dash: AppliTrack declares no character set and serves these raw.
var applitrackDetail739 = applitrackWrite(
	`<div id='AppliTrackListContent'><p id='p739_h'><span class='ListHeader'>Openings as of 9/2/2026</span></p>` +
		`<ul class='postingsList' id='p739_'><table class='title'><tr><td id='wrapword'>Network Engineer</td>` +
		`<td><span class='title2'> JobID: 739 </span></td></tr></table><div style='position:relative;'>` +
		`<li><span class='label'>Position Type:</span><br/>&nbsp;&nbsp;<span class='normal'>Technology</span><br/><br/></li>` +
		`<li><span class="label" >Date Posted:</span><br/>&nbsp;&nbsp;<span class="normal">9/1/2026</span><br/><br/></li>` +
		`<li><span class="label" >Location:</span><br/>&nbsp;&nbsp;<span class="normal">Central Office</span><br/><br/></li>` +
		`<li><span class="label" > Closing Date: </span><br/>&nbsp;&nbsp;<span class='normal'>Open until Filled</span><br/><br/></li>` +
		`<span>&nbsp&nbsp</span><span class="normal"><p>Keep the district` + "\x96" +
		`wide network running.</p>` +
		`<ul><li>Cisco</li><li>Windows Server</li></ul><script>track()</script></span>` +
		`</div></ul></div>`)

// applitrackDetail812 states no location and no publish date — both rows are optional.
var applitrackDetail812 = applitrackWrite(
	`<div id='AppliTrackListContent'><ul class='postingsList' id='p812_'>` +
		`<table class='title'><tr><td id='wrapword'>Desktop Technician</td></tr></table><div style='position:relative;'>` +
		`<li><span class='label'>Position Type:</span><br/>&nbsp;&nbsp;<span class='normal'>Transportation</span><br/><br/></li>` +
		`<span>&nbsp&nbsp</span><span class="normal"><p>Reimage the lab carts.</p></span></div></ul></div>`)

// applitrackDetail900 is a posting whose employer attached a PDF instead of typing a body: every
// field row is there and no body span is.
var applitrackDetail900 = applitrackWrite(
	`<div id='AppliTrackListContent'><ul class='postingsList' id='p900_'>` +
		`<table class='title'><tr><td id='wrapword'>Systems Analyst</td></tr></table><div style='position:relative;'>` +
		`<li><span class="label" >Date Posted:</span><br/>&nbsp;&nbsp;<span class="normal">8/4/2026</span><br/><br/></li>` +
		`<span>&nbsp&nbsp</span><div class="AppliTrackJobPostingAttachments">Attachment(s):` +
		`<ul><li><a href="1BrowseFile.aspx?id=1">Systems Analyst.pdf</a></li></ul></div></div></ul></div>`)

// applitrackDetail950 is what the endpoint answers for an id the board no longer carries: 200,
// the page's own furniture, and an empty posting list.
var applitrackDetail950 = applitrackWrite(`<div id='AppliTrackListContent'></div>`)

func applitrackFake() *routedHTTP {
	// Contains-matching, so the detail routes (whose URLs carry "AppliTrackJobId=") precede the
	// listing ones, and the category slice precedes the unfiltered menu it is a longer form of.
	// Nothing routes the categories the allowlist rejects: a request for one is a test failure.
	return (&routedHTTP{}).
		route("AppliTrackJobId=739", applitrackDetail739).
		route("AppliTrackJobId=812", applitrackDetail812).
		route("AppliTrackJobId=900", applitrackDetail900).
		route("AppliTrackJobId=950", applitrackDetail950).
		route("category=Technology", applitrackTechListingBody).
		route("Output.asp?AppliTrackLayoutMode=condensed", applitrackMenuBody)
}

func applitrackEntry() CompanyEntry {
	return CompanyEntry{Company: "Acme Schools", Provider: "applitrack", Board: "acme"}
}

func TestAppliTrackProvider(t *testing.T) {
	if got := NewAppliTrack(nil).Provider(); got != "applitrack" {
		t.Errorf("Provider() = %q, want %q", got, "applitrack")
	}
}

func TestAppliTrackWritten(t *testing.T) {
	cases := []struct {
		body, want string
		complete   bool
	}{
		{`document.write('<b>hi</b>')`, "<b>hi</b>", true},
		// Two calls concatenate, and only the escaped character survives a backslash.
		{`document.write('<a class=\'x\'>')document.write('</a>')`, `<a class='x'></a>`, true},
		// The analytics tag is written through unescape(), not as a string literal.
		{`document.write(unescape("%3Cscript%3E"))`, "", true},
		{`no writes here`, "", true},
		// A call that runs off the end of the body is a response that was cut short. What it
		// carried parses, which is exactly why the caller must be told rather than left to walk
		// half a category and take it for the whole.
		{`document.write('<td>half a r`, "<td>half a r", false},
	}
	for _, c := range cases {
		got, complete := applitrackWritten(c.body)
		if got != c.want || complete != c.complete {
			t.Errorf("applitrackWritten(%q) = (%q, %v), want (%q, %v)",
				c.body, got, complete, c.want, c.complete)
		}
	}
}

func TestAppliTrackTruncatedResponseFailsBoard(t *testing.T) {
	// The markup before the cut parses and the rows before it read fine, so a crawl that
	// accepted it would take an incomplete category for a complete one — and on a
	// company-swept provider that closes every posting past the cut.
	cut := applitrackTechListingBody[:len(applitrackTechListingBody)/2]
	fake := (&routedHTTP{}).
		route("category=Technology", cut).
		route("Output.asp?AppliTrackLayoutMode=condensed", applitrackMenuBody)
	_, err := NewAppliTrack(fake).Fetch(context.Background(), applitrackEntry())
	if err == nil {
		t.Fatal("Fetch: got nil error, want the board to fail rather than under-list")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("Fetch error = %v, want it to say the response was cut short", err)
	}
}

func TestAppliTrackJobID(t *testing.T) {
	cases := map[string]string{
		`javascript:updateHrefFromCurrentWindowLocation("ApplitrackHardcodedURL%3F1%3D1%26AppliTrackJobId%3D739%26AppliTrackLayoutMode%3Ddetail")`: "739",
		"https://www.applitrack.com/acme/onlineapp/jobpostings/view.asp?AppliTrackJobId=4803":                                                      "4803",
		// Case varies across the platform's own links.
		"view.asp?applitrackjobid=12": "12",
		"view.asp?AppliTrackJobId=":   "",
		"onlineapp/default.aspx":      "",
	}
	for href, want := range cases {
		if got := applitrackJobID(href); got != want {
			t.Errorf("applitrackJobID(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestAppliTrackDate(t *testing.T) {
	posted := applitrackDate("9/1/2026")
	if posted == nil || posted.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("applitrackDate(%q) = %v, want 2026-09-01", "9/1/2026", posted)
	}
	// The neighbouring date rows are free text, and a row a posting leaves out reads as empty;
	// neither may become a publish date.
	for _, raw := range []string{"", "ASAP", "Open until Filled", "SY 26-27"} {
		if got := applitrackDate(raw); got != nil {
			t.Errorf("applitrackDate(%q) = %v, want nil", raw, got)
		}
	}
}

func TestAppliTrackFetchMapsPostings(t *testing.T) {
	jobs, err := NewAppliTrack(applitrackFake()).Fetch(context.Background(), applitrackEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the body-less posting and the one the board no longer "+
			"carries are both skipped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	a, ok := byID["739"]
	if !ok {
		t.Fatalf("missing job 739; got %v", byID)
	}
	if a.Title != "Network Engineer" {
		t.Errorf("job 739 title = %q, want %q (the row's text without its view control)",
			a.Title, "Network Engineer")
	}
	if a.Company != "Acme Schools" {
		t.Errorf("job 739 company = %q, want the configured district", a.Company)
	}
	if a.Location != "Central Office" {
		t.Errorf("job 739 location = %q, want %q", a.Location, "Central Office")
	}
	want := "https://www.applitrack.com/acme/onlineapp/jobpostings/view.asp?AppliTrackJobId=739"
	if a.URL != want {
		t.Errorf("job 739 URL = %q, want the human-facing permalink %q", a.URL, want)
	}
	if a.PostedAt == nil || a.PostedAt.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("job 739 PostedAt = %v, want 2026-09-01", a.PostedAt)
	}
	if !strings.Contains(a.Description, "district–wide") {
		t.Errorf("job 739 description = %q, want the windows-1252 en dash decoded", a.Description)
	}
	if !strings.Contains(a.Description, "<li>Cisco</li>") {
		t.Errorf("job 739 description = %q, want the body's own bullet list kept (it is not a "+
			"field row)", a.Description)
	}
	if strings.Contains(a.Description, "Position Type") || strings.Contains(a.Description, "<script>") {
		t.Errorf("job 739 description = %q, want the field rows left out and the script stripped",
			a.Description)
	}
	if a.Remote || a.WorkMode != "" {
		t.Errorf("job 739: got Remote=%v WorkMode=%q, want an on-site posting and no structured "+
			"work mode (the platform states none)", a.Remote, a.WorkMode)
	}

	b, ok := byID["812"]
	if !ok {
		t.Fatalf("missing job 812")
	}
	if b.Location != "" {
		t.Errorf("job 812 location = %q, want empty (the posting states no location row)", b.Location)
	}
	if b.PostedAt != nil {
		t.Errorf("job 812 PostedAt = %v, want nil (the posting states no date row)", b.PostedAt)
	}
}

func TestAppliTrackFetchNewSkipsSeenDetail(t *testing.T) {
	fake := applitrackFake()
	jobs, err := NewAppliTrack(fake).(HydratingSource).FetchNew(context.Background(),
		applitrackEntry(), func(id string) bool { return id != "739" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (every listed posting is reported; the seen ones need no "+
			"page of their own, so nothing about them can be skipped)", len(jobs))
	}
	// The category menu, the one technology category's listing, and the single unseen posting's
	// page — the three seen postings must cost no detail fetch.
	if fake.calls != 3 {
		t.Errorf("made %d requests, want 3 (menu, technology listing, one detail)", fake.calls)
	}
	for _, j := range jobs {
		if j.ExternalID == "739" {
			if j.SeenRefresh || j.Description == "" {
				t.Errorf("job 739 was unseen and must be hydrated: SeenRefresh=%v description=%q",
					j.SeenRefresh, j.Description)
			}
			continue
		}
		if !j.SeenRefresh {
			t.Errorf("job %s: SeenRefresh = false, want true", j.ExternalID)
		}
		if j.Description != "" {
			t.Errorf("job %s: a refresh must carry no content, got description %q",
				j.ExternalID, j.Description)
		}
		if j.Title == "" {
			t.Errorf("job %s: a refresh must carry the title, so the catalogue filter can be "+
				"re-applied to it", j.ExternalID)
		}
	}
}

func TestAppliTrackCrawlsOnlyTheTechnologyCategories(t *testing.T) {
	// The load-bearing decision: a district's board is almost all teaching and school-support
	// work that freehire's non-technical gate does not turn away, so the crawl reads only the
	// categories that district files IT work under. The menu here carries three it must not
	// read, two of them K-12 look-alikes, and the fake routes none of them — a request for one
	// errors, which fails the board rather than passing quietly.
	menu := parseHTML(t, applitrackMarkup(t, applitrackMenuBody))
	if got := applitrackTechnologyCategories(menu); len(got) != 1 || got[0] != "Technology" {
		t.Fatalf("applitrackTechnologyCategories = %q, want [Technology] — \"Ed Tech\" is Maine's "+
			"word for a paraprofessional and \"Career and Technical Education\" is a teaching subject",
			got)
	}
	jobs, err := NewAppliTrack(applitrackFake()).Fetch(context.Background(), applitrackEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want the technology category's two hydratable postings", len(jobs))
	}
}

func TestAppliTrackListingCollapsesRepeatedPostings(t *testing.T) {
	// A statewide consortium lists one requisition once per participating district, and every
	// one of those rows carries the same AppliTrackJobId with one posting page behind it. They
	// are one posting, so the walk keeps the first row and drops the rest.
	root := parseHTML(t, applitrackMarkup(t, applitrackTechListingBody))
	postings := applitrackListing(root)
	if len(postings) != 4 {
		t.Fatalf("got %d postings from 5 rows, want 4 (the repeated row is the same requisition)",
			len(postings))
	}
}

func TestAppliTrackBoardWithNoTechnologyCategoryYieldsNothing(t *testing.T) {
	// The intended answer, not a failure: the district has no IT opening filed as such, and the
	// crawl costs the one menu request that established it.
	menu := applitrackWrite(`<select id='AppliTrackSearchCategory'>` +
		`<option value='{id:"Food Service",vals:[""]}'>Food Service</option></select>`)
	fake := (&routedHTTP{}).route("Output.asp?AppliTrackLayoutMode=condensed", menu)
	jobs, err := NewAppliTrack(fake).Fetch(context.Background(), applitrackEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want none", len(jobs))
	}
	if fake.calls != 1 {
		t.Errorf("made %d requests, want 1 (the menu alone settles it)", fake.calls)
	}
}
