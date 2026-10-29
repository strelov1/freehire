package sources

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// pracujFake serves the real fixtures (fetched 2026-10-29, trimmed to the fields read): the IT
// slice's two listing pages (three postings, one offered in 16 cities), and three offer pages —
// an IT posting paid on a B2B contract, a call-centre posting paid by the hour on two contracts,
// and an archived one. Every other slice lists the second page's posting again, as a posting
// filed under categories of two slices is. Offer pages are mapped to the listed postings newest
// first: RGB (2026-10-28) → IT, WNG (2026-10-17) → hourly, OTTO (2026-10-08) → archived.
type pracujFake struct {
	// noResultsFirst answers the first request of every URL with a page rendered without its
	// results, as the search's server render sometimes does; the retry must clear it.
	noResultsFirst bool
	// failListing fails every request for the IT slice's second page.
	failListing bool
	// failOffers fails every offer page.
	failOffers bool

	mu     sync.Mutex
	asked  map[string]int
	offers []string
}

var pracujFakeOffers = map[string]string{
	"https://www.pracuj.pl/praca/manager-ds-inwestycji-dzial-drzwi-bialystok,oferta,1005187251":   "offer_it.html",
	"https://www.pracuj.pl/praca/pracownik-ogolnobudowlany-k-m-niemcy,oferta,12325357":            "offer_salary.html",
	"https://www.pracuj.pl/praca/pracownik-produkcji-k-m-szlifowanie-zawiercie,oferta,1005144102": "offer_archived.html",
}

func (f *pracujFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.mu.Lock()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[u]++
	first := f.asked[u] == 1
	isOffer := strings.Contains(u, ",oferta,")
	if isOffer {
		f.offers = append(f.offers, u)
	}
	f.mu.Unlock()
	var file string
	switch {
	case f.noResultsFirst && first:
		file = "listing_no_results.html"
	case isOffer:
		if f.failOffers {
			return nil, &StatusError{Method: "GET", Code: 500, URL: u}
		}
		file = pracujFakeOffers[u]
		if file == "" {
			return nil, &StatusError{Method: "GET", Code: 404, URL: u}
		}
	case strings.Contains(u, "cc=5015,5016&") && strings.HasSuffix(u, "&pn=1"):
		file = "listing_it_1.html"
	case strings.Contains(u, "cc=5015,5016&") && strings.HasSuffix(u, "&pn=2"):
		if f.failListing {
			return nil, &StatusError{Method: "GET", Code: 503, URL: u}
		}
		file = "listing_it_2.html"
	default:
		file = "listing_it_2.html"
	}
	raw, err := os.ReadFile("testdata/pracuj/" + file)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func (f *pracujFake) offerRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.offers...)
}

func newTestPracuj(f *pracujFake) pracuj {
	return pracuj{
		http:    f,
		limiter: rate.NewLimiter(rate.Inf, 1),
		slices: []pracujSlice{
			{categories: "5015,5016", tech: true},
			{categories: "5022"},
		},
		pageSize:     2,
		maxNewPerRun: 10,
		maxPages:     10,
		retryBackoff: time.Millisecond,
	}
}

func pracujByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestPracujFetchNewHydratesNewPostings(t *testing.T) {
	f := &pracujFake{}
	jobs, err := newTestPracuj(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// The archived posting is the site's answer that it is gone: not yielded.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	byID := pracujByID(jobs)

	it, ok := byID["70220000-56be-0050-3009-08df2855eb64"]
	if !ok {
		t.Fatalf("IT posting missing: %v", byID)
	}
	if it.Company != "ITDS Polska Sp. z o.o." || it.Title != "Senior Business Analyst" {
		t.Errorf("identity = %q / %q", it.Company, it.Title)
	}
	if it.URL != "https://www.pracuj.pl/praca/manager-ds-inwestycji-dzial-drzwi-bialystok,oferta,1005187251" {
		t.Errorf("URL = %q", it.URL)
	}
	if !it.IsTechHint || it.WorkMode != "remote" || !it.Remote || it.Seniority != "senior" || it.EmploymentType != "full_time" {
		t.Errorf("facets: tech=%v mode=%q remote=%v seniority=%q type=%q", it.IsTechHint, it.WorkMode, it.Remote, it.Seniority, it.EmploymentType)
	}
	if it.Location != "Warszawa" || len(it.Countries) != 1 || it.Countries[0] != "pl" || len(it.FallbackCountries) != 1 || it.FallbackCountries[0] != "pl" {
		t.Errorf("place = %q %v %v", it.Location, it.Countries, it.FallbackCountries)
	}
	// Only a B2B contract is stated, so its net figure is the salary.
	if it.SalaryMin == nil || *it.SalaryMin != 18900 || *it.SalaryMax != 23100 || it.SalaryCurrency != "PLN" || it.SalaryPeriod != "month" {
		t.Errorf("salary = %v %v %q %q", it.SalaryMin, it.SalaryMax, it.SalaryCurrency, it.SalaryPeriod)
	}
	for _, want := range []string{"18900–23100 PLN netto (+ VAT) / miesięcznie (kontrakt B2B)", "<li>", "<h2>"} {
		if !strings.Contains(it.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, it.Description)
		}
	}
	if it.PostedAt == nil {
		t.Error("PostedAt not set")
	}

	hourly := byID["bc410000-56be-0050-0868-08df2ca356f7"]
	// Two contracts state pay; the gross one (umowa zlecenie) wins over B2B's net one.
	if hourly.SalaryMin == nil || *hourly.SalaryMin != 36 || *hourly.SalaryMax != 48 || hourly.SalaryPeriod != "hour" {
		t.Errorf("hourly salary = %v %v %q", hourly.SalaryMin, hourly.SalaryMax, hourly.SalaryPeriod)
	}
	// Junior AND mid: two levels name no single seniority. Listed by the IT slice, the posting
	// keeps that signal although its page's leading category is not IT.
	if hourly.Seniority != "" || hourly.EmploymentType != "part_time" || !hourly.IsTechHint {
		t.Errorf("hourly facets: seniority=%q type=%q tech=%v", hourly.Seniority, hourly.EmploymentType, hourly.IsTechHint)
	}
	if hourly.WorkMode != "remote" {
		t.Errorf("hourly work mode = %q, want remote (entirely remote work)", hourly.WorkMode)
	}
}

func TestPracujRefreshesSeenPostingsWithoutBuyingThem(t *testing.T) {
	f := &pracujFake{}
	jobs, err := newTestPracuj(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Description != "" {
			t.Errorf("%s: SeenRefresh=%v description=%q", j.ExternalID, j.SeenRefresh, j.Description)
		}
	}
	if got := f.offerRequests(); len(got) != 0 {
		t.Errorf("bought %d offer pages for seen postings", len(got))
	}
}

func TestPracujGateSkipsCoveredEmployers(t *testing.T) {
	f := &pracujFake{}
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		return map[string]bool{"RGB Drożdżalski sp.j.": true}
	}}
	jobs, err := newTestPracuj(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range f.offerRequests() {
		if strings.Contains(u, "1005187251") {
			t.Errorf("bought the covered employer's page %s", u)
		}
	}
	covered := pracujByID(jobs)["70220000-56be-0050-3009-08df2855eb64"]
	if covered.Description != "" || covered.Company != "RGB Drożdżalski sp.j." {
		t.Errorf("covered posting = %+v, want the body-less listing job", covered)
	}
	// Listed by the IT slice: the base job carries the slice's signal and the listing's mode.
	if !covered.IsTechHint {
		t.Error("covered IT-slice posting lost its tech hint")
	}
}

func TestPracujBoundsNewPostingsNewestFirst(t *testing.T) {
	f := &pracujFake{}
	s := newTestPracuj(f)
	s.maxNewPerRun = 1
	if _, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	got := f.offerRequests()
	if len(got) != 1 || !strings.Contains(got[0], "1005187251") {
		t.Errorf("offer requests = %v, want only the newest posting's", got)
	}
}

func TestPracujDeduplicatesAcrossSlicesKeepingTheTechSignal(t *testing.T) {
	f := &pracujFake{}
	postings, err := newTestPracuj(f).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3", len(postings))
	}
	for _, p := range postings {
		if !p.tech {
			t.Errorf("%s lost the IT slice's signal after the second slice listed it", p.GroupID)
		}
	}
	if postings[0].GroupID != "70220000-56be-0050-3009-08df2855eb64" {
		t.Errorf("first posting = %s, want the newest", postings[0].GroupID)
	}
}

func TestPracujRetriesAPageRenderedWithoutResults(t *testing.T) {
	f := &pracujFake{noResultsFirst: true}
	jobs, err := newTestPracuj(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("got %d jobs, want 2", len(jobs))
	}
}

func TestPracujFailsTheCrawlOnAFailedListingPage(t *testing.T) {
	f := &pracujFake{failListing: true}
	if _, err := newTestPracuj(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a listing page that never answered must fail the crawl")
	}
}

func TestPracujFailsWhenNoOfferPageAnswers(t *testing.T) {
	f := &pracujFake{failOffers: true}
	if _, err := newTestPracuj(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("a crawl that read no offer page must fail")
	}
}

func TestPracujWorkMode(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"Praca stacjonarna"}, "onsite"},
		{[]string{"Praca hybrydowa (do uzgodnienia)", "Praca stacjonarna"}, "hybrid"},
		{[]string{"Praca hybrydowa", "Praca zdalna"}, "remote"},
		{[]string{"full-office", "hybrid"}, "hybrid"},
		{[]string{"home-office"}, "remote"},
		{[]string{"Praca mobilna"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := pracujWorkMode(c.in); got != c.want {
			t.Errorf("pracujWorkMode(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPracujSalaryPrefersTheEmploymentContractsGrossPay(t *testing.T) {
	pay := func(id int, kind string, from, to float64, unit int, currency string) pracujContract {
		c := pracujContract{ID: id}
		c.Salary = &struct {
			From     *float64 `json:"from"`
			To       *float64 `json:"to"`
			Currency struct {
				Code string `json:"code"`
			} `json:"currency"`
			TimeUnit struct {
				ID       int `json:"id"`
				LongForm struct {
					Name string `json:"name"`
				} `json:"longForm"`
			} `json:"timeUnit"`
			Kind struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"salaryKind"`
		}{From: &from, To: &to}
		c.Salary.Currency.Code = currency
		c.Salary.TimeUnit.ID = unit
		c.Salary.Kind.Code = kind
		return c
	}
	contracts := []pracujContract{
		pay(3, "net-plus-vat", 20000, 25000, 0, "PLN"),
		pay(2, "gross", 9000, 17000, 0, "PLN"),
		pay(0, "gross", 9000, 15000, 0, "PLN"),
	}
	lo, hi, period, ok := pracujSalary(contracts)
	if !ok || lo != 9000 || hi != 15000 || period != "month" {
		t.Errorf("salary = %d-%d %q %v, want the contract of employment's 9000-15000 month", lo, hi, period, ok)
	}
	if _, _, _, ok := pracujSalary([]pracujContract{pay(0, "gross", 1000, 2000, 0, "EUR")}); ok {
		t.Error("a salary in EUR must not be stored")
	}
	if _, _, _, ok := pracujSalary([]pracujContract{pay(0, "gross", 1000, 2000, 7, "PLN")}); ok {
		t.Error("a salary with an unknown period must not be stored")
	}
	if _, _, _, ok := pracujSalary([]pracujContract{{ID: 0}}); ok {
		t.Error("a contract without pay must not yield a salary")
	}
}

func TestPracujSeniority(t *testing.T) {
	if got := pracujSeniority([]int{18}); got != "senior" {
		t.Errorf("senior = %q", got)
	}
	if got := pracujSeniority([]int{17, 4}); got != "" {
		t.Errorf("junior+mid = %q, want none", got)
	}
	if got := pracujSeniority([]int{20}); got != "" {
		t.Errorf("manager = %q, want none", got)
	}
}
