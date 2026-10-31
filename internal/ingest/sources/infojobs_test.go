package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ijPage wraps an inner __INITIAL_PROPS__ JSON document in the exact double-encoding the real
// pages use: JSON.parse("<the document as a JS string literal>"). json.Marshal of the document
// string produces that literal (quoted, with the inner quotes escaped), so the adapter's decoder
// is exercised end-to-end.
func ijPage(innerJSON string) string {
	lit, err := json.Marshal(innerJSON)
	if err != nil {
		panic(err)
	}
	return `<html><head><script>window.__INITIAL_PROPS__ = JSON.parse(` + string(lit) + `);</script></head><body></body></html>`
}

// infojobsHTTP is a test TextGetter serving the whole-site listing's first page (root), the
// category listings by categoryIds and ?page=N, and offer pages by full URL; anything else fails.
// A category page it holds no body for answers what the real site serves past a listing's end:
// the category's props with no offers.
type infojobsHTTP struct {
	root     string
	catPages map[string]map[int]string
	details  map[string]string
	failList bool
	delay    time.Duration
	// flaky is how many times each listing page ("<category>:<page>", "" for the root) fails
	// before it is served.
	flaky map[string]int
	// refusedDetails is how many times each offer page answers 405 before it is served.
	refusedDetails map[string]int

	mu       sync.Mutex
	got      []string
	inFlight int
	peak     int
}

func (f *infojobsHTTP) GetText(_ context.Context, rawURL string) (string, error) {
	f.mu.Lock()
	f.got = append(f.got, rawURL)
	f.inFlight++
	if f.inFlight > f.peak {
		f.peak = f.inFlight
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	time.Sleep(f.delay)
	if !strings.HasPrefix(rawURL, "https://www.infojobs.net/ofertas-trabajo?") {
		f.mu.Lock()
		refused := f.refusedDetails[rawURL] > 0
		if refused {
			f.refusedDetails[rawURL]--
		}
		f.mu.Unlock()
		if refused {
			return "", &StatusError{Method: "GET", Code: 405, URL: rawURL}
		}
		if body, ok := f.details[rawURL]; ok {
			return body, nil
		}
		return "", errors.New("infojobsHTTP: unexpected request " + rawURL)
	}
	if f.failList {
		return "", errors.New("infojobsHTTP: list boom")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	cat, page := u.Query().Get("categoryIds"), u.Query().Get("page")
	key := cat + ":" + page
	f.mu.Lock()
	failing := f.flaky[key] > 0
	if failing {
		f.flaky[key]--
	}
	f.mu.Unlock()
	if failing {
		return "", errors.New("infojobsHTTP: browser fetch timed out")
	}
	if cat == "" {
		if page != "1" {
			return "", errors.New("infojobsHTTP: the whole-site listing past page 1 was requested: " + rawURL)
		}
		return f.root, nil
	}
	n, _ := strconv.Atoi(page)
	if body, ok := f.catPages[cat][n]; ok {
		return body, nil
	}
	return ijPage(`{"offers":[],"search":{"categoryIds":["` + cat + `"]}}`), nil
}

func (f *infojobsHTTP) listRequests() int {
	n := 0
	for _, u := range f.got {
		if strings.HasPrefix(u, "https://www.infojobs.net/ofertas-trabajo?") {
			n++
		}
	}
	return n
}

// ijCat is one category of a fake site: its id and its listing's pages of offers (raw JSON
// objects, see ijOffer). The category states as many pages as it is given.
type ijCat struct {
	id    string
	pages [][]string
}

// ijSite builds a fake site whose root page states every category with the number of offers it
// holds, and the whole site's total as their sum.
func ijSite(cats ...ijCat) *infojobsHTTP {
	f := &infojobsHTTP{catPages: map[string]map[int]string{}}
	var agg []string
	total := 0
	for _, c := range cats {
		count := 0
		for _, p := range c.pages {
			count += len(p)
		}
		total += count
		agg = append(agg, `{"value":"`+c.id+`","count":`+strconv.Itoa(count)+`}`)
		f.catPages[c.id] = map[int]string{}
		for i, offers := range c.pages {
			f.catPages[c.id][i+1] = ijCategoryPage(c.id, len(c.pages), offers...)
		}
	}
	f.root = ijRootPage(total, agg...)
	return f
}

func ijRootPage(total int, aggCategories ...string) string {
	return ijPage(`{"overview":{"totalElements":` + strconv.Itoa(total) + `},"aggregation":{"category":[` +
		strings.Join(aggCategories, ",") + `]},"offers":[` + ijOffer("PROMO", "") + `]}`)
}

func ijCategoryPage(cat string, totalPages int, offers ...string) string {
	return ijPage(`{"navigation":{"totalPages":` + strconv.Itoa(totalPages) + `},"search":{"categoryIds":["` + cat +
		`"]},"offers":[` + strings.Join(offers, ",") + `]}`)
}

// ijOffer is one listing offer whose link is https://www.infojobs.net/x/of-i<code>.
func ijOffer(code, publishedAt string) string {
	return `{"code":"` + code + `","title":"T","companyName":"Co","publishedAt":"` + publishedAt +
		`","link":"//www.infojobs.net/x/of-i` + code + `"}`
}

// ijOffers is ijOffer for each code, with no publication date.
func ijOffers(codes ...string) []string {
	var out []string
	for _, c := range codes {
		out = append(out, ijOffer(c, ""))
	}
	return out
}

func ijCodes(offers []infojobsOffer) []string {
	var codes []string
	for _, o := range offers {
		codes = append(codes, o.Code)
	}
	sort.Strings(codes)
	return codes
}

func TestInfojobsFetchNewRefreshesSeenAndHydratesNew(t *testing.T) {
	// Offer B's body carries an embedded quote and a `");` sequence to exercise the decoder.
	http := ijSite(ijCat{id: "190", pages: [][]string{{
		`{"code":"AAA","title":"Backend Engineer","companyName":"Acme","city":"Madrid",
		  "publishedAt":"2026-07-12T08:00:00Z","teleworking":"Presencial","link":"//www.infojobs.net/madrid/backend/of-iAAA?applicationOrigin=search"}`,
		`{"code":"BBB","title":"Data Scientist","companyName":"Beta","city":"Barcelona","description":"Teaser.",
		  "publishedAt":"2026-07-11T09:00:00Z","teleworking":"Solo teletrabajo","link":"//www.infojobs.net/barcelona/data/of-iBBB?applicationOrigin=search"}`,
	}}})
	http.details = map[string]string{
		"https://www.infojobs.net/barcelona/data/of-iBBB": ijPage(`{"offer":{"description":"Full body. He said \"hi\"); and more text."}}`),
	}

	// AAA already ingested → seen, no detail request; BBB new → hydrated.
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "AAA" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	aaa, bbb := byID["AAA"], byID["BBB"]
	if !aaa.SeenRefresh || aaa.URL != "https://www.infojobs.net/madrid/backend/of-iAAA" || aaa.WorkMode != "onsite" {
		t.Errorf("AAA = %+v, want a liveness refresh with the cleaned URL and onsite", aaa)
	}
	if bbb.SeenRefresh || !strings.Contains(bbb.Description, "Full body") || !strings.Contains(bbb.Description, "more text") {
		t.Errorf("BBB = %+v, want the hydrated body", bbb)
	}
	if bbb.WorkMode != "remote" || !bbb.Remote || bbb.PostedAt == nil {
		t.Errorf("BBB fields = %+v, want remote and a posted date", bbb)
	}
	// The Spanish board's bare city ("Murcia") states no country; Spain is the fallback.
	if len(bbb.FallbackCountries) != 1 || bbb.FallbackCountries[0] != "es" || bbb.Countries != nil {
		t.Errorf("BBB countries = %v / fallback %v, want fallback [es]", bbb.Countries, bbb.FallbackCountries)
	}
	for _, u := range http.got {
		if strings.Contains(u, "of-iAAA") {
			t.Error("a seen offer must not cost a detail request")
		}
	}
}

// A new offer whose page fails is deferred, not stored with its teaser: a stored row is seen
// and would never get its body.
func TestInfojobsDetailFailureDefersTheOffer(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}})
	http.details = map[string]string{
		"https://www.infojobs.net/x/of-iA": ijPage(`{"offer":{"description":"Body A."}}`),
		"https://www.infojobs.net/x/of-iB": `<html>No podemos identificar tu navegador</html>`,
	}
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "A" || !strings.Contains(jobs[0].Description, "Body A") {
		t.Errorf("jobs = %+v, want only A, hydrated", jobs)
	}
}

// An offer page refused with 405 is sent again: through the Scrapling tier the refusal retires
// its browser session, so the retry opens on another exit IP. Measured 2026-10-30, a session
// that started answering 405 answered it to 942 of the next ~950 pages, while a fresh one was
// served — so the refused page is worth one more navigation, not a deferral to the next run.
func TestInfojobsDetailRefusalIsRetriedOnce(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}})
	http.details = map[string]string{
		"https://www.infojobs.net/x/of-iA": ijPage(`{"offer":{"description":"Body A."}}`),
		"https://www.infojobs.net/x/of-iB": ijPage(`{"offer":{"description":"Body B."}}`),
	}
	http.refusedDetails = map[string]int{
		"https://www.infojobs.net/x/of-iA": 1,                      // served on the retry
		"https://www.infojobs.net/x/of-iB": infojobsDetailAttempts, // refused every time
	}
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "A" || !strings.Contains(jobs[0].Description, "Body A") {
		t.Errorf("jobs = %+v, want A hydrated after its refusal and B deferred", jobs)
	}
}

// A page that is not a refusal is not sent again: a gone offer, or a challenge page served with
// a 200, would be the same on the next navigation.
func TestInfojobsDetailNonRefusalIsNotRetried(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A")}})
	http.details = map[string]string{
		"https://www.infojobs.net/x/of-iA": `<html>No podemos identificar tu navegador</html>`,
	}
	if _, err := (infojobs{http: http}).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, u := range http.got {
		if u == "https://www.infojobs.net/x/of-iA" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("offer page requested %d times, want 1", n)
	}
}

// A run hydrates at most maxNewPerRun new offers, newest first — the categories are walked one
// after another, so their order says nothing about age — and the rest are deferred (not yielded,
// so not stored) and offered again next run, while every seen offer is still refreshed.
func TestInfojobsHydrationBudgetTakesTheNewestFirst(t *testing.T) {
	details := map[string]string{}
	for _, c := range []string{"A", "B", "C", "D"} {
		details["https://www.infojobs.net/x/of-i"+c] = ijPage(`{"offer":{"description":"Body ` + c + `."}}`)
	}
	http := ijSite(
		ijCat{id: "1", pages: [][]string{{ijOffer("S", "2026-10-01T00:00:00Z"), ijOffer("A", "2026-10-02T00:00:00Z"), ijOffer("B", "2026-10-28T00:00:00Z")}}},
		ijCat{id: "2", pages: [][]string{{ijOffer("C", "2026-10-03T00:00:00Z"), ijOffer("D", "2026-10-29T00:00:00Z")}}},
	)
	http.details = details
	jobs, err := infojobs{http: http, maxNewPerRun: 2}.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "S" })
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, j := range jobs {
		got = append(got, j.ExternalID)
	}
	sort.Strings(got)
	if strings.Join(got, "") != "BDS" {
		t.Errorf("jobs = %v, want the seen S plus the two newest new offers (B, D)", got)
	}
	detailCalls := 0
	for _, u := range http.got {
		if strings.Contains(u, "/x/of-i") {
			detailCalls++
		}
	}
	if detailCalls != 2 {
		t.Errorf("detail requests = %d, want 2 (the budget)", detailCalls)
	}
}

// Fetch (list-only) yields the teaser description without any detail request.
func TestInfojobsFetchListOnly(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{{
		`{"code":"Y","title":"QA","companyName":"Co","city":"Sevilla","teleworking":"",
		  "description":"Just the teaser.","link":"//www.infojobs.net/sevilla/qa/of-iY"}`,
	}}})
	jobs, err := infojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !strings.Contains(jobs[0].Description, "Just the teaser") {
		t.Fatalf("list-only teaser expected: %+v", jobs)
	}
	if jobs[0].WorkMode != "" {
		t.Fatalf("empty teleworking should yield empty work mode, got %q", jobs[0].WorkMode)
	}
	for _, u := range http.got {
		if !strings.HasPrefix(u, "https://www.infojobs.net/ofertas-trabajo?") {
			t.Fatalf("unexpected non-listing request: %q", u)
		}
	}
}

// A first-page listing failure is a board-level error.
func TestInfojobsListFailure(t *testing.T) {
	http := &infojobsHTTP{failList: true}
	if _, err := (infojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on list failure")
	}
}

// The double-encoded __INITIAL_PROPS__ decodes even when the payload contains escaped quotes and
// a `");` sequence that would defeat a lazy regex.
func TestInfojobsDecodeProps(t *testing.T) {
	inner := `{"offers":[{"description":"a \"quote\" then ); and \"end\");"}]}`
	var props infojobsListProps
	if err := infojobsDecodeProps(ijPage(inner), &props); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(props.Offers) != 1 || props.Offers[0].Description != `a "quote" then ); and "end");` {
		t.Fatalf("decoded offers mismatch: %+v", props.Offers)
	}
}

func TestInfojobsWorkMode(t *testing.T) {
	for label, want := range map[string]string{
		"Solo teletrabajo": "remote",
		"Híbrido":          "hybrid",
		"Presencial":       "onsite",
		"":                 "",
	} {
		if got := infojobsWorkMode(label); got != want {
			t.Errorf("infojobsWorkMode(%q) = %q, want %q", label, got, want)
		}
	}
}

// The whole-site listing stops serving at page 2999 (page 3000 answers an error page with no
// offers) while it states ~63.7k offers, 3,187 pages: the oldest ~3.7k offers sit past the cap
// and an unfiltered walk can never list them. The categories partition the catalogue and each
// is far under the cap, so the walk reads every category to its stated last page — and never
// the root listing past its first page, which only states the categories.
func TestInfojobsCrawlWalksEveryCategory(t *testing.T) {
	http := ijSite(
		ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("E")}},
		ijCat{id: "120", pages: [][]string{ijOffers("F", "G"), ijOffers("H")}},
	)
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if got := strings.Join(ijCodes(offers), ""); got != "ABCDEFGH" {
		t.Errorf("offers = %s, want every category's offers ABCDEFGH (and not the root's promoted one)", got)
	}
	for _, u := range http.got {
		if strings.Contains(u, "categoryIds=190") && strings.Contains(u, "page=5") {
			t.Errorf("read %s, past the category's stated end and its tail allowance", u)
		}
	}
}

// Offers published while a category is walked push its oldest ones past the page that was its
// last when the walk began. They are the same offers every run, so missing them is systematic
// and the 48-hour window cannot absorb it: the walk reads a tail past the stated end.
func TestInfojobsCrawlReadsOffersPushedPastTheStatedEnd(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D")}})
	http.root = ijRootPage(4, `{"value":"190","count":4}`)
	http.catPages["190"][3] = ijCategoryPage("190", 2, ijOffers("E")...)
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if got := strings.Join(ijCodes(offers), ""); got != "ABCDE" {
		t.Errorf("offers = %s, want ABCDE (E was pushed onto page 3)", got)
	}
}

// Offers removed while a category is walked can empty its last page; that is the end arriving
// early, not a failure.
func TestInfojobsCrawlAcceptsAnEmptiedLastPage(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("E")}})
	delete(http.catPages["190"], 3)
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(offers) != 4 {
		t.Errorf("got %d offers, want 4", len(offers))
	}
}

// An empty page with offers after it is not the end of the listing: something on the site
// stopped serving a range of pages (the shape of the whole-site cap), and the source-scoped sweep
// would close every offer on it.
func TestInfojobsCrawlFailsOnAnEmptyPageBeforeTheEnd(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("E", "F"), ijOffers("G")}})
	delete(http.catPages["190"], 2)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail on an empty page followed by offers")
	}
}

// More empty pages inside the stated range than drift can explain is a listing that stopped
// serving, not one that shrank.
func TestInfojobsCrawlFailsWhenTheStatedRangeEndsTooEarly(t *testing.T) {
	var pages [][]string
	for i := range 10 {
		pages = append(pages, ijOffers("P"+strconv.Itoa(i)))
	}
	http := ijSite(ijCat{id: "190", pages: pages})
	for p := 4; p <= 10; p++ {
		delete(http.catPages["190"], p)
	}
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail when most of the stated pages are empty")
	}
}

// The categories must account for every offer the site states, or the walk cannot prove it
// read the whole catalogue — and the provider is swept by source.
func TestInfojobsCrawlFailsWhenTheCategoriesDoNotCoverTheTotal(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}})
	http.root = ijRootPage(3, `{"value":"190","count":2}`)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error when the categories cover fewer offers than the stated total")
	}
}

// A category page that does not echo its filter is the whole listing served under the
// category's name (the semantic /ofertas-trabajo/<slug> path does exactly that); walking it
// once per category would read the site 21 times.
func TestInfojobsCrawlFailsWhenTheCategoryFilterIsIgnored(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}})
	http.catPages["190"][1] = ijPage(`{"navigation":{"totalPages":1},"search":{"categoryIds":[]},"offers":[` + strings.Join(ijOffers("A", "B"), ",") + `]}`)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error when a category page did not apply its filter")
	}
}

// The filter is checked on every page, not just the first: a later page served unfiltered holds
// other categories' offers in place of the ones it should list.
func TestInfojobsCrawlFailsWhenALaterPageIgnoresTheFilter(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D")}})
	http.catPages["190"][2] = ijPage(`{"navigation":{"totalPages":2},"search":{"categoryIds":[]},"offers":[` + strings.Join(ijOffers("X", "Y"), ",") + `]}`)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error when a later category page did not apply its filter")
	}
}

// A category the root says holds offers but whose listing comes back empty was not read.
func TestInfojobsCrawlFailsWhenAStatedCategoryListsNothing(t *testing.T) {
	http := ijSite(
		ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}},
		ijCat{id: "120", pages: [][]string{ijOffers("C")}},
	)
	http.catPages["120"][1] = ijCategoryPage("120", 1)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error when a category stated to hold offers lists none")
	}
}

func TestInfojobsCrawlRefusesACategoryPastTheCeiling(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B")}})
	http.catPages["190"][1] = ijCategoryPage("190", infojobsMaxPages+1, ijOffers("A", "B")...)
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error for a category longer than the site serves")
	}
	if n := http.listRequests(); n != 2 {
		t.Errorf("sent %d listing requests, want 2 (root, category page 1) before refusing", n)
	}
}

// A page that fails after the first — here the bot-challenge page, which carries no props blob —
// fails the whole crawl rather than returning the offers gathered so far, which the sweep would
// read as the full listing.
func TestInfojobsCrawlFailsWhenAnyLaterPageFails(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("E")}})
	http.catPages["190"][2] = `<html>No podemos identificar tu navegador</html>`
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail when one page is the bot challenge")
	}
}

// The listing is thousands of pages and each one is a browser navigation of several seconds,
// so pages are fetched concurrently — bounded — with offers that shifted between pages while the
// crawl ran counted once.
func TestInfojobsCrawlFetchesPagesConcurrently(t *testing.T) {
	http := ijSite(
		ijCat{id: "1", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("D", "E")}}, // D shifted down a page
		ijCat{id: "2", pages: [][]string{ijOffers("F", "G"), ijOffers("H", "I")}},
	)
	http.root = ijRootPage(9, `{"value":"1","count":5}`, `{"value":"2","count":4}`)
	http.delay = 20 * time.Millisecond
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if got := strings.Join(ijCodes(offers), ""); got != "ABCDEFGHI" {
		t.Errorf("offers = %s, want ABCDEFGHI (D once)", got)
	}
	if http.peak < 2 || http.peak > infojobsListWorkers {
		t.Errorf("peak concurrency = %d, want between 2 and %d", http.peak, infojobsListWorkers)
	}
}

// A listing page that fails is sent again: through the Scrapling tier a failed navigation
// retires its browser session, so the next attempt opens on another exit IP. Measured on prod
// 2026-10-26..28, every failed run died on the first page of a freshly opened session (pages 1,
// 501, 1001 against a 500-page session life) while the pages around it were served.
func TestInfojobsCrawlRetriesAFailedPage(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D"), ijOffers("E", "F")}})
	http.flaky = map[string]int{":1": infojobsListAttempts - 1, "190:1": infojobsListAttempts - 1, "190:3": infojobsListAttempts - 1}
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if len(offers) != 6 {
		t.Errorf("got %d offers, want 6", len(offers))
	}
}

// A page that keeps failing past the attempts still fails the crawl.
func TestInfojobsCrawlFailsAPageThatKeepsFailing(t *testing.T) {
	http := ijSite(ijCat{id: "190", pages: [][]string{ijOffers("A", "B"), ijOffers("C", "D")}})
	http.flaky = map[string]int{"190:2": infojobsListAttempts}
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail when a page fails every attempt")
	}
}

// The walk reads the whole catalogue and fails rather than truncate, so an offer it did not list
// is gone: infojobs is swept by source. The company-scoped close it had before could only reach
// the companies a run WROTE a posting for, and a run that hydrates ~60 new offers and refreshes
// ~6k seen ones wrote few of the catalogue's companies — 22.8k gone offers stayed open
// (2026-10-30).
func TestInfojobsIsFullCatalog(t *testing.T) {
	if !slices.Contains(FullCatalogProviders(map[string]Source{"infojobs": NewInfoJobs(nil)}), "infojobs") {
		t.Error("FullCatalogProviders() should include infojobs")
	}
}
