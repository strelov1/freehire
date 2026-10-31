package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

// phenomFake is a JSONPoster that routes by the request body's ddoKey: a refineSearch
// returns the canned page for its "from" offset, a jobDetail returns the canned detail
// for its jobSeqNo (or errors when absent, so the adapter skips that posting). It records
// every jobSeqNo whose detail was asked for.
type phenomFake struct {
	pages   map[int]string
	details map[string]string
	// slices serves the listing filtered to one state (selected_fields), by state and offset.
	slices map[string]map[int]string

	mu     sync.Mutex
	detail []string
}

func (f *phenomFake) PostJSON(_ context.Context, _ string, body, v any) error {
	m := body.(map[string]any)
	switch m["ddoKey"] {
	case "refineSearch":
		pages := f.pages
		if sel, ok := m["selected_fields"].(map[string][]string); ok {
			pages = f.slices[sel["state"][0]]
		}
		page, ok := pages[m["from"].(int)]
		if !ok {
			return fmt.Errorf("unexpected page from=%v", m["from"])
		}
		return json.Unmarshal([]byte(page), v)
	case "jobDetail":
		seq := m["jobSeqNo"].(string)
		f.mu.Lock()
		f.detail = append(f.detail, seq)
		f.mu.Unlock()
		js, ok := f.details[seq]
		if !ok {
			return fmt.Errorf("no detail for %v", seq)
		}
		return json.Unmarshal([]byte(js), v)
	}
	return fmt.Errorf("unexpected ddoKey %v", m["ddoKey"])
}

func (f *phenomFake) detailsAsked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.detail...)
}

// phenomListPage renders a refineSearch page from a list of jobSeqNos, stating total as the
// board's totalHits.
func phenomListPage(total int, seqs ...string) string {
	jobs := make([]string, len(seqs))
	for i, s := range seqs {
		jobs[i] = fmt.Sprintf(`{"jobSeqNo":%q,"title":"Engineer %s","cityState":"Bonn, NRW","locale":"en_GLOBAL","postedDate":"2026-05-17T22:00:00.000+0000"}`, s, s)
	}
	return fmt.Sprintf(`{"refineSearch":{"totalHits":%d,"data":{"jobs":[%s]}}}`, total, strings.Join(jobs, ","))
}

func phenomDetail(desc string) string {
	return fmt.Sprintf(`{"jobDetail":{"data":{"job":{"description":%q,"title":"Detail Title"}}}}`, desc)
}

// phenomSeqs returns n sequence ids starting at S<from>.
func phenomSeqs(from, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("S%d", from+i)
	}
	return out
}

// newPhenomFake serves a board of total postings in pages of pageLen — the server's own
// page length, which may be shorter than the size the adapter asks for — every one with a
// detail.
func newPhenomFake(total, pageLen int) *phenomFake {
	f := &phenomFake{pages: map[int]string{}, details: map[string]string{}}
	for from := 0; from < total; from += pageLen {
		seqs := phenomSeqs(from, min(pageLen, total-from))
		f.pages[from] = phenomListPage(total, seqs...)
		for _, s := range seqs {
			f.details[s] = phenomDetail("<p>full body for " + s + "</p>")
		}
	}
	f.pages[total] = phenomListPage(total)
	return f
}

var phenomEntry = CompanyEntry{Company: "DHL", Provider: "phenom", Board: "careers.dhl.com"}

func TestPhenomProvider(t *testing.T) {
	if got := NewPhenom(nil).Provider(); got != "phenom" {
		t.Errorf("Provider() = %q, want %q", got, "phenom")
	}
}

func TestPhenomFetchListsToTheStatedTotalAndFetchesDetail(t *testing.T) {
	// The server clamps a page below the size asked for, so the walk advances by what each
	// page held and stops at the board's stated total, not at the first page shorter than
	// phenomPageSize.
	fake := newPhenomFake(7, 3)
	delete(fake.details, "S6") // its detail errors -> that posting is skipped

	jobs, err := NewPhenom(fake).Fetch(context.Background(), phenomEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 6 {
		t.Fatalf("len(jobs) = %d, want 6 (7 listed, S6 without detail)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["S0"]
	if !ok {
		t.Fatal("posting S0 missing")
	}
	if j.Title != "Engineer S0" {
		t.Errorf("Title = %q, want the list title", j.Title)
	}
	if j.URL != "https://careers.dhl.com/global/en/job/S0" {
		t.Errorf("URL = %q, want locale-derived public page", j.URL)
	}
	if j.Location != "Bonn, NRW" {
		t.Errorf("Location = %q, want cityState", j.Location)
	}
	if !strings.Contains(j.Description, "full body for S0") {
		t.Errorf("Description = %q, want the detail body", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed postedDate (2026)", j.PostedAt)
	}
}

// The listing serves nothing once from+size reaches 10,000, so a board stating more is read one
// state at a time (selected_fields): each slice is under the window and is walked to its own
// end. A posting listed in several states comes back once.
func TestPhenomReadsABoardOverTheWindowStateByState(t *testing.T) {
	fake := &phenomFake{
		pages: map[int]string{0: phenomOverWindowPage(15000, map[string]int{"Ohio": 9000, "Texas": 7000}, "S0")},
		slices: map[string]map[int]string{
			"Ohio":  {0: phenomListPage(9000, "O1", "O2"), 2: phenomListPage(9000, "O3", "S9"), 4: phenomListPage(0)},
			"Texas": {0: phenomListPage(7000, "T1", "S9"), 2: phenomListPage(0)},
		},
	}
	all := func(string) bool { return true }
	jobs, err := NewPhenom(fake).(HydratingSource).FetchNew(context.Background(), phenomEntry, all)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	slices.Sort(ids)
	if got := strings.Join(ids, ","); got != "O1,O2,O3,S9,T1" {
		t.Errorf("read %q, want every state's postings once (one listed in two states is one)", got)
	}
}

// A board over the window is read only when its slices account for every posting it states and
// each fits under the window; otherwise the crawl fails rather than passing a partial read off
// as the board.
func TestPhenomFailsABoardOverTheWindowItCannotSlice(t *testing.T) {
	twoStates := phenomOverWindowPage(15000, map[string]int{"Ohio": 9000, "Texas": 7000})
	readable := map[int]string{0: phenomListPage(7000, "T1"), 1: phenomListPage(0)}
	cases := map[string]struct {
		first  string
		slices map[string]map[int]string
	}{
		"states short of the total": {first: phenomOverWindowPage(15000, map[string]int{"Ohio": 9000, "Texas": 5000})},
		"a state over the window":   {first: phenomOverWindowPage(15000, map[string]int{"Ohio": 12000, "Texas": 3000})},
		// Its last page may start anywhere below 9,700, so from+size can cross 10,000.
		"a state too close to the window": {first: phenomOverWindowPage(15000, map[string]int{"Ohio": 9700, "Texas": 5300})},
		"no state counts":                 {first: phenomListPage(15000)},
		"a counted state's slice is empty": {first: twoStates, slices: map[string]map[int]string{
			"Ohio": {0: phenomListPage(0)}, "Texas": readable,
		}},
		"a slice the filter did not narrow": {first: twoStates, slices: map[string]map[int]string{
			"Ohio": {0: phenomListPage(15000, "S0")}, "Texas": readable,
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &phenomFake{pages: map[int]string{0: c.first}, slices: c.slices}
			if _, err := NewPhenom(fake).Fetch(context.Background(), phenomEntry); err == nil {
				t.Fatal("Fetch succeeded on a board over the window it could not slice")
			}
		})
	}
}

// phenomOverWindowPage is the first page of a board stating more postings than the listing
// window serves, carrying the per-state counts the request asks for.
func phenomOverWindowPage(total int, states map[string]int, seqs ...string) string {
	agg, _ := json.Marshal([]map[string]any{{"field": "state", "value": states}})
	return strings.Replace(phenomListPage(total, seqs...), `"data":{`, `"data":{"aggregations":`+string(agg)+`,`, 1)
}

// An answer without totalHits is not a listing this adapter can prove complete.
func TestPhenomFailsAListingWithoutATotal(t *testing.T) {
	fake := &phenomFake{pages: map[int]string{
		0: `{"refineSearch":{"data":{"jobs":[]}}}`,
	}}
	if _, err := NewPhenom(fake).Fetch(context.Background(), phenomEntry); err == nil {
		t.Fatal("Fetch succeeded on a listing that stated no total")
	}
}

func TestPhenomEmptyBoardIsAnEmptyListing(t *testing.T) {
	fake := &phenomFake{pages: map[int]string{0: phenomListPage(0)}}
	jobs, err := NewPhenom(fake).Fetch(context.Background(), phenomEntry)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("Fetch = %d jobs, %v; want none and no error", len(jobs), err)
	}
}

func TestPhenomFailsAListingPastItsPageCeiling(t *testing.T) {
	// Every page claims more remains; the walk must stop at its ceiling and fail.
	fake := &phenomFake{pages: map[int]string{}}
	for p := 0; p <= phenomMaxPages; p++ {
		fake.pages[p] = phenomListPage(phenomListingWindow, fmt.Sprintf("S%d", p))
	}
	if _, err := NewPhenom(fake).Fetch(context.Background(), phenomEntry); err == nil {
		t.Fatal("Fetch succeeded past its page ceiling")
	}
}

// A stored posting is re-listed as a SeenRefresh with no detail request: re-reading every
// stored body on every run is what kept phenom's crawl from reaching most of its boards.
func TestPhenomFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	fake := newPhenomFake(4, 4)
	seen := func(id string) bool { return id == "S0" || id == "S2" }

	jobs, err := NewPhenom(fake).(HydratingSource).FetchNew(context.Background(), phenomEntry, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if got := strings.Join(slices.Sorted(slices.Values(fake.detailsAsked())), ","); got != "S1,S3" {
		t.Errorf("details asked for %q, want only the unseen S1,S3", got)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 4 {
		t.Fatalf("yielded %d postings, want all 4", len(byID))
	}
	r := byID["S0"]
	if !r.SeenRefresh || r.Description != "" || r.URL != "https://careers.dhl.com/global/en/job/S0" || r.Company != "DHL" {
		t.Errorf("S0 = %+v, want a SeenRefresh carrying the listing's identity", r)
	}
	if h := byID["S1"]; h.SeenRefresh || !strings.Contains(h.Description, "full body for S1") {
		t.Errorf("S1 = %+v, want a hydrated posting", h)
	}
}

func TestPhenomFetchNewBoundsHydrationPerBoard(t *testing.T) {
	fake := newPhenomFake(phenomMaxNewPerBoard+5, phenomPageSize)
	never := func(string) bool { return false }

	jobs, err := NewPhenom(fake).(HydratingSource).FetchNew(context.Background(), phenomEntry, never)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if n := len(fake.detailsAsked()); n != phenomMaxNewPerBoard {
		t.Errorf("asked %d details, want the per-board bound %d", n, phenomMaxNewPerBoard)
	}
	if len(jobs) != phenomMaxNewPerBoard {
		t.Errorf("yielded %d postings, want %d (the rest left for a later run)", len(jobs), phenomMaxNewPerBoard)
	}
}

// Every board of a run shares one ~1 req/s pace, so the boards crawled first must not spend the
// run's time on bodies and leave the later boards' listings unread at the deadline.
func TestPhenomFetchNewBoundsHydrationPerRun(t *testing.T) {
	src := NewPhenom(nil).(phenom)
	never := func(string) bool { return false }
	asked := 0
	for i := 0; asked < phenomMaxNewPerRun+phenomMaxNewPerBoard; i++ {
		fake := newPhenomFake(phenomMaxNewPerBoard, phenomPageSize)
		src.http = fake
		e := phenomEntry
		e.Board = fmt.Sprintf("board%d.phenompro.com", i)
		if _, err := src.FetchNew(context.Background(), e, never); err != nil {
			t.Fatalf("FetchNew: %v", err)
		}
		n := len(fake.detailsAsked())
		if n == 0 {
			break
		}
		asked += n
	}
	if asked != phenomMaxNewPerRun {
		t.Errorf("run hydrated %d postings, want the run bound %d", asked, phenomMaxNewPerRun)
	}
}

// Phenom's offset paging is not a stable slice: two walks of one board at the same moment read
// different subsets (careers.kohls.com, 2026-10-30: 4,403 and 4,398 distinct of a stated 5,211,
// together exactly 5,211), so no single crawl proves it read the whole board.
func TestPhenomIsNotFullBoardListing(t *testing.T) {
	if FullBoardListingProviders(map[string]Source{"phenom": NewPhenom(nil)})["phenom"] {
		t.Error("phenom carries fullBoardListing, but one walk of its listing reads ~85-92% of a board")
	}
}

func TestPhenomJobURL(t *testing.T) {
	cases := []struct{ board, locale, seq, want string }{
		{"careers.dhl.com", "en_GLOBAL", "X1", "https://careers.dhl.com/global/en/job/X1"},
		{"careers.dhl.com", "en_us", "X2", "https://careers.dhl.com/us/en/job/X2"},
		{"careers.dhl.com", "", "X3", "https://careers.dhl.com/job/X3"},
	}
	for _, c := range cases {
		if got := phenomJobURL(c.board, c.locale, c.seq); got != c.want {
			t.Errorf("phenomJobURL(%q,%q,%q) = %q, want %q", c.board, c.locale, c.seq, got, c.want)
		}
	}
}
