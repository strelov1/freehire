package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures under testdata/idealtraits were recorded live on 2026-10-20 from the Comma
// Insurance board (business id 1162292, five postings) and one posting of another board
// (370499, the only shape of the three "Remote Type" values Comma's postings lack). The posting
// pages are byte-for-byte what the site served except for third-party widget tokens (a reCAPTCHA
// site key, two apply-widget API tokens, Cloudflare beacon ids), replaced with "redacted" so the
// secret scanner does not read them as leaked credentials; the adapter reads none of them.
// listing_8739_empty.json is a real board with no openings.

func idealtraitsFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/idealtraits/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// idealtraitsCommaIDs are the board's postings in the listing's order.
var idealtraitsCommaIDs = []string{"369444", "367541", "367535", "367487", "354262"}

func idealtraitsCommaFake(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, id := range idealtraitsCommaIDs {
		r.route("/career/Comma-Insurance/"+id+"CPG", idealtraitsFixture(t, "job_"+id+".html"))
	}
	return r.route("MTE2MjI5Mg==/careers?page=1", idealtraitsFixture(t, "listing_1162292.json"))
}

func idealtraitsComma() CompanyEntry {
	return CompanyEntry{Provider: "idealtraits", Board: "1162292", Company: "Comma Insurance"}
}

func TestIdealTraitsProvider(t *testing.T) {
	if got := NewIdealTraits(nil).Provider(); got != "idealtraits" {
		t.Errorf("Provider() = %q, want idealtraits", got)
	}
}

func TestIdealTraitsListURL(t *testing.T) {
	got, err := idealtraitsListURL("1162292", 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://app.idealtraits.com/company/MTE2MjI5Mg==/careers?page=2"; got != want {
		t.Errorf("idealtraitsListURL = %q, want %q", got, want)
	}
	for _, bad := range []string{"", "MTE2MjI5Mg==", "Comma-Insurance", "0", "12a"} {
		if _, err := idealtraitsListURL(bad, 1); err == nil {
			t.Errorf("idealtraitsListURL(%q) accepted a non-numeric board", bad)
		}
	}
}

func TestIdealTraitsFetch(t *testing.T) {
	fake := idealtraitsCommaFake(t)
	jobs, err := NewIdealTraits(fake).Fetch(context.Background(), idealtraitsComma())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != len(idealtraitsCommaIDs) {
		t.Fatalf("got %d jobs, want %d", len(jobs), len(idealtraitsCommaIDs))
	}
	// One listing request (jobcount reached on page 1) plus one page per posting.
	if fake.calls != 1+len(idealtraitsCommaIDs) {
		t.Errorf("made %d requests, want %d", fake.calls, 1+len(idealtraitsCommaIDs))
	}
	byID := map[string]Job{}
	for i, j := range jobs {
		if j.ExternalID != idealtraitsCommaIDs[i] {
			t.Errorf("job %d id = %q, want %q", i, j.ExternalID, idealtraitsCommaIDs[i])
		}
		byID[j.ExternalID] = j
	}

	j := byID["367535"]
	if j.Title != "Personal Lines Insurance Advisor" || j.Company != "Comma Insurance" {
		t.Errorf("title/company = %q/%q", j.Title, j.Company)
	}
	if j.URL != "https://app.idealtraits.com/career/Comma-Insurance/367535CPG" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Nashville, TN, USA" || len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("location/countries = %q/%v", j.Location, j.Countries)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("work mode = %q remote=%v, want remote (the page states Fully Remote)", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format("2006-01-02T15:04:05") != "2026-09-03T07:08:56" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 55000 || j.SalaryMax == nil || *j.SalaryMax != 120000 ||
		j.SalaryCurrency != "USD" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if strings.Contains(j.Description, "###") {
		t.Errorf("description still carries the platform's ### joins")
	}
	if !strings.Contains(j.Description, "<li>Paid Time Off (PTO)</li>") || !strings.Contains(j.Description, "Comma team") {
		t.Errorf("description lost its body or its benefit list: %.300s", j.Description)
	}

	if got := byID["369444"].WorkMode; got != "hybrid" {
		t.Errorf("369444 work mode = %q, want hybrid (the page states Hybrid Remote)", got)
	}
}

func TestIdealTraitsOnsitePosting(t *testing.T) {
	root, err := html.Parse(strings.NewReader(idealtraitsFixture(t, "job_370499.html")))
	if err != nil {
		t.Fatal(err)
	}
	p := idealtraitsPosting{id: "370499", url: "https://app.idealtraits.com/career/AAA-The-Auto-Club-Group-David-Bordes-Agency/370499CPG"}
	j, ok := parseIdealTraitsPosting(root, p, "AAA")
	if !ok {
		t.Fatal("posting did not parse")
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("work mode = %q remote=%v, want onsite (the page states No)", j.WorkMode, j.Remote)
	}
	if j.Title != "Insurance Sales Representative" || j.Location != "Novi, MI, USA" {
		t.Errorf("title/location = %q/%q", j.Title, j.Location)
	}
}

func TestIdealTraitsEmptyBoard(t *testing.T) {
	if _, ok := NewIdealTraits(nil).(emptyBoardListing); !ok {
		t.Fatal("explicit jobcount zero must support empty-board closure")
	}
	fake := (&routedHTTP{}).route("ODczOQ==/careers?page=1", idealtraitsFixture(t, "listing_8739_empty.json"))
	jobs, err := NewIdealTraits(fake).Fetch(context.Background(), CompanyEntry{Board: "8739", Company: "Cragin & Pike"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs from a board with no openings", len(jobs))
	}
}

func TestIdealTraitsRejectsUnprovenEmptyListing(t *testing.T) {
	for _, body := range []string{`{}`, `{"joblist":""}`, `{"jobcount":0}`, `{"joblist":null,"jobcount":0}`, `{"joblist":"","jobcount":null}`, `{"joblist":"","jobcount":-1}`, `{"joblist":"<p>Loading</p>","jobcount":0}`} {
		fake := (&routedHTTP{}).route("careers?page=1", body)
		if _, err := NewIdealTraits(fake).Fetch(context.Background(), CompanyEntry{Board: "8739"}); err == nil {
			t.Errorf("unproven empty listing accepted: %s", body)
		}
	}
}

func TestIdealTraitsLaterListingFailureFailsBoard(t *testing.T) {
	listing := strings.Replace(idealtraitsFixture(t, "listing_1162292.json"), `"jobcount":5`, `"jobcount":6`, 1)
	fake := (&routedHTTP{}).route("careers?page=1", listing).routeErr("careers?page=2", errors.New("connection reset"))
	if _, err := NewIdealTraits(fake).Fetch(context.Background(), idealtraitsComma()); err == nil {
		t.Fatal("later page failure accepted a partial listing")
	}
}

// A listing that runs out of cards before the board's own jobcount is a truncated walk, and on a
// fullBoardListing provider it must fail rather than let the sweep close what it did not reach.
// The page-1 body is the real Comma listing with its jobcount raised by one; page 2 is the empty
// joblist the platform answers past a board's end.
func TestIdealTraitsShortListingFails(t *testing.T) {
	listing := strings.Replace(idealtraitsFixture(t, "listing_1162292.json"), `"jobcount":5`, `"jobcount":6`, 1)
	if !strings.Contains(listing, `"jobcount":6`) {
		t.Fatal("fixture no longer carries jobcount 5")
	}
	fake := (&routedHTTP{}).
		route("careers?page=1", listing).
		route("careers?page=2", `{"joblist":"","recordcount":"","jobcount":6}`)
	if _, err := NewIdealTraits(fake).Fetch(context.Background(), idealtraitsComma()); err == nil {
		t.Fatal("a listing short of its jobcount was accepted")
	}
}

func TestIdealTraitsFetchNewHydratesOnlyUnseen(t *testing.T) {
	fake := idealtraitsCommaFake(t)
	seen := func(id string) bool { return id != "367535" }
	jobs, err := idealtraits{http: fake}.FetchNew(context.Background(), idealtraitsComma(), seen)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 {
		t.Errorf("made %d requests, want 2 (the listing and the one unseen posting)", fake.calls)
	}
	for _, j := range jobs {
		if j.ExternalID == "367535" {
			if j.SeenRefresh || j.Description == "" {
				t.Errorf("unseen posting was not hydrated")
			}
			continue
		}
		if !j.SeenRefresh || j.Title == "" || j.Description != "" {
			t.Errorf("seen posting %s = %+v, want a titled liveness refresh", j.ExternalID, j)
		}
	}
	if got := jobs[0].Title; got != "Personal Lines Account Manager" {
		t.Errorf("refresh title = %q, want the card's", got)
	}
}

func TestIdealTraitsDetailFailures(t *testing.T) {
	fake := idealtraitsCommaFake(t).
		routeErr("369444CPG", &StatusError{Method: http.MethodGet, Code: http.StatusGone}).
		routeErr("367541CPG", &StatusError{Method: http.MethodGet, Code: http.StatusInternalServerError}).
		routeErr("367535CPG", errors.New("timeout"))
	jobs, err := NewIdealTraits(fake).Fetch(context.Background(), idealtraitsComma())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ExternalID] = j.Unreadable
	}
	if _, ok := got["369444"]; ok {
		t.Error("a posting the platform answered 410 for was yielded")
	}
	if !got["367541"] || !got["367535"] {
		t.Errorf("an unread posting was not marked unreadable: %v", got)
	}
	if got["367487"] || got["354262"] {
		t.Errorf("a read posting was marked unreadable: %v", got)
	}
}

func TestIdealTraitsBenefitLists(t *testing.T) {
	in := `<h4>Benefits</h4><br />Paid Time Off (PTO)###Work from Home### ###<br /><p>Pay is 5 # per hour</p>`
	want := `<h4>Benefits</h4><br /><ul><li>Paid Time Off (PTO)</li><li>Work from Home</li></ul><br /><p>Pay is 5 # per hour</p>`
	if got := idealtraitsBenefitLists(in); got != want {
		t.Errorf("idealtraitsBenefitLists =\n%s\nwant\n%s", got, want)
	}
}

func TestRateLimitedIdealTraitsGetterGatesBothKinds(t *testing.T) {
	waiter := &recordingWaiter{}
	g := rateLimitedIdealTraitsGetter{inner: idealtraitsCommaFake(t), limiter: waiter}
	var page idealtraitsPage
	if err := g.GetJSONWithHeaders(context.Background(), "https://app.idealtraits.com/company/MTE2MjI5Mg==/careers?page=1", idealtraitsXHR, &page); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GetHTML(context.Background(), "https://app.idealtraits.com/career/Comma-Insurance/367535CPG"); err != nil {
		t.Fatal(err)
	}
	if waiter.calls != 2 {
		t.Errorf("limiter.Wait called %d times, want 2 (one per request kind)", waiter.calls)
	}
}
