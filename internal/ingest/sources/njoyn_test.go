package sources

import (
	"context"
	"embed"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
)

// The fixtures under testdata/njoyn were recorded from CGI's site (cgi.njoyn.com, clid 21001) on
// 2026-11-17 and trimmed to what the adapter reads:
//   - cgi_listing_page1/2: the results table of listing pages 1 and 61 of 61, cut to three and
//     two rows, with the pager rewritten to "Page 1 of 2" and "Page 2 of 2" so the pair is a
//     whole board.
//   - cgi_detail_J1126-1603 (English) and cgi_detail_J1126-1479_fr (French): the JobPosting
//     ld+json and the posting's section, without the application form after it.
//   - cgi_detail_closed: the notice served for a posting id that is gone.
//   - cgi_refused: the whole body the site served every request once it refused the client,
//     with a documentation address in place of the client's IP.
//   - laval_listing_page1 and laval_detail_J1126-0070: the second site template, Société de
//     transport de Laval's (lavaltransit.njoyn.com, site CL2, clid 60406), recorded through the
//     Scrapling tier on 2026-11-22: the #searchtable listing cut to three rows, and a posting's
//     ld+json and #njnContentContainer. Session tokens (tbtoken, chk) are replaced.
//   - radware_block: Radware's bot wall (validate.perfdrive.com), where a datacenter IP is
//     redirected, recorded from the crawl host 2026-11-17 and cut to its title and CAPTCHA
//     notice, without the per-request tokens and the client's address.
//
//go:embed testdata/njoyn
var njoynFixtures embed.FS

func njoynFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := njoynFixtures.ReadFile("testdata/njoyn/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func njoynParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(njoynFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

var cgiEntry = CompanyEntry{Company: "CGI", Provider: "njoyn", Board: "cgi/corp/21001"}

// testNjoyn is the adapter without its request pacing.
func testNjoyn(c HTMLGetter) njoyn { return njoyn{http: c, maxNewPerBoard: njoynMaxNewPerBoard} }

// cgiBoard routes the two listing pages.
func cgiBoard(t *testing.T) *routedHTTP {
	t.Helper()
	return (&routedHTTP{}).
		route("Page=joblisting&lang=1&pn=1", njoynFixture(t, "cgi_listing_page1.html")).
		route("Page=joblisting&lang=1&pn=2", njoynFixture(t, "cgi_listing_page2.html"))
}

func TestNjoynProvider(t *testing.T) {
	if got := NewNjoyn(nil).Provider(); got != "njoyn" {
		t.Errorf("Provider() = %q, want njoyn", got)
	}
	if _, ok := NewNjoyn(nil).(HydratingSource); !ok {
		t.Error("njoyn must be a HydratingSource")
	}
	if _, ok := NewNjoyn(nil).(fullBoardListing); !ok {
		t.Error("njoyn must carry the fullBoardListing marker")
	}
	if _, ok := proxiedProviders["njoyn"]; !ok {
		t.Error("njoyn must be in proxiedProviders (Radware walls the prod datacenter IP)")
	}
	if r, ok := scraplingProviders["njoyn"]; !ok || r.route.mode != scraplingfetch.ModeHTTP || !r.route.proxy {
		t.Error("njoyn must ride the Scrapling tier over HTTP + proxy (the fixed proxy exit drew XWP10022)")
	}
}

func TestNjoynBoard(t *testing.T) {
	b, err := parseNjoynBoard("CGI/CORP/21001")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := b.listingURL(2), "https://cgi.njoyn.com/corp/xweb/xweb.asp?NTKN=c&clid=21001&Page=joblisting&lang=1&pn=2"; got != want {
		t.Errorf("listingURL = %q, want %q", got, want)
	}
	for _, bad := range []string{"", "cgi", "cgi/corp", "cgi/corp/abc", "cgi.njoyn.com/corp/21001"} {
		if _, err := parseNjoynBoard(bad); err == nil {
			t.Errorf("parseNjoynBoard(%q) accepted a malformed board", bad)
		}
	}
}

func TestNjoynListingPage(t *testing.T) {
	rows, at, of, err := njoynListingPage(njoynParse(t, "cgi_listing_page1.html"))
	if err != nil {
		t.Fatal(err)
	}
	if at != 1 || of != 2 || len(rows) != 3 {
		t.Fatalf("page = %d of %d with %d rows, want 1 of 2 with 3", at, of, len(rows))
	}
	if r := rows[0]; r.id != "J0626-0094" || r.brid != "1307933" || r.country != "United States" ||
		r.title != "SAP Training Specialist (Mid) - U.S. Citizenship Required" {
		t.Errorf("first row = %+v", r)
	}
	if r := rows[2]; r.id != "J1126-1603" || r.city != "Montréal" || r.country != "Canada" {
		t.Errorf("third row = %+v", r)
	}
}

func TestNjoynListingPageRejectsANonListing(t *testing.T) {
	if _, _, _, err := njoynListingPage(njoynParse(t, "cgi_detail_J1126-1603.html")); err == nil {
		t.Error("a posting page was read as a listing")
	}
	for _, page := range []string{"cgi_refused.html", "radware_block.html"} {
		if _, _, _, err := njoynListingPage(njoynParse(t, page)); !errors.Is(err, errNjoynRefused) {
			t.Errorf("%s read as %v, want errNjoynRefused", page, err)
		}
	}
}

func TestNjoynFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	fake := cgiBoard(t).route("Jobid=J1126-1603", njoynFixture(t, "cgi_detail_J1126-1603.html"))
	jobs, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry,
		func(id string) bool { return id != "J1126-1603" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want the board's 5", len(jobs))
	}
	if fake.calls != 2+1 {
		t.Errorf("made %d requests, want two listing pages and one posting page", fake.calls)
	}
	for _, j := range jobs {
		if (j.ExternalID == "J1126-1603") == j.SeenRefresh {
			t.Errorf("%s: SeenRefresh = %v", j.ExternalID, j.SeenRefresh)
		}
		if j.ExternalID == "J0925-1760" && j.Location != "Warsaw, Poland" {
			t.Errorf("a seen posting's place = %q, want the listing's", j.Location)
		}
	}
}

func TestNjoynFetchNewFailsWhenItReadsNoneOfItsNewPostings(t *testing.T) {
	fake := cgiBoard(t).route("Page=JobDetails", njoynFixture(t, "cgi_refused.html"))
	_, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry,
		func(id string) bool { return id != "J1126-1603" && id != "J1126-1490" })
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want a failed crawl", err)
	}
}

func TestNjoynFetchNewFailsOnARefusedListingPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("pn=1", njoynFixture(t, "cgi_listing_page1.html")).
		route("pn=2", njoynFixture(t, "cgi_refused.html"))
	if _, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(string) bool { return true }); !errors.Is(err, errNjoynRefused) {
		t.Errorf("err = %v, want the refusal: a board cut short must not read as a smaller board", err)
	}
}

func TestNjoynDetailEnglish(t *testing.T) {
	fake := (&routedHTTP{}).route("Jobid=J1126-1603", njoynFixture(t, "cgi_detail_J1126-1603.html"))
	b, _ := parseNjoynBoard(cgiEntry.Board)
	j, err := njoynDetail(context.Background(), fake, cgiEntry, b,
		njoynRow{id: "J1126-1603", brid: "1350470", title: "IT Service Transition Consultant", city: "Montréal", country: "Canada"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Location != "Montréal, QC, CA" || len(j.Countries) != 1 || j.Countries[0] != "ca" {
		t.Errorf("Location = %q, Countries = %v", j.Location, j.Countries)
	}
	// The whole posting, not the ld+json's first section.
	for _, want := range []string{"Operations Center", "Required qualifications"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description lacks %q", want)
		}
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-17" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2027-01-16" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if !strings.Contains(j.URL, "Jobid=J1126-1603") || !strings.Contains(j.URL, "BRID=1350470") {
		t.Errorf("URL = %q", j.URL)
	}
}

func TestNjoynDetailFrench(t *testing.T) {
	fake := (&routedHTTP{}).route("Jobid=J1126-1479", njoynFixture(t, "cgi_detail_J1126-1479_fr.html"))
	b, _ := parseNjoynBoard(cgiEntry.Board)
	j, err := njoynDetail(context.Background(), fake, cgiEntry, b,
		njoynRow{id: "J1126-1479", brid: "1350158", title: "Développeur(se) Back-End C – Systèmes critiques", city: "Montréal", country: "Canada"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Title != "Développeur(se) Back-End C – Systèmes critiques" || j.Location != "Montréal, QC, CA" {
		t.Errorf("job = %q in %q", j.Title, j.Location)
	}
	if !strings.Contains(j.Description, "propriétaires") || len(j.Description) < 1000 {
		t.Errorf("description is %d bytes, want the whole French posting", len(j.Description))
	}
}

func TestNjoynDetailDefersAPostingItCannotRead(t *testing.T) {
	b, _ := parseNjoynBoard(cgiEntry.Board)
	for page, want := range map[string]error{
		"cgi_detail_closed.html": errNjoynNoPosting,
		"cgi_refused.html":       errNjoynRefused,
		"radware_block.html":     errNjoynRefused,
	} {
		fake := (&routedHTTP{}).route("Jobid=J0000-0001", njoynFixture(t, page))
		if j, err := njoynDetail(context.Background(), fake, cgiEntry, b, njoynRow{id: "J0000-0001", title: "x"}); !errors.Is(err, want) {
			t.Errorf("%s: read as %+v, %v; want the posting deferred with %v", page, j, err, want)
		}
	}
}

func TestNjoynFetchNewHydratesTheNewestPostingsUpToTheBound(t *testing.T) {
	detail := njoynFixture(t, "cgi_detail_J1126-1603.html")
	// Only the two newest ids are routed: asking for any other posting page fails the request.
	fake := cgiBoard(t).route("Jobid=J1126-1603", detail).route("Jobid=J1126-1490", detail)
	s := testNjoyn(fake)
	s.maxNewPerBoard = 2
	jobs, err := s.FetchNew(context.Background(), cgiEntry, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2+2 {
		t.Errorf("made %d requests, want two listing pages and the bound's two posting pages", fake.calls)
	}
	if len(jobs) != 2 {
		t.Errorf("got %d jobs, want the two hydrated: a posting past the bound waits for a later run", len(jobs))
	}
}

func TestNjoynNewestFirstOrdersByTheMonthTheIDNames(t *testing.T) {
	rows := []njoynRow{{id: "J0925-1760"}, {id: "odd"}, {id: "J1126-1490"}, {id: "J1225-0712"}, {id: "J0626-0094"}, {id: "J1126-1603"}}
	sortNjoynNewestFirst(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.id)
	}
	if want := "J1126-1603 J1126-1490 J0626-0094 J1225-0712 J0925-1760 odd"; strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

func TestNjoynFetchNewStopsAskingOnceRefused(t *testing.T) {
	fake := cgiBoard(t).route("Page=JobDetails", njoynFixture(t, "cgi_refused.html"))
	_, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "read none") {
		t.Errorf("err = %v, want a failed crawl", err)
	}
	if fake.calls != 2+1 {
		t.Errorf("made %d requests, want two listing pages and the one refused posting page", fake.calls)
	}
}

func TestNjoynFetchNewKeepsWhatItReadBeforeARefusal(t *testing.T) {
	fake := cgiBoard(t).
		route("Jobid=J1126-1603", njoynFixture(t, "cgi_detail_J1126-1603.html")).
		route("Page=JobDetails", njoynFixture(t, "cgi_refused.html"))
	jobs, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(id string) bool { return id == "J0925-1760" })
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2+2 {
		t.Errorf("made %d requests, want two listing pages, the read posting and the refused one", fake.calls)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	if strings.Join(ids, " ") != "J1126-1603 J0925-1760" {
		t.Errorf("jobs = %v, want the read posting and the seen one", ids)
	}
}

func TestNjoynPlacesJoinsEveryLocation(t *testing.T) {
	var places schemaPlaces
	for _, a := range []schemaAddress{
		{AddressLocality: "Montréal", AddressRegion: "QC", AddressCountry: "CA "},
		{AddressLocality: "Calgary", AddressRegion: "AB", AddressCountry: "CA"},
	} {
		places = append(places, schemaPlace{Address: a})
	}
	loc, countries := njoynPlaces(places)
	if loc != "Montréal, QC, CA; Calgary, AB, CA" || len(countries) != 1 || countries[0] != "ca" {
		t.Errorf("places = %q %v", loc, countries)
	}
}

// clients.njoyn.com serves every client by clid, so a board there is the client's own board
// under a second name: the two fold to one identity, and the generic one is refused on its own.
func TestNjoynClientsHostIsTheClientsOwnBoard(t *testing.T) {
	own, _ := BoardDedupeKey(CompanyEntry{Provider: "njoyn", Board: "cgi/corp/21001"})
	generic, _ := BoardDedupeKey(CompanyEntry{Provider: "njoyn", Board: "clients/corp/21001"})
	other, _ := BoardDedupeKey(CompanyEntry{Provider: "njoyn", Board: "cgi/corp/21002"})
	if own != generic {
		t.Errorf("cgi/corp/21001 and clients/corp/21001 fold to %q and %q, want one identity", own, generic)
	}
	if own == other {
		t.Error("a different clid folded to the same identity")
	}
	registry := map[string]Source{"njoyn": NewNjoyn(nil)}
	entry := func(board string) Config {
		return Config{Provider: "njoyn", Sources: []CompanyEntry{{Provider: "njoyn", Board: board, Company: "CGI"}}}
	}
	if err := entry("clients/corp/21001").Validate(registry); err == nil || !strings.Contains(err.Error(), "serves every client") {
		t.Errorf("a board on the generic host validated: %v", err)
	}
	if err := entry("cgi/corp/21001").Validate(registry); err != nil {
		t.Errorf("the client's own board failed validation: %v", err)
	}
}

// flakyListing serves a page that is not the listing for the first bad answers to one URL,
// then the real page: what the rotating proxy does now and then to one of a board's pages.
type flakyListing struct {
	*routedHTTP
	match string
	bad   int
	asked int
}

func (f *flakyListing) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if strings.Contains(url, f.match) {
		f.asked++
		if f.asked <= f.bad {
			return html.Parse(strings.NewReader("<html><body><p>Something else</p></body></html>"))
		}
	}
	return f.routedHTTP.GetHTML(ctx, url)
}

func TestNjoynListingAsksAgainForAPageWithoutTheResultsTable(t *testing.T) {
	fake := &flakyListing{routedHTTP: cgiBoard(t), match: "pn=2", bad: 1}
	jobs, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(string) bool { return true })
	if err != nil {
		t.Fatalf("one bad answer failed the crawl: %v", err)
	}
	if len(jobs) != 5 || fake.asked != 2 {
		t.Errorf("got %d jobs asking page 2 %d times, want the board's 5 after asking twice", len(jobs), fake.asked)
	}
}

func TestNjoynListingFailsWhenEveryAttemptMissesTheTable(t *testing.T) {
	fake := &flakyListing{routedHTTP: cgiBoard(t), match: "pn=2", bad: njoynListingMaxAttempts}
	_, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(string) bool { return true })
	if !errors.Is(err, errNjoynNoListing) || fake.asked != njoynListingMaxAttempts {
		t.Errorf("err = %v after %d attempts, want errNjoynNoListing after %d", err, fake.asked, njoynListingMaxAttempts)
	}
}

func TestNjoynListingDoesNotAskAgainAfterARefusal(t *testing.T) {
	fake := (&routedHTTP{}).
		route("pn=1", njoynFixture(t, "cgi_listing_page1.html")).
		route("pn=2", njoynFixture(t, "cgi_refused.html"))
	if _, err := testNjoyn(fake).FetchNew(context.Background(), cgiEntry, func(string) bool { return true }); !errors.Is(err, errNjoynRefused) || fake.calls != 2 {
		t.Errorf("err = %v after %d requests, want the refusal after 2", err, fake.calls)
	}
}

var lavalEntry = CompanyEntry{Company: "Société de transport de Laval", Provider: "njoyn", Board: "lavaltransit/cl2/60406"}

func TestNjoynReadsTheSearchtableTemplate(t *testing.T) {
	rows, at, of, err := njoynListingPage(njoynParse(t, "laval_listing_page1.html"))
	if err != nil {
		t.Fatal(err)
	}
	if at != 1 || of != 1 || len(rows) != 3 {
		t.Fatalf("page = %d of %d with %d rows, want 1 of 1 with 3", at, of, len(rows))
	}
	// No id column: the identity is the link's Jobid; no country column either.
	if r := rows[0]; r.id != "J1126-0070" || r.brid != "383743" || r.title != "Conseiller∙ère en Cybersécurité" || r.city != "Laval" || r.country != "" {
		t.Errorf("first row = %+v", r)
	}
}

func TestNjoynFetchesALavalPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("Page=joblisting&lang=1&pn=1", njoynFixture(t, "laval_listing_page1.html")).
		route("Jobid=J1126-0070", njoynFixture(t, "laval_detail_J1126-0070.html"))
	jobs, err := testNjoyn(fake).FetchNew(context.Background(), lavalEntry, func(id string) bool { return id != "J1126-0070" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want the board's 3", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "J1126-0070" || j.SeenRefresh {
		t.Fatalf("first job = %s (SeenRefresh %v), want the hydrated J1126-0070", j.ExternalID, j.SeenRefresh)
	}
	if !strings.Contains(j.Description, "En accompagnement") || !strings.Contains(j.Description, "Conditions de travail") {
		t.Errorf("description lacks the posting: %.200q", j.Description)
	}
	if j.Location != "Laval, QC, CA" || len(j.Countries) != 1 || j.Countries[0] != "ca" {
		t.Errorf("Location = %q, Countries = %v", j.Location, j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-05" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-22" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if !strings.Contains(j.URL, "lavaltransit.njoyn.com/cl2/") || !strings.Contains(j.URL, "Jobid=J1126-0070") {
		t.Errorf("URL = %q", j.URL)
	}
}
