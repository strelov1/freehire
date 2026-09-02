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

func TestParseDayforceBoard(t *testing.T) {
	cases := []struct {
		board                 string
		wantErr               bool
		tenant, site, culture string
	}{
		{"dcrusa/join-us", false, "dcrusa", "join-us", "en-US"},
		{"gm/candidateportal/fr-CA", false, "gm", "candidateportal", "fr-CA"},
		{"gm/candidateportal/", false, "gm", "candidateportal", "en-US"}, // empty culture = default
		{"dcrusa", true, "", "", ""},                                     // no site
		{"/join-us", true, "", "", ""},                                   // no tenant
		{"a/b/c/d", true, "", "", ""},                                    // too many parts
		{"", true, "", "", ""},
	}
	for _, tc := range cases {
		b, err := parseDayforceBoard(tc.board)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseDayforceBoard(%q) error = %v, wantErr %v", tc.board, err, tc.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if b.tenant != tc.tenant || b.site != tc.site || b.culture != tc.culture {
			t.Errorf("parseDayforceBoard(%q) = %+v, want %s/%s/%s", tc.board, b, tc.tenant, tc.site, tc.culture)
		}
	}
}

func TestDayforceSiteID(t *testing.T) {
	cases := map[string]string{
		"gm/candidateportal/fr-CA": "gm/candidateportal",
		"gm/candidateportal":       "gm/candidateportal",
		"nonsense":                 "nonsense",
		"a/b/c/d":                  "a/b/c/d",
	}
	for board, want := range cases {
		if got := dayforceSiteID(board); got != want {
			t.Errorf("dayforceSiteID(%q) = %q, want %q", board, got, want)
		}
	}
}

// Two cultures of one career site are one crawl target: the same posting keeps one
// jobPostingId in both, so keeping both entries would store it twice under two external_id
// namespaces and let the company-scoped sweep close whichever copy a run did not refresh.
func TestParseConfigFoldsDayforceCultureVariants(t *testing.T) {
	data := []byte(`
- company: Le Groupe Maurice
  board: gm/candidateportal/fr-CA
- company: Le Groupe Maurice (English)
  board: gm/candidateportal
- company: dnata
  board: dcrusa/join-us
`)
	cfg, err := ParseConfig("dayforce", data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	want := []CompanyEntry{
		{Company: "Le Groupe Maurice", Provider: "dayforce", Board: "gm/candidateportal/fr-CA"}, // first wins
		{Company: "dnata", Provider: "dayforce", Board: "dcrusa/join-us"},
	}
	if len(cfg.Sources) != len(want) {
		t.Fatalf("len(Sources) = %d, want %d: %+v", len(cfg.Sources), len(want), cfg.Sources)
	}
	for i, w := range want {
		if cfg.Sources[i] != w {
			t.Errorf("Sources[%d] = %+v, want %+v", i, cfg.Sources[i], w)
		}
	}
}

// dayforceFake serves the CSRF bootstrap GET and the listing POST, paging by the request
// body's paginationStart. routedHTTP cannot: its routes match on the URL, and every listing
// page of a board is requested at the same URL with a different body.
type dayforceFake struct {
	mu sync.Mutex
	// pages maps a paginationStart to the page body served for it; a missing offset errors.
	pages map[int]string
	// tokens are the tokens successive bootstrap GETs issue; the last one repeats once spent.
	tokens []string
	// accept is the token the listing accepts; every other one is refused with a 403, the
	// shape a stale token takes. "" accepts whatever it is given.
	accept string
	// gets counts bootstrap GETs, headers records the header of each listing POST.
	gets    int
	headers []string
}

func (f *dayforceFake) GetJSON(_ context.Context, _ string, v any) error {
	f.mu.Lock()
	var token string
	if len(f.tokens) > 0 {
		token = f.tokens[min(f.gets, len(f.tokens)-1)]
	}
	f.gets++
	f.mu.Unlock()
	return json.Unmarshal([]byte(fmt.Sprintf(`{"csrfToken":%q}`, token)), v)
}

func (f *dayforceFake) PostJSONWithHeaders(_ context.Context, _ string, headers map[string]string, body, v any) error {
	token := headers[dayforceCSRFHeader]
	f.mu.Lock()
	f.headers = append(f.headers, token)
	f.mu.Unlock()
	if f.accept != "" && token != f.accept {
		return &StatusError{Method: "POST", Code: 403, URL: dayforceBaseURL}
	}
	start, _ := body.(map[string]any)["paginationStart"].(int)
	page, ok := f.pages[start]
	if !ok {
		return fmt.Errorf("dayforceFake: no page at offset %d", start)
	}
	return json.Unmarshal([]byte(page), v)
}

// dayforcePage renders a listing page carrying maxCount and the given postings verbatim.
func dayforcePage(maxCount int, postings ...string) string {
	return fmt.Sprintf(`{"maxCount":%d,"jobPostings":[%s]}`, maxCount, strings.Join(postings, ","))
}

const dayforceSitedPosting = `{
	"jobPostingId": 2566,
	"jobTitle": "  Backend Engineer  ",
	"jobDescription": "We&rsquo;re hiring.\n\nYou will:\n- ship Go\n- keep it simple",
	"postingStartTimestampUTC": "2026-09-01T05:00:00+00:00",
	"hasVirtualLocation": false,
	"postingLocations": [
		{"cityName":"Houston","stateCode":"TX","isoCountryCode":"US"},
		{"cityName":"Toronto","stateCode":"ON","isoCountryCode":"CA"}
	]
}`

const dayforceVirtualPosting = `{
	"jobPostingId": 3851,
	"jobTitle": "Director, Strategic Services",
	"jobDescription": "Lead the practice.",
	"hasVirtualLocation": true,
	"postingLocations": null
}`

func TestDayforceFetch(t *testing.T) {
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0: dayforcePage(2, dayforceSitedPosting, dayforceVirtualPosting),
	}}

	jobs, err := NewDayforce(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "dcrusa/join-us"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "2566" || j.Title != "Backend Engineer" || j.Company != "Acme" {
		t.Errorf("job0 id/title/company = %q/%q/%q", j.ExternalID, j.Title, j.Company)
	}
	if want := "https://jobs.dayforcehcm.com/en-US/dcrusa/join-us/jobs/2566"; j.URL != want {
		t.Errorf("job0 URL = %q, want %q", j.URL, want)
	}
	// Several locations are joined with "; ", each rendered "City, State, Country-code".
	if want := "Houston, TX, US; Toronto, ON, CA"; j.Location != want {
		t.Errorf("job0 Location = %q, want %q", j.Location, want)
	}
	if len(j.Countries) != 2 || j.Countries[0] != "us" || j.Countries[1] != "ca" {
		t.Errorf("job0 Countries = %v, want [us ca]", j.Countries)
	}
	// The entity-encoded plain-text body is decoded and rebuilt into paragraphs and a list.
	if !strings.Contains(j.Description, "We’re hiring.") {
		t.Errorf("job0 Description did not decode entities: %q", j.Description)
	}
	if !strings.Contains(j.Description, "<li>ship Go</li>") {
		t.Errorf("job0 Description did not rebuild the bullet list: %q", j.Description)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("job0 Remote/WorkMode = %v/%q, want false/empty", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("job0 PostedAt is nil")
	}

	// A virtual posting carries no location at all; the platform's own flag is the structured
	// remote signal.
	v := jobs[1]
	if !v.Remote || v.WorkMode != "remote" {
		t.Errorf("job1 Remote/WorkMode = %v/%q, want true/remote", v.Remote, v.WorkMode)
	}
	if v.Location != "" || v.Countries != nil {
		t.Errorf("job1 Location/Countries = %q/%v, want empty", v.Location, v.Countries)
	}
	if v.PostedAt != nil {
		t.Errorf("job1 PostedAt = %v, want nil (the posting states none)", v.PostedAt)
	}
}

// A place merely NAMED "Remote" sets the heuristic remote flag but must not reach WorkMode,
// which carries structured signal only.
func TestDayforceFetchRemoteNamedPlaceIsNotStructured(t *testing.T) {
	posting := `{"jobPostingId":7,"jobTitle":"SRE","jobDescription":"body",
		"hasVirtualLocation":false,"postingLocations":[{"cityName":"Remote","isoCountryCode":"US"}]}`
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{0: dayforcePage(1, posting)}}

	jobs, err := NewDayforce(fake).Fetch(context.Background(), CompanyEntry{Board: "t/s"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !jobs[0].Remote || jobs[0].WorkMode != "" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/empty", jobs[0].Remote, jobs[0].WorkMode)
	}
}

// The board's culture reaches both the request and the posting URL.
func TestDayforceFetchUsesTheBoardCulture(t *testing.T) {
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0: dayforcePage(1, `{"jobPostingId":12,"jobTitle":"Conseiller","jobDescription":"corps"}`),
	}}
	jobs, err := NewDayforce(fake).Fetch(context.Background(),
		CompanyEntry{Board: "gm/candidateportal/fr-CA"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "https://jobs.dayforcehcm.com/fr-CA/gm/candidateportal/jobs/12"; jobs[0].URL != want {
		t.Errorf("URL = %q, want %q", jobs[0].URL, want)
	}
}

// maxCount is exact, so the walk stops once it has that many postings rather than requesting
// the empty page past the end.
func TestDayforceFetchPaginatesToMaxCount(t *testing.T) {
	first := make([]string, dayforcePageSize)
	for i := range first {
		first[i] = fmt.Sprintf(`{"jobPostingId":%d,"jobTitle":"role","jobDescription":"body"}`, i+1)
	}
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0:                dayforcePage(dayforcePageSize+1, first...),
		dayforcePageSize: dayforcePage(dayforcePageSize+1, `{"jobPostingId":99,"jobTitle":"last","jobDescription":"body"}`),
		// No page at offset 50: requesting one would error and fail the assertion below.
	}}

	jobs, err := NewDayforce(fake).Fetch(context.Background(), CompanyEntry{Board: "t/s"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != dayforcePageSize+1 {
		t.Fatalf("got %d jobs, want %d", len(jobs), dayforcePageSize+1)
	}
}

// An empty page ends the walk even when maxCount claims more, so a count that ever went
// wrong cannot spin the loop.
func TestDayforceFetchStopsOnAnEmptyPage(t *testing.T) {
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0:                dayforcePage(1000, `{"jobPostingId":1,"jobTitle":"role","jobDescription":"body"}`),
		dayforcePageSize: dayforcePage(1000),
	}}
	jobs, err := NewDayforce(fake).Fetch(context.Background(), CompanyEntry{Board: "t/s"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
}

// The first page failing is a board-level error; a later page failing ends the walk with what
// was already gathered.
func TestDayforceFetchPageFailures(t *testing.T) {
	empty := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{}}
	if _, err := NewDayforce(empty).Fetch(context.Background(), CompanyEntry{Board: "t/s"}); err == nil {
		t.Error("want an error when the first listing page fails")
	}

	first := make([]string, dayforcePageSize)
	for i := range first {
		first[i] = fmt.Sprintf(`{"jobPostingId":%d,"jobTitle":"role","jobDescription":"body"}`, i+1)
	}
	partial := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0: dayforcePage(1000, first...), // page 2 is missing → the fake errors
	}}
	jobs, err := NewDayforce(partial).Fetch(context.Background(), CompanyEntry{Board: "t/s"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != dayforcePageSize {
		t.Errorf("got %d jobs, want the %d gathered before the failure", len(jobs), dayforcePageSize)
	}
}

func TestDayforceFetchRejectsAMalformedBoard(t *testing.T) {
	if _, err := NewDayforce(&dayforceFake{}).Fetch(context.Background(), CompanyEntry{Board: "bad"}); err == nil {
		t.Error("want an error for a malformed board")
	}
}

// The token is minted once for the whole crawl and echoed on every listing POST, so a run
// over thousands of boards costs one bootstrap, not one per board.
func TestDayforceCSRFTokenIsMintedOnceAndEchoed(t *testing.T) {
	fake := &dayforceFake{tokens: []string{"tok"}, pages: map[int]string{
		0: dayforcePage(1, `{"jobPostingId":1,"jobTitle":"role","jobDescription":"body"}`),
	}}
	src := NewDayforce(fake)
	for _, board := range []string{"a/s", "b/s"} {
		if _, err := src.Fetch(context.Background(), CompanyEntry{Board: board}); err != nil {
			t.Fatalf("Fetch %s: %v", board, err)
		}
	}
	if fake.gets != 1 {
		t.Errorf("bootstrap GETs = %d, want 1", fake.gets)
	}
	for i, h := range fake.headers {
		if h != "tok" {
			t.Errorf("listing POST %d carried %s = %q, want %q", i, dayforceCSRFHeader, h, "tok")
		}
	}
}

// A token minted once for a whole run can go stale mid-run, and a stale one refuses every
// board left in it. A refusal re-mints and gives the board a second chance.
func TestDayforceStaleCSRFTokenIsReMintedOnce(t *testing.T) {
	fake := &dayforceFake{
		tokens: []string{"stale", "fresh"},
		accept: "fresh",
		pages: map[int]string{
			0: dayforcePage(1, `{"jobPostingId":1,"jobTitle":"role","jobDescription":"body"}`),
		},
	}
	src := NewDayforce(fake)
	jobs, err := src.Fetch(context.Background(), CompanyEntry{Board: "a/s"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the retry should have succeeded)", len(jobs))
	}
	// The next board is handed the fresh token straight away rather than being refused again.
	if _, err := src.Fetch(context.Background(), CompanyEntry{Board: "b/s"}); err != nil {
		t.Fatalf("Fetch second board: %v", err)
	}
	if fake.gets != 2 {
		t.Errorf("bootstrap GETs = %d, want 2 (one mint, one re-mint)", fake.gets)
	}
	if want := []string{"stale", "fresh", "fresh"}; !slices.Equal(fake.headers, want) {
		t.Errorf("listing POSTs carried %v, want %v", fake.headers, want)
	}
}

// A refusal the fresh token does not fix is the board's own failure, not a token to keep
// re-minting: the retry happens once and then the error stands.
func TestDayforceRefusalSurvivingTheReMintFailsTheBoard(t *testing.T) {
	fake := &dayforceFake{
		tokens: []string{"one", "two"},
		accept: "never-issued",
		pages: map[int]string{
			0: dayforcePage(1, `{"jobPostingId":1,"jobTitle":"role","jobDescription":"body"}`),
		},
	}
	if _, err := NewDayforce(fake).Fetch(context.Background(), CompanyEntry{Board: "a/s"}); err == nil {
		t.Fatal("want an error when the board is refused with a freshly minted token")
	}
	if fake.gets != 2 {
		t.Errorf("bootstrap GETs = %d, want 2 (exactly one re-mint)", fake.gets)
	}
}

// A bootstrap that issues no token fails the board rather than posting an empty header the
// platform would refuse.
func TestDayforceCSRFBootstrapWithoutATokenErrors(t *testing.T) {
	fake := &dayforceFake{tokens: []string{""}, pages: map[int]string{0: dayforcePage(0)}}
	if _, err := NewDayforce(fake).Fetch(context.Background(), CompanyEntry{Board: "t/s"}); err == nil {
		t.Error("want an error when the bootstrap issues no token")
	}
}

func TestDayforceRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["dayforce"]
	if !ok {
		t.Fatal("All() missing provider dayforce")
	}
	if s.Provider() != "dayforce" {
		t.Errorf("All()[dayforce].Provider() = %q", s.Provider())
	}
	// Board-based (not boardless): it appears in the source facet.
	if !slices.Contains(FilterableProviders(), "dayforce") {
		t.Error("FilterableProviders() should include dayforce")
	}
}
