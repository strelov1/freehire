package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
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
func ijPage(t *testing.T, innerJSON string) string {
	t.Helper()
	lit, err := json.Marshal(innerJSON)
	if err != nil {
		t.Fatalf("marshal literal: %v", err)
	}
	return `<html><head><script>window.__INITIAL_PROPS__ = JSON.parse(` + string(lit) + `);</script></head><body></body></html>`
}

// infojobsHTTP is a test TextGetter serving listing pages keyed by ?page=N and offer pages by
// full URL; anything else fails.
type infojobsHTTP struct {
	listPages map[int]string
	details   map[string]string
	failList  bool
	delay     time.Duration
	// flaky is how many times each listing page fails before it is served.
	flaky map[int]int

	mu       sync.Mutex
	got      []string
	inFlight int
	peak     int
}

var infojobsPageRE = regexp.MustCompile(`[?&]page=(\d+)`)

func (f *infojobsHTTP) GetText(_ context.Context, url string) (string, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.inFlight++
	if f.inFlight > f.peak {
		f.peak = f.inFlight
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	time.Sleep(f.delay)
	if strings.HasPrefix(url, "https://www.infojobs.net/ofertas-trabajo?") {
		if f.failList {
			return "", errors.New("infojobsHTTP: list boom")
		}
		page := 1
		if m := infojobsPageRE.FindStringSubmatch(url); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		f.mu.Lock()
		failing := f.flaky[page] > 0
		if failing {
			f.flaky[page]--
		}
		f.mu.Unlock()
		if failing {
			return "", errors.New("infojobsHTTP: browser fetch timed out")
		}
		if body, ok := f.listPages[page]; ok {
			return body, nil
		}
		return `<html>window.__INITIAL_PROPS__ = JSON.parse("{\"offers\":[]}");</html>`, nil
	}
	if body, ok := f.details[url]; ok {
		return body, nil
	}
	return "", errors.New("infojobsHTTP: unexpected request " + url)
}

func TestInfojobsFetchNewRefreshesSeenAndHydratesNew(t *testing.T) {
	// Two offers on page 1; totalElements=2 stops pagination after one page. Offer B's body
	// carries an embedded quote and a `");` sequence to exercise the decoder.
	list := `{"overview":{"totalElements":2},"offers":[
	  {"code":"AAA","title":"Backend Engineer","companyName":"Acme","city":"Madrid",
	   "publishedAt":"2026-07-12T08:00:00Z","teleworking":"Presencial","link":"//www.infojobs.net/madrid/backend/of-iAAA?applicationOrigin=search"},
	  {"code":"BBB","title":"Data Scientist","companyName":"Beta","city":"Barcelona","description":"Teaser.",
	   "publishedAt":"2026-07-11T09:00:00Z","teleworking":"Solo teletrabajo","link":"//www.infojobs.net/barcelona/data/of-iBBB?applicationOrigin=search"}
	]}`
	http := &infojobsHTTP{
		listPages: map[int]string{1: ijPage(t, list)},
		details: map[string]string{
			"https://www.infojobs.net/barcelona/data/of-iBBB": ijPage(t, `{"offer":{"description":"Full body. He said \"hi\"); and more text."}}`),
		},
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
	http := &infojobsHTTP{listPages: map[int]string{1: ijListPage(t, 2, "A", "B")}, details: map[string]string{
		"https://www.infojobs.net/x/of-iA": ijPage(t, `{"offer":{"description":"Body A."}}`),
		"https://www.infojobs.net/x/of-iB": `<html>No podemos identificar tu navegador</html>`,
	}}
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "A" || !strings.Contains(jobs[0].Description, "Body A") {
		t.Errorf("jobs = %+v, want only A, hydrated", jobs)
	}
}

// A run hydrates at most maxNewPerRun new offers; the rest are deferred — not yielded, so not
// stored — and offered again next run, while every seen offer is still refreshed.
func TestInfojobsHydrationBudgetDefersTheRest(t *testing.T) {
	details := map[string]string{}
	for _, c := range []string{"A", "B", "C", "D"} {
		details["https://www.infojobs.net/x/of-i"+c] = ijPage(t, `{"offer":{"description":"Body `+c+`."}}`)
	}
	http := &infojobsHTTP{listPages: map[int]string{1: ijListPage(t, 5, "S", "A", "B", "C", "D")}, details: details}
	jobs, err := infojobs{http: http, maxNewPerRun: 2}.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == "S" })
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, j := range jobs {
		got = append(got, j.ExternalID)
	}
	sort.Strings(got)
	if strings.Join(got, "") != "ABS" {
		t.Errorf("jobs = %v, want the seen S plus the first two new offers", got)
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
	list := `{"overview":{"totalElements":1},"offers":[
	  {"code":"Y","title":"QA","companyName":"Co","city":"Sevilla","teleworking":"",
	   "description":"Just the teaser.","link":"//www.infojobs.net/sevilla/qa/of-iY"}]}`
	http := &infojobsHTTP{listPages: map[int]string{1: ijPage(t, list)}}
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
	// No detail URL should have been requested.
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
	page := ijPage(t, inner)
	var props infojobsListProps
	if err := infojobsDecodeProps(page, &props); err != nil {
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

// A page that fails after the first — here the bot-challenge page, which carries no props blob —
// fails the whole crawl rather than returning the offers gathered so far, which the sweep would
// read as the full listing.
func TestInfojobsLaterPageFailureFailsCrawl(t *testing.T) {
	list := `{"overview":{"totalElements":3},"offers":[
	  {"code":"A","title":"Dev","companyName":"Co","link":"//www.infojobs.net/x/of-iA"}]}`
	http := &infojobsHTTP{listPages: map[int]string{
		1: ijPage(t, list),
		2: `<html><title>InfoJobs</title>No podemos identificar tu navegador</html>`,
	}}
	if _, err := (infojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when a later listing page fails, got a partial list")
	}
}

func ijListPage(t *testing.T, total int, codes ...string) string {
	t.Helper()
	var offers []string
	for _, c := range codes {
		offers = append(offers, `{"code":"`+c+`","title":"T","companyName":"Co","link":"//www.infojobs.net/x/of-i`+c+`"}`)
	}
	return ijPage(t, `{"overview":{"totalElements":`+strconv.Itoa(total)+`},"offers":[`+strings.Join(offers, ",")+`]}`)
}

// The listing is thousands of pages and each one is a browser navigation of several seconds,
// so pages after the first are fetched concurrently — bounded, in page order, with offers that
// shifted between pages while the crawl ran counted once.
func TestInfojobsCrawlFetchesPagesConcurrentlyInOrder(t *testing.T) {
	http := &infojobsHTTP{delay: 20 * time.Millisecond, listPages: map[int]string{
		1: ijListPage(t, 9, "A", "B"),
		2: ijListPage(t, 9, "C", "D"),
		3: ijListPage(t, 9, "D", "E"), // D shifted down a page mid-crawl
		4: ijListPage(t, 9, "F", "G"),
		5: ijListPage(t, 9, "H", "I"),
	}}
	offers, err := infojobs{http: http}.crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	var codes []string
	for _, o := range offers {
		codes = append(codes, o.Code)
	}
	if got := strings.Join(codes, ""); got != "ABCDEFGHI" {
		t.Errorf("offers = %s, want ABCDEFGHI (page order, D once)", got)
	}
	if len(http.got) != 5 {
		t.Errorf("fetched %d pages, want 5 (ceil(9/2))", len(http.got))
	}
	if http.peak < 2 || http.peak > infojobsListWorkers {
		t.Errorf("peak concurrency = %d, want between 2 and %d", http.peak, infojobsListWorkers)
	}
}

func TestInfojobsCrawlFailsWhenAnyLaterPageFails(t *testing.T) {
	http := &infojobsHTTP{listPages: map[int]string{
		1: ijListPage(t, 6, "A", "B"),
		2: ijListPage(t, 6, "C", "D"),
		3: `<html>No podemos identificar tu navegador</html>`,
	}}
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail when one page is the bot challenge")
	}
}

// A listing page that fails is sent again: through the Scrapling tier a failed navigation
// retires its browser session, so the next attempt opens on another exit IP. Measured on prod
// 2026-10-26..28, every failed run died on the first page of a freshly opened session (pages 1,
// 501, 1001 against a 500-page session life) while the pages around it were served.
func TestInfojobsCrawlRetriesAFailedPage(t *testing.T) {
	http := &infojobsHTTP{
		listPages: map[int]string{
			1: ijListPage(t, 6, "A", "B"),
			2: ijListPage(t, 6, "C", "D"),
			3: ijListPage(t, 6, "E", "F"),
		},
		flaky: map[int]int{1: infojobsListAttempts - 1, 3: infojobsListAttempts - 1},
	}
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
	http := &infojobsHTTP{
		listPages: map[int]string{1: ijListPage(t, 4, "A", "B"), 2: ijListPage(t, 4, "C", "D")},
		flaky:     map[int]int{2: infojobsListAttempts},
	}
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want the crawl to fail when a page fails every attempt")
	}
	if len(http.got) != 1+infojobsListAttempts {
		t.Errorf("sent %d requests, want %d", len(http.got), 1+infojobsListAttempts)
	}
}

func TestInfojobsCrawlRefusesAListingPastTheCeiling(t *testing.T) {
	http := &infojobsHTTP{listPages: map[int]string{1: ijListPage(t, 2*(infojobsMaxPages+1), "A", "B")}}
	if _, err := (infojobs{http: http}).crawl(context.Background()); err == nil {
		t.Fatal("want an error for a listing longer than the page ceiling")
	}
	if len(http.got) != 1 {
		t.Errorf("fetched %d pages, want only the first before refusing", len(http.got))
	}
}
