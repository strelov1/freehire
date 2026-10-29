package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// paycomFake stands in for the three transport roles the Paycom adapter uses: GetText (the
// portal page carrying the session JWT), PostJSONWithHeaders (the authed previews search),
// and GetJSONWithHeaders (the authed company-name + job detail). It serves a canned body per
// URL substring and records the Authorization header it last saw.
type paycomFake struct {
	pages map[string]string // GetText: url-substr -> html
	posts map[string]string // PostJSON: url-substr -> json
	gets  map[string]string // GetJSON: url-substr -> json
	// The adapter fans its detail fetches out across a worker pool, so this recorder is
	// written from several goroutines at once and needs the lock. The assertions read it
	// after Fetch has joined the pool, which is already ordered.
	mu     sync.Mutex
	lastAu string
}

func (f *paycomFake) GetText(_ context.Context, url string) (string, error) {
	for k, v := range f.pages {
		if strings.Contains(url, k) {
			return v, nil
		}
	}
	return "", fmt.Errorf("paycomFake: no page for %s", url)
}

func (f *paycomFake) match(m map[string]string, url string) (string, bool) {
	// longest-key-first so "job-postings/" wins over a broader substring
	best, ok := "", false
	for k := range m {
		if strings.Contains(url, k) && (!ok || len(k) > len(best)) {
			best, ok = k, true
		}
	}
	if !ok {
		return "", false
	}
	return m[best], true
}

func (f *paycomFake) GetJSONWithHeaders(_ context.Context, url string, h map[string]string, v any) error {
	f.mu.Lock()
	f.lastAu = h["Authorization"]
	f.mu.Unlock()
	body, ok := f.match(f.gets, url)
	if !ok {
		return fmt.Errorf("paycomFake: no GET for %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *paycomFake) PostJSONWithHeaders(_ context.Context, url string, h map[string]string, _, v any) error {
	f.mu.Lock()
	f.lastAu = h["Authorization"]
	f.mu.Unlock()
	body, ok := f.match(f.posts, url)
	if !ok {
		return fmt.Errorf("paycomFake: no POST for %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

const paycomPortalPage = `<html><head><script>
var configsFromHost = {"sessionJWT":"JWT123","atsPortalMantleServiceUrl":"https:\/\/portal-applicant-tracking.us-cent.paycomonline.net\/"};
</script></head><body>Loading...</body></html>`

func TestPaycomProvider(t *testing.T) {
	if got := NewPaycom(nil).Provider(); got != "paycom" {
		t.Errorf("Provider() = %q, want %q", got, "paycom")
	}
}

func TestPaycomFetchBootstrapsListsAndMaps(t *testing.T) {
	fake := &paycomFake{
		pages: map[string]string{"/portal/CK/jobs/1": paycomPortalPage},
		gets: map[string]string{
			"/api/ats/company-name":        `{"companyName":"City Electric Supply"}`,
			"/api/ats/job-postings/546849": `{"jobPosting":{"jobId":546849,"jobTitle":"Van Driver","location":"Centennial, CO","city":"Centennial","remoteType":"","description":"<p>Drive a van.</p><script>x()<\/script>","startDate":"2026-06-20T00:00:00+00:00"}}`,
			"/api/ats/job-postings/537487": `{"jobPosting":{"jobId":537487,"jobTitle":"Gear Specialist","location":"Maitland, FL","city":"Maitland","remoteType":"Remote","description":"<p>Quote gears.</p>","startDate":"2026-06-19T00:00:00+00:00"}}`,
		},
		posts: map[string]string{
			// one page of two previews, total count 2; jobId is a JSON number
			"/api/ats/job-posting-previews/search": `{"jobPostingPreviewsCount":2,"jobPostingPreviews":[{"jobId":546849},{"jobId":537487}]}`,
		},
	}

	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Company: "fallback", Board: "CK"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.lastAu != "JWT123" {
		t.Errorf("Authorization header = %q, want the bootstrapped JWT", fake.lastAu)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	v, ok := byID["546849"]
	if !ok {
		t.Fatalf("missing job 546849; got %v", byID)
	}
	if v.Title != "Van Driver" || v.Company != "City Electric Supply" || v.Location != "Centennial, CO" {
		t.Errorf("job 546849: title=%q company=%q loc=%q", v.Title, v.Company, v.Location)
	}
	if strings.Contains(v.Description, "<script>") {
		t.Errorf("description not sanitized: %q", v.Description)
	}
	if v.PostedAt == nil {
		t.Errorf("job 546849 PostedAt is nil, want parsed startDate")
	}
	if r := byID["537487"]; !r.Remote {
		t.Errorf("job 537487 should be Remote (remoteType=Remote)")
	}
}

// paycomRetiredPortal is the portal's verbatim answer for a client key it no longer serves,
// captured from prod on 2026-09-06: a 122-byte page under HTTP 200, with no session JWT and
// no Mantle host in it.
const paycomRetiredPortal = `
<div style="text-align:center">
    <p style="padding-top: 140px; font-size: 22px;">Job board does not exist.</p>
</div>
`

// A retired client key must come back as ErrBoardGone rather than as the missing-token
// error, which reads as a parse bug against a portal that was never served. 51 prod boards
// sat under that message from July to September 2026.
func TestPaycomFetchReportsBoardGone(t *testing.T) {
	fake := &paycomFake{pages: map[string]string{"/portal/": paycomRetiredPortal}}
	_, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "deadkey"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want it to wrap ErrBoardGone", err)
	}
	if !strings.Contains(err.Error(), "deadkey") {
		t.Errorf("err = %q, want the board named in it", err)
	}
}

// A portal that is served but yields no token is a different failure and must keep saying so
// — otherwise a transport or markup change would be filed as a dead board.
func TestPaycomFetchKeepsTokenFailureDistinctFromBoardGone(t *testing.T) {
	fake := &paycomFake{pages: map[string]string{"/portal/": `<html><body>Loading...</body></html>`}}
	_, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "livekey"})
	if err == nil {
		t.Fatal("a portal with no session token must fail")
	}
	if errors.Is(err, ErrBoardGone) {
		t.Errorf("err = %v, want a token failure, not ErrBoardGone", err)
	}
}

// The notice changed wording in 2026-10 ("... does not exist or is unavailable at this time."),
// and the old constant's trailing period stopped matching it, so every retired board failed on
// the missing token instead.
func TestPaycomFetchReportsBoardGoneUnderTheCurrentNotice(t *testing.T) {
	page := `<div style="text-align: center; padding-top: 140px; font-size: 22px;">Job board does not exist or is unavailable at this time.</div>`
	fake := &paycomFake{pages: map[string]string{"/portal/": page}}
	if _, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "deadkey"}); !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want it to wrap ErrBoardGone", err)
	}
}

func TestPaycomIsRegisteredAsFullBoardListing(t *testing.T) {
	if _, ok := NewPaycom(nil).(fullBoardListing); !ok {
		t.Error("paycom should implement the fullBoardListing marker")
	}
	if !FullBoardListingProviders(All(nil))["paycom"] {
		t.Error("FullBoardListingProviders(All(nil)) should include paycom")
	}
}

// paycomPagedFake serves a board of n postings through the real skip/take paging, stating
// count as the total, and lets a test break one page or one detail.
type paycomPagedFake struct {
	n, count   int
	failPage   int // page whose search request errors; -1 for none
	emptyPage  int // page that comes back with no previews; -1 for none
	detailErrs map[string]error
	posts      int
}

func (f *paycomPagedFake) GetText(context.Context, string) (string, error) {
	return paycomPortalPage, nil
}

func (f *paycomPagedFake) PostJSONWithHeaders(_ context.Context, _ string, _ map[string]string, body, v any) error {
	f.posts++
	skip := body.(map[string]any)["skip"].(int)
	page := skip / paycomPageSize
	if page == f.failPage {
		return &StatusError{Method: "POST", Code: 503}
	}
	var previews []string
	for i := skip; i < f.n && i < skip+paycomPageSize && page != f.emptyPage; i++ {
		previews = append(previews, fmt.Sprintf(`{"jobId":%d}`, 1000+i))
	}
	return json.Unmarshal([]byte(fmt.Sprintf(`{"jobPostingPreviewsCount":%d,"jobPostingPreviews":[%s]}`, f.count, strings.Join(previews, ","))), v)
}

func (f *paycomPagedFake) GetJSONWithHeaders(_ context.Context, url string, _ map[string]string, v any) error {
	if strings.HasSuffix(url, "/company-name") {
		return json.Unmarshal([]byte(`{"companyName":"Brown Sims PC"}`), v)
	}
	id := url[strings.LastIndex(url, "/")+1:]
	if err, ok := f.detailErrs[id]; ok {
		return err
	}
	if id == "2000" { // a 200 that carries no posting
		return json.Unmarshal([]byte(`{}`), v)
	}
	return json.Unmarshal([]byte(fmt.Sprintf(`{"jobPosting":{"jobId":%s,"jobTitle":"Paralegal","location":"Houston, TX","description":"<p>x</p>"}}`, id)), v)
}

// A board longer than one page is read to the total its search states.
func TestPaycomFetchReadsEveryPageToTheStatedTotal(t *testing.T) {
	fake := &paycomPagedFake{n: 120, count: 120, failPage: -1, emptyPage: -1}
	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "CK"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 120 {
		t.Errorf("got %d jobs, want 120", len(jobs))
	}
}

// An emptied board states a total of zero, and that is a complete (empty) listing.
func TestPaycomFetchReadsAnEmptyBoardAsEmpty(t *testing.T) {
	fake := &paycomPagedFake{n: 0, count: 0, failPage: -1, emptyPage: -1}
	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "CK"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("Fetch = %d jobs, %v; want an empty listing", len(jobs), err)
	}
}

// fullBoardListing's bar: a listing the crawl could not read to its stated total is a failed
// Fetch, never a partial success the board-scoped close would read as removals.
func TestPaycomFetchFailsAListingShortOfItsTotal(t *testing.T) {
	cases := map[string]*paycomPagedFake{
		"a later page errors":                    {n: 120, count: 120, failPage: 1, emptyPage: -1},
		"a later page is empty before the total": {n: 120, count: 120, failPage: -1, emptyPage: 1},
		"the first page errors":                  {n: 120, count: 120, failPage: 0, emptyPage: -1},
		"the page cap comes first":               {n: (paycomMaxPages + 1) * paycomPageSize, count: (paycomMaxPages + 1) * paycomPageSize, failPage: -1, emptyPage: -1},
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "CK"}); err == nil {
				t.Errorf("Fetch = %d jobs and no error, want a failed crawl", len(jobs))
			}
		})
	}
}

// An answer without the stated total is not a listing that ends here: read as a total of 0 it
// would end the walk on page one and hand the board scope a partial board.
func TestPaycomFetchFailsAPreviewsAnswerWithoutItsCount(t *testing.T) {
	fake := &paycomFake{
		pages: map[string]string{"/portal/CK/jobs/1": paycomPortalPage},
		gets:  map[string]string{"/api/ats/job-postings/": `{"jobPosting":{"jobId":1,"jobTitle":"Paralegal"}}`},
		posts: map[string]string{"/api/ats/job-posting-previews/search": `{"jobPostingPreviews":[{"jobId":1}]}`},
	}
	if jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "CK"}); err == nil {
		t.Errorf("Fetch = %d jobs and no error, want a failed crawl", len(jobs))
	}
}

// A posting the platform answers 404 for is gone ("Active job posting not found") and is
// dropped; a detail this crawl merely could not read is an Unreadable marker, so the sweep does
// not read it as a removal.
func TestPaycomFetchTellsAGoneDetailFromAnUnreadableOne(t *testing.T) {
	fake := &paycomPagedFake{n: 4, count: 4, failPage: -1, emptyPage: -1, detailErrs: map[string]error{
		"1001": &StatusError{Method: "GET", Code: 404},
		"1002": &StatusError{Method: "GET", Code: 503},
		"1003": errors.New("connection reset"),
	}}
	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Company: "Brown Sims", Board: "CK"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ExternalID] = j.Unreadable
	}
	want := map[string]bool{"1000": false, "1002": true, "1003": true}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ExternalID -> Unreadable = %v, want %v (1001 answered 404 and is dropped)", got, want)
	}
}

// A 200 detail that carries no posting is unreadable, not gone.
func TestPaycomFetchReadsAnEmptyDetailAsUnreadable(t *testing.T) {
	fake := &paycomPagedFake{n: 1001, count: 1001, failPage: -1, emptyPage: -1}
	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Board: "CK"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "2000" && !j.Unreadable {
			t.Error("posting 2000 answered 200 with no posting and must be Unreadable")
		}
	}
}
