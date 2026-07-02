package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaleoProvider(t *testing.T) {
	if got := NewTaleo(nil).Provider(); got != "taleo" {
		t.Errorf("Provider() = %q, want %q", got, "taleo")
	}
}

func TestTaleoInRegistry(t *testing.T) {
	if s, ok := All(nil)["taleo"]; !ok || s.Provider() != "taleo" {
		t.Fatal("All() missing provider taleo")
	}
}

func TestTaleoBoardValidation(t *testing.T) {
	for _, board := range []string{"", "valero.taleo.net", "/2", "valero.taleo.net/"} {
		if _, err := NewTaleo(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Company: "Valero", Board: board}); err == nil {
			t.Errorf("board %q: want error, got nil", board)
		}
	}
}

// taleoCareersection is a minimal careersection HTML carrying the portalNo the adapter
// scrapes to authorize the searchjobs POST.
const taleoCareersection = `<html><head><script>
	var config = { portalNo: '101430233', portalCode: 2 };
</script></head><body>Job Search</body></html>`

// taleoJobDetail builds a jobdetail.ftl page whose description lives in the hidden
// initialHistory input: URL-encoded HTML after the "!*!" marker, then more "!|!"-delimited
// state. This mirrors the live Taleo shape the adapter must decode.
func taleoJobDetail(encodedDescHTML string) string {
	return fmt.Sprintf(`<html><body>
		<input type="hidden" name="focusOnField" id="focusOnField" value="" />
		<input type="hidden" name="initialHistory" id="initialHistory"
			value="ftlx0!|!x!%%24!requisitionDescriptionInterface!|!descRequisition!|!!*!%s!|!false!|!true" />
		</body></html>`, encodedDescHTML)
}

// TestTaleoFetch covers the core path: scrape the portal from the careersection, POST
// searchjobs to page the requisition list, then decode each job's description from its
// detail page. external_id is contestNo; location is the JSON-array column; the description
// is URL-decoded and sanitized.
func TestTaleoFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		route("careersection/2/jobsearch.ftl", taleoCareersection).
		route("searchjobs", `{
			"requisitionList": [
				{"jobId": "311673", "contestNo": "2600197", "locationsColumns": [1],
				 "column": ["Occupational Health Professional", "[\"US-OK-Ardmore\"]", "Jul 2, 2026"]},
				{"jobId": "311680", "contestNo": "2600205", "locationsColumns": [1],
				 "column": ["Backend Engineer", "[\"US-TX-San Antonio\"]", "Jun 30, 2026"]}
			],
			"pagingData": {"totalCount": 2}
		}`).
		// Description HTML url-encoded: "<p>Provide care.</p><script>alert(1)</script>"
		route("job=2600197", taleoJobDetail("%3Cp%3EProvide%20care.%3C%2Fp%3E%3Cscript%3Ealert(1)%3C%2Fscript%3E")).
		route("job=2600205", taleoJobDetail("%3Cp%3EWrite%20Go.%3C%2Fp%3E"))

	jobs, err := NewTaleo(fake).Fetch(context.Background(), CompanyEntry{Company: "Valero", Board: "valero.taleo.net/2"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["2600197"]
	if !ok {
		t.Fatalf("missing job with external_id=contestNo 2600197; got %v", byID)
	}
	if j.Title != "Occupational Health Professional" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Valero" {
		t.Errorf("Company = %q, want Valero", j.Company)
	}
	if j.Location != "US-OK-Ardmore" {
		t.Errorf("Location = %q, want US-OK-Ardmore", j.Location)
	}
	if !strings.Contains(j.URL, "valero.taleo.net/careersection/2/jobdetail.ftl?job=2600197") {
		t.Errorf("URL = %q", j.URL)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-02" {
		t.Errorf("PostedAt = %v, want 2026-07-02", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Provide care") {
		t.Errorf("Description missing text: %q", j.Description)
	}
	if strings.Contains(j.Description, "script") || strings.Contains(j.Description, "alert") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
}

// TestTaleoFields locks the varying column layout across tenants: location is read from the
// API's locationsColumns index (never guessed), and the posted date only from a column that
// parses as "Jan 2, 2006". A tenant whose listing omits location/date (schneider/baesystems
// shapes) yields empty location and a nil date rather than misreading an id column.
func TestTaleoFields(t *testing.T) {
	cases := []struct {
		name         string
		req          taleoRequisition
		wantTitle    string
		wantLocation string
		wantPosted   string // "" means nil
	}{
		{
			name:         "valero: title, location, date",
			req:          taleoRequisition{Column: []string{"Occupational Health Professional", `["US-OK-Ardmore"]`, "Jul 2, 2026"}, LocationsColumns: []int{1}},
			wantTitle:    "Occupational Health Professional",
			wantLocation: "US-OK-Ardmore",
			wantPosted:   "2026-07-02",
		},
		{
			name:         "schneider: title + id column, no location index",
			req:          taleoRequisition{Column: []string{"Dedicated truck driver", "261143"}, LocationsColumns: []int{}},
			wantTitle:    "Dedicated truck driver",
			wantLocation: "",
			wantPosted:   "",
		},
		{
			name:         "baesystems: title only",
			req:          taleoRequisition{Column: []string{"Principal Commercial Officer"}},
			wantTitle:    "Principal Commercial Officer",
			wantLocation: "",
			wantPosted:   "",
		},
		{
			name:         "multi-location index joins",
			req:          taleoRequisition{Column: []string{"Role", `["US-TX-Austin","US-CA-SF"]`}, LocationsColumns: []int{1}},
			wantTitle:    "Role",
			wantLocation: "US-TX-Austin, US-CA-SF",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, location, posted := taleoFields(tc.req)
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if location != tc.wantLocation {
				t.Errorf("location = %q, want %q", location, tc.wantLocation)
			}
			got := ""
			if posted != nil {
				got = posted.Format("2006-01-02")
			}
			if got != tc.wantPosted {
				t.Errorf("posted = %q, want %q", got, tc.wantPosted)
			}
		})
	}
}

// TestKeyedMutexSerializesPerKey asserts the same key is mutually exclusive (a second lock
// blocks until the first unlocks) while distinct keys are independent.
func TestKeyedMutexSerializesPerKey(t *testing.T) {
	k := newKeyedMutex()

	// Distinct keys must not block each other: acquire both without releasing.
	unlockA := k.lock("a")
	unlockB := k.lock("b")
	unlockB()

	// Same key must be exclusive: a second lock on "a" cannot proceed until unlockA runs.
	acquired := make(chan struct{})
	go func() {
		release := k.lock("a")
		close(acquired)
		release()
	}()
	select {
	case <-acquired:
		t.Fatal("second lock on key \"a\" acquired while first still held")
	case <-time.After(20 * time.Millisecond):
	}
	unlockA()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second lock on key \"a\" never acquired after release")
	}
}

// TestTaleoConcurrentSameHostFetch runs two crawls of the same host concurrently through the
// (concurrency-safe) fake; with -race this guards the per-host locking added for session
// isolation against data races and deadlocks.
func TestTaleoConcurrentSameHostFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		route("jobsearch.ftl", taleoCareersection).
		route("searchjobs", `{"requisitionList":[{"jobId":"1","contestNo":"c1","locationsColumns":[1],"column":["Role","[\"US\"]","Jul 1, 2026"]}],"pagingData":{"totalCount":1}}`).
		route("jobdetail.ftl", taleoJobDetail("%3Cp%3Ex%3C%2Fp%3E"))
	s := NewTaleo(fake)

	var wg sync.WaitGroup
	for _, board := range []string{"acme.taleo.net/1", "acme.taleo.net/2"} {
		wg.Add(1)
		go func(b string) {
			defer wg.Done()
			jobs, err := s.Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: b})
			if err != nil {
				t.Errorf("board %s: %v", b, err)
			}
			if len(jobs) != 1 {
				t.Errorf("board %s: got %d jobs, want 1", b, len(jobs))
			}
		}(board)
	}
	wg.Wait()
}

// taleoPagingFake serves the careersection + detail via URL routes but returns searchjobs
// pages from a queue, so pagination (pageNo in the POST body, not the URL) can be exercised.
type taleoPagingFake struct {
	routes []struct{ match, body string }
	mu     sync.Mutex
	pages  []string
	posts  int
}

func (f *taleoPagingFake) route(match, body string) *taleoPagingFake {
	f.routes = append(f.routes, struct{ match, body string }{match, body})
	return f
}

func (f *taleoPagingFake) GetText(_ context.Context, url string) (string, error) {
	for _, r := range f.routes {
		if strings.Contains(url, r.match) {
			return r.body, nil
		}
	}
	return "", fmt.Errorf("taleoPagingFake: no route for %s", url)
}

func (f *taleoPagingFake) PostJSONWithHeaders(_ context.Context, _ string, _ map[string]string, _, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.posts >= len(f.pages) {
		return fmt.Errorf("taleoPagingFake: unexpected extra searchjobs POST #%d", f.posts+1)
	}
	body := f.pages[f.posts]
	f.posts++
	return json.Unmarshal([]byte(body), v)
}

func TestTaleoPaginates(t *testing.T) {
	page := func(contest string) string {
		return fmt.Sprintf(`{"requisitionList":[{"jobId":"%s","contestNo":"%s","column":["Role %s","[\"US\"]","Jul 1, 2026"]}],"pagingData":{"totalCount":3}}`, contest, contest, contest)
	}
	fake := (&taleoPagingFake{
		pages: []string{page("1"), page("2"), page("3")},
	}).
		route("jobsearch.ftl", taleoCareersection).
		route("jobdetail.ftl", taleoJobDetail("%3Cp%3Ex%3C%2Fp%3E"))

	jobs, err := NewTaleo(fake).Fetch(context.Background(), CompanyEntry{Company: "Valero", Board: "valero.taleo.net/2"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (paged)", len(jobs))
	}
	if fake.posts != 3 {
		t.Errorf("made %d searchjobs POSTs, want 3", fake.posts)
	}
}
