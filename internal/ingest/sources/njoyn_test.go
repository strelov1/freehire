package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
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
func njoynFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/njoyn/" + name)
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
func testNjoyn(c HTMLGetter) njoyn { return njoyn{http: c} }

// cgiBoard routes the two listing pages.
func cgiBoard(t *testing.T) *routedHTTP {
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
	if _, _, _, err := njoynListingPage(njoynParse(t, "cgi_refused.html")); !errors.Is(err, errNjoynRefused) {
		t.Errorf("the refusal read as %v, want errNjoynRefused", err)
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
	j, ok := njoynDetail(context.Background(), fake, cgiEntry, b,
		njoynRow{id: "J1126-1603", brid: "1350470", title: "IT Service Transition Consultant", city: "Montréal", country: "Canada"})
	if !ok {
		t.Fatal("detail not read")
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
	j, ok := njoynDetail(context.Background(), fake, cgiEntry, b,
		njoynRow{id: "J1126-1479", brid: "1350158", title: "Développeur(se) Back-End C – Systèmes critiques", city: "Montréal", country: "Canada"})
	if !ok {
		t.Fatal("detail not read")
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
	for _, page := range []string{"cgi_detail_closed.html", "cgi_refused.html"} {
		fake := (&routedHTTP{}).route("Jobid=J0000-0001", njoynFixture(t, page))
		if j, ok := njoynDetail(context.Background(), fake, cgiEntry, b, njoynRow{id: "J0000-0001", title: "x"}); ok {
			t.Errorf("%s: read as %+v, want the posting deferred", page, j)
		}
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
