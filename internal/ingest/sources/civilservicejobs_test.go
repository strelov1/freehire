package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

func csjFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/civilservicejobs/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// csjFake serves the site's flow from fixtures: the home page answers with the proof-of-work
// interstitial until the answer is posted, then the search POST and its "next" link serve the
// two result pages, and every jobs.cgi request the posting page.
type csjFake struct {
	t        *testing.T
	pages    map[string]string // GET url -> raw body
	search   string            // body of the search POST
	mu       sync.Mutex
	answered bool
	posts    []url.Values
	gets     []string
}

func (f *csjFake) GetText(_ context.Context, u string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, u)
	if u == csjBaseURL+"/csr/index.cgi" && !f.answered {
		return csjFixture(f.t, "challenge.html"), nil
	}
	if body, ok := f.pages[u]; ok {
		return body, nil
	}
	if strings.Contains(u, "/csr/jobs.cgi?jcode=") {
		return csjFixture(f.t, "detail.html"), nil
	}
	return "", &StatusError{Code: 404}
}

func (f *csjFake) PostFormText(_ context.Context, u string, v url.Values) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, v)
	switch {
	case v.Get("altcha") != "":
		f.answered = true
		return csjFixture(f.t, "home.html"), nil
	case strings.Contains(u, "esearch.cgi"):
		return f.search, nil
	}
	return "", &StatusError{Code: 400}
}

func newCSJFake(t *testing.T) *csjFake {
	return &csjFake{
		t: t,
		pages: map[string]string{
			csjBaseURL + "/csr/index.cgi?ProtectCaptcha=1":                       csjFixture(t, "challenge.json"),
			"https://www.civilservicejobs.service.gov.uk/csr/index.cgi?SID=c2lk": csjFixture(t, "listing_last.html"),
		},
		search: csjFixture(t, "listing.html"),
	}
}

func newTestCSJ(h csjHTTP) civilServiceJobs {
	s := NewCivilServiceJobs(h).(civilServiceJobs)
	s.limiter = noWait{}
	return s
}

type noWait struct{}

func (noWait) Wait(context.Context) error { return nil }

func TestSolveAltchaAnswersARealChallenge(t *testing.T) {
	var ch altchaChallenge
	if err := json.Unmarshal([]byte(csjFixture(t, "challenge.json")), &ch); err != nil {
		t.Fatal(err)
	}
	got, err := solveAltcha(ch)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Number    int    `json:"number"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	// The number the site's own widget found for this challenge, captured with it.
	if payload.Number != 210427 || payload.Signature != ch.Signature {
		t.Errorf("payload = %+v, want number 210427 and the challenge's signature", payload)
	}
}

func TestSolveAltchaRefusesAnUnsolvableChallenge(t *testing.T) {
	ch := altchaChallenge{Algorithm: "SHA-256", Challenge: strings.Repeat("00", 32), MaxNumber: 10, Salt: "s"}
	if _, err := solveAltcha(ch); err == nil {
		t.Fatal("want an error when no number in range matches")
	}
	if _, err := solveAltcha(altchaChallenge{Algorithm: "MD5"}); err == nil {
		t.Fatal("want an error for an unknown algorithm")
	}
}

func TestCSJFetchPassesTheCheckAndReadsTheWholeCatalogue(t *testing.T) {
	f := newCSJFake(t)
	jobs, err := newTestCSJ(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	// The listing fixtures hold 3 + 2 cards and report a total of 5.
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	if got := f.posts[0].Get("originalRequestToken"); got != "test-request-token" {
		t.Errorf("answer carried token %q, want the interstitial's", got)
	}
	search := f.posts[1]
	if search.Get("what") != "" || search.Get("where") != "" || search.Get("overseas") != "1" || search.Get("SID") == "" {
		t.Errorf("search form = %v, want empty what/where and the form's hidden fields", search)
	}
	for _, j := range jobs {
		if j.ExternalID == "" || !strings.HasPrefix(j.URL, csjBaseURL+"/csr/jobs.cgi?jcode="+j.ExternalID) || j.Company == "" {
			t.Errorf("job without identity: %+v", j)
		}
	}
}

func TestCSJFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	f := newCSJFake(t)
	cards := csjFixtureCards(t, "listing.html")
	seen := map[string]bool{cards[0].id: true}
	jobs, err := newTestCSJ(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatal(err)
	}
	var refreshed, details int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.ExternalID != cards[0].id || j.Title != cards[0].title {
				t.Errorf("refresh = %+v, want card %+v", j, cards[0])
			}
		}
	}
	for _, u := range f.gets {
		if strings.Contains(u, "jobs.cgi") {
			details++
		}
	}
	if refreshed != 1 || details != 4 {
		t.Errorf("refreshed %d, fetched %d details; want 1 and 4", refreshed, details)
	}
}

func TestCSJFetchNewBoundsHydrationPerRun(t *testing.T) {
	s := newTestCSJ(newCSJFake(t))
	s.maxNewPerRun = 2
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("got %d jobs, want the 2 the budget allows", len(jobs))
	}
}

func TestCSJFailsAWalkShorterThanTheReportedTotal(t *testing.T) {
	f := newCSJFake(t)
	f.search = strings.Replace(f.search, "5 Search results", "6 Search results", 1)
	if _, err := newTestCSJ(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when the walk reads fewer postings than the site reports")
	}
}

func TestCSJFailsWhenALaterPageFails(t *testing.T) {
	f := newCSJFake(t)
	delete(f.pages, "https://www.civilservicejobs.service.gov.uk/csr/index.cgi?SID=c2lk")
	if _, err := newTestCSJ(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error: a fullCatalog walk must not end early on a failed page")
	}
}

func TestCSJFailsWhenTheAnswerIsRefused(t *testing.T) {
	f := &csjRefusingFake{newCSJFake(t)}
	_, err := newTestCSJ(f).Fetch(context.Background(), CompanyEntry{})
	if !errors.Is(err, errCSJChallenge) {
		t.Fatalf("err = %v, want errCSJChallenge", err)
	}
}

// csjRefusingFake answers the proof-of-work POST with the interstitial again.
type csjRefusingFake struct{ *csjFake }

func (f *csjRefusingFake) PostFormText(context.Context, string, url.Values) (string, error) {
	return csjFixture(f.t, "challenge.html"), nil
}

func csjFixtureCards(t *testing.T, name string) []csjCard {
	t.Helper()
	page, err := csjDecode(csjFixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	return csjCards(root)
}

func TestCSJCardsReadsEveryField(t *testing.T) {
	cards := csjFixtureCards(t, "listing.html")
	if len(cards) != 3 {
		t.Fatalf("got %d cards, want 3", len(cards))
	}
	c := cards[0]
	if c.id == "" || c.title == "" || c.department == "" || c.location == "" || !strings.HasPrefix(c.salary, "£") {
		t.Errorf("card = %+v, want every field read", c)
	}
	for _, c := range cards {
		if strings.Contains(c.location, "see the job advert") || strings.Contains(c.salary, "Salary") {
			t.Errorf("card %s kept a label or the truncation tail: %+v", c.id, c)
		}
	}
}

func TestCSJDetailReadsThePosting(t *testing.T) {
	page, err := csjDecode(csjFixture(t, "detail.html"))
	if err != nil {
		t.Fatal(err)
	}
	card := csjCard{id: "2017844", department: "Cabinet Office", location: "Bristol, Glasgow", salary: "£59,207 to £70,958"}
	j, ok := csjDetail(page, card)
	if !ok {
		t.Fatal("detail not read")
	}
	if j.Title != "Senior Software Developer" || j.Company != "Cabinet Office" || j.Location != "Bristol, Glasgow" {
		t.Errorf("identity = %q / %q / %q", j.Title, j.Company, j.Location)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 59207 || *j.SalaryMax != 70958 || j.SalaryCurrency != "GBP" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want none for a role offered full- and part-time", j.EmploymentType)
	}
	for _, want := range []string{"Job grade: Grade 7", "Contract type: Permanent", "Technical Live Services", "£17,152"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description lacks %q", want)
		}
	}
	if strings.Contains(j.Description, "Skip to section") || strings.ContainsRune(j.Description, '�') {
		t.Error("description kept the table of contents or a mis-decoded character")
	}
}

func TestCSJDetailRefusesTheSignInPage(t *testing.T) {
	page, err := csjDecode(csjFixture(t, "detail_missing.html"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := csjDetail(page, csjCard{id: "1", department: "X"}); ok {
		t.Error("a closed posting's sign-in page was read as a posting")
	}
}

func TestCSJSalary(t *testing.T) {
	for _, tc := range []struct {
		in     string
		lo, hi int
		ok     bool
	}{
		{"£59,207 to £70,958", 59207, 70958, true},
		{"£34,000", 34000, 34000, true},
		{"£475", 0, 0, false},
		{"£1,000 to £30,000", 0, 0, false},
		{"", 0, 0, false},
	} {
		lo, hi, ok := csjSalary(tc.in)
		if lo != tc.lo || hi != tc.hi || ok != tc.ok {
			t.Errorf("csjSalary(%q) = %d, %d, %v; want %d, %d, %v", tc.in, lo, hi, ok, tc.lo, tc.hi, tc.ok)
		}
	}
}

func TestCSJEmploymentType(t *testing.T) {
	for in, want := range map[string]string{
		"Full-time":                   "full_time",
		"Flexible working, Full-time": "full_time",
		"Part-time, Job share":        "part_time",
		"Full-time, Part-time":        "",
		"Flexible working, Full-time, Job share, Part-time": "",
		"": "",
	} {
		if got := csjEmploymentType(in); got != want {
			t.Errorf("csjEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCSJIsAFullCatalogAggregator(t *testing.T) {
	s := NewCivilServiceJobs(nil)
	if _, ok := s.(fullCatalog); !ok {
		t.Error("not fullCatalog")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("not an aggregator")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("not hydrating")
	}
}

func TestCSJFieldValueSeparatesLines(t *testing.T) {
	root, err := html.Parse(strings.NewReader(`<div class="vac_display_field_value">Permanent<br>Loan<BR> Secondment </div>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := csjFieldValue(firstByClass(root, "vac_display_field_value")); got != "Permanent, Loan, Secondment" {
		t.Errorf("csjFieldValue = %q", got)
	}
}
