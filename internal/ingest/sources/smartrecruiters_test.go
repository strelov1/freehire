package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// routedHTTP is a test HTTPClient that returns a different canned body per URL,
// matching the first route whose substring is contained in the requested URL. It is
// concurrency-safe so the SmartRecruiters adapter can fan out detail fetches.
type routedHTTP struct {
	routes    []struct{ match, body string }
	errRoutes []struct {
		match string
		err   error
	}
	redirects []struct{ match, final string }
	mu        sync.Mutex
	calls     int
}

// routeRedirect makes a GetJSONResolved request whose URL contains match resolve to final and
// fail its decode — the shape of a platform that answers "not serving this board" by
// redirecting onto an HTML page rather than with a status code.
func (r *routedHTTP) routeRedirect(match, final string) *routedHTTP {
	r.redirects = append(r.redirects, struct{ match, final string }{match, final})
	return r
}

func (r *routedHTTP) route(match, body string) *routedHTTP {
	r.routes = append(r.routes, struct{ match, body string }{match, body})
	return r
}

// routeErr makes every request whose URL contains match fail with err, ahead of any body
// route. It exists so a test can drive what an adapter does with a PARTICULAR failure — the
// platform answering 404 for a posting it has taken down reads nothing like an origin refusing
// to serve one, and only the first is evidence a posting is gone.
func (r *routedHTTP) routeErr(match string, err error) *routedHTTP {
	r.errRoutes = append(r.errRoutes, struct {
		match string
		err   error
	}{match, err})
	return r
}

// routedErr returns the error routed for url, or nil when none is.
func (r *routedHTTP) routedErr(url string) error {
	for _, rt := range r.errRoutes {
		if strings.Contains(url, rt.match) {
			return rt.err
		}
	}
	return nil
}

func (r *routedHTTP) GetJSON(_ context.Context, url string, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

// GetJSONResolved answers with the final URL a routeRedirect declared for this request, or
// with the requested URL when none did. A declared redirect also fails the decode, since the
// platform this models lands the follow on an HTML page.
func (r *routedHTTP) GetJSONResolved(_ context.Context, url string, v any) (string, error) {
	r.mu.Lock()
	final, redirected := "", false
	for _, rd := range r.redirects {
		if strings.Contains(url, rd.match) {
			final, redirected = rd.final, true
			break
		}
	}
	r.mu.Unlock()
	if redirected {
		return final, errors.New("invalid character '<' looking for beginning of value")
	}
	return url, r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetXML(_ context.Context, url string, v any) error {
	return r.decode(url, xml.Unmarshal, v)
}

func (r *routedHTTP) PostJSON(_ context.Context, url string, _, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetJSONWithHeaders(_ context.Context, url string, _ map[string]string, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) PostJSONWithHeaders(_ context.Context, url string, _ map[string]string, _, v any) error {
	return r.decode(url, json.Unmarshal, v)
}

func (r *routedHTTP) GetHTML(_ context.Context, url string) (*html.Node, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if err := r.routedErr(url); err != nil {
		return nil, err
	}
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return html.Parse(strings.NewReader(rt.body))
		}
	}
	return nil, fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) GetStream(_ context.Context, url, _ string, fn func(io.Reader) error) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if err := r.routedErr(url); err != nil {
		return err
	}
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return fn(strings.NewReader(rt.body))
		}
	}
	return fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) GetText(_ context.Context, url string) (string, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if err := r.routedErr(url); err != nil {
		return "", err
	}
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return rt.body, nil
		}
	}
	return "", fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) GetLargeText(_ context.Context, url string) (string, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if err := r.routedErr(url); err != nil {
		return "", err
	}
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return rt.body, nil
		}
	}
	return "", fmt.Errorf("routedHTTP: no route for %s", url)
}

func (r *routedHTTP) decode(url string, unmarshal func([]byte, any) error, v any) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if err := r.routedErr(url); err != nil {
		return err
	}
	for _, rt := range r.routes {
		if strings.Contains(url, rt.match) {
			return unmarshal([]byte(rt.body), v)
		}
	}
	return fmt.Errorf("routedHTTP: no route for %s", url)
}

func detailBody(id, title string) string {
	return fmt.Sprintf(`{
		"id": %q,
		"postingUrl": "https://jobs.smartrecruiters.com/Acme/%s",
		"experienceLevel": {"id": "mid_senior_level", "label": "Mid-Senior Level"},
		"typeOfEmployment": {"id": "permanent", "label": "Full-time"},
		"jobAd": {"sections": {
			"companyDescription": {"title": "Company", "text": "<p>boilerplate</p>"},
			"jobDescription": {"title": "Job", "text": "<p>%s do the job.</p>"},
			"qualifications": {"title": "Qualifications", "text": "<ul><li>Go</li></ul>"},
			"additionalInformation": {"title": "More", "text": "<p>EEO notice.</p>"}
		}}
	}`, id, id, title)
}

func TestSmartRecruitersProvider(t *testing.T) {
	if got := NewSmartRecruiters(nil).Provider(); got != "smartrecruiters" {
		t.Errorf("Provider() = %q, want %q", got, "smartrecruiters")
	}
}

// SmartRecruiters earns fullBoardListing because listPostings proves completeness:
// totalFound is authoritative (pages until offset >= totalFound or an empty page), no
// artificial cap exists, and any listing error aborts the whole Fetch.
func TestSmartRecruitersMarkers(t *testing.T) {
	s := NewSmartRecruiters(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("smartrecruiters should implement the fullBoardListing marker")
	}
}

func TestSmartRecruitersRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["smartrecruiters"] {
		t.Error("FullBoardListingProviders(All(nil)) should include smartrecruiters")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestSmartRecruitersMarkers' fullBoardListing claim rests on.
func TestSmartRecruitersFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

// The SmartRecruiters experienceLevel enum maps onto freehire's seniority vocabulary.
// Ambiguous/unset values (not_applicable, unknown) map to "" so the title dictionary
// decides instead — structured signal only, never a guess.
func TestSmartRecruitersSeniority(t *testing.T) {
	cases := map[string]string{
		"internship":       "intern",
		"entry_level":      "junior",
		"associate":        "middle",
		"mid_senior_level": "senior",
		"director":         "lead",
		"executive":        "c_level",
		"not_applicable":   "",
		"":                 "",
		"something_new":    "",
	}
	for in, want := range cases {
		if got := smartRecruitersSeniority(in); got != want {
			t.Errorf("smartRecruitersSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

// The SmartRecruiters typeOfEmployment enum maps onto freehire's employment-type
// vocabulary; unknown/absent ids map to "" so the description parser decides.
func TestSmartRecruitersEmploymentType(t *testing.T) {
	cases := map[string]string{
		"permanent":   "full_time",
		"full-time":   "full_time",
		"part-time":   "part_time",
		"contract":    "contract",
		"temporary":   "contract",
		"internship":  "internship",
		"traineeship": "internship",
		"":            "",
		"whatever":    "",
	}
	for in, want := range cases {
		if got := smartRecruitersEmploymentType(in); got != want {
			t.Errorf("smartRecruitersEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSmartRecruitersFetchPaginatesAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 3, "content": [
			{"id": "P1", "name": "Backend Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Berlin", "region": "", "country": "de", "remote": true}},
			{"id": "P2", "name": "Frontend Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Remote", "country": "us", "remote": true}}
		]}`).
		route("offset=2", `{"totalFound": 3, "content": [
			{"id": "P3", "name": "Data Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "NYC", "country": "us", "remote": false}}
		]}`).
		route("/postings/P1", detailBody("P1", "P1")).
		route("/postings/P2", detailBody("P2", "P2")).
		route("/postings/P3", detailBody("P3", "P3"))

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 across two pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["P1"]
	if !ok {
		t.Fatal("posting P1 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.smartrecruiters.com/Acme/P1" {
		t.Errorf("URL = %q, want the postingUrl from detail", j.URL)
	}
	if j.Location != "Berlin, de" {
		t.Errorf("Location = %q, want city/country joined", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from location.remote")
	}
	for _, want := range []string{"do the job.", "Go", "EEO notice."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "boilerplate") {
		t.Errorf("Description should exclude companyDescription, got %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed releasedDate (2024)", j.PostedAt)
	}
	if j.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior (mapped from experienceLevel mid_senior_level)", j.Seniority)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (mapped from typeOfEmployment permanent)", j.EmploymentType)
	}
}

// A tenant that puts the whole ad in companyDescription and leaves the three role sections
// empty would otherwise be ingested with no description at all (1,066 such live rows on prod,
// freehire#1866). The boilerplate exclusion is a preference, not a rule: with nothing else to
// show, the company section IS the posting.
func TestSmartRecruitersFallsBackToCompanyDescription(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 1, "content": [
			{"id": "P1", "name": "Backend Engineer", "location": {"city": "Berlin", "country": "de"}}
		]}`).
		route("/postings/P1", `{
			"id": "P1",
			"postingUrl": "https://jobs.smartrecruiters.com/Acme/P1",
			"jobAd": {"sections": {
				"companyDescription": {"title": "Company", "text": "<p>Acme builds rockets. Join us.</p>"},
				"jobDescription": {"title": "Job", "text": ""},
				"qualifications": {"title": "Qualifications", "text": ""},
				"additionalInformation": {"title": "More", "text": ""}
			}}
		}`)

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "smartrecruiters", Board: "Acme", Company: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "Acme builds rockets") {
		t.Errorf("Description = %q, want the companyDescription fallback", jobs[0].Description)
	}
}

// The fallback must not fire when the role sections carry text — a posting with a body keeps
// excluding the boilerplate, which is the whole point of the exclusion.
func TestSmartRecruitersFallbackDoesNotFireWhenRoleSectionsFilled(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 1, "content": [
			{"id": "P1", "name": "Backend Engineer", "location": {"city": "Berlin", "country": "de"}}
		]}`).
		route("/postings/P1", `{
			"id": "P1",
			"postingUrl": "https://jobs.smartrecruiters.com/Acme/P1",
			"jobAd": {"sections": {
				"companyDescription": {"title": "Company", "text": "<p>boilerplate</p>"},
				"jobDescription": {"title": "Job", "text": ""},
				"qualifications": {"title": "Qualifications", "text": ""},
				"additionalInformation": {"title": "More", "text": "<p>EEO notice.</p>"}
			}}
		}`)

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "smartrecruiters", Board: "Acme", Company: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if strings.Contains(jobs[0].Description, "boilerplate") {
		t.Errorf("Description = %q, want companyDescription still excluded", jobs[0].Description)
	}
}

// A detail request the crawl could not READ must not look like a posting that is not there:
// the detail is smartrecruiters' only source for a posting and is re-requested every run, so a
// dropped one leaves a live vacancy missing from a crawl that reported no failure, and the
// stale-job sweep closes it once the grace window elapses.
func TestSmartRecruitersUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	// P2 has no detail route -> its detail fetch errors with a plain transport failure.
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 2, "content": [
			{"id": "P1", "name": "Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Berlin", "country": "de", "remote": false}},
			{"id": "P2", "name": "Broken", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "NYC", "country": "us", "remote": false}}
		]}`).
		route("/postings/P1", detailBody("P1", "P1"))

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != "P1" {
		t.Fatalf("read = %v, want only P1", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "P2" {
		t.Fatalf("unreadable markers = %v, want one for P2", markers)
	}
}

// SmartRecruiters carries remote and hybrid as separate booleans on the posting's
// location, and a hybrid posting sets remote=false — so reading location.remote alone
// leaves every hybrid role without a work mode.
func TestSmartRecruitersFetchHybridPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("offset=0", `{"totalFound": 1, "content": [
			{"id": "H1", "name": "Product Owner", "releasedDate": "2024-06-11T15:19:46.134Z",
			 "location": {"city": "Sunshine Coast", "region": "Queensland", "country": "au", "remote": false, "hybrid": true}}
		]}`).
		route("/postings/H1", detailBody("H1", "H1"))

	jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	if jobs[0].WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from location.hybrid", jobs[0].WorkMode)
	}
	if jobs[0].Remote {
		t.Error("Remote = true, want false: a hybrid posting is not remote")
	}
}

// A 200 carrying nothing usable is the same hole as a failed request, and it does not look
// like one anywhere: GetJSON returns no error, the LISTING already supplied the id, the name
// and the location, and the posting goes on looking read while holding no URL and no
// description. The run would count it toward the board's coverage and the sweep would close it
// all the same — the very defect the marker exists to prevent, wearing a success.
func TestSmartRecruitersEmptyDetailPayloadIsUnreadable(t *testing.T) {
	for name, body := range map[string]string{
		"empty object":     `{}`,
		"null":             `null`,
		"interstitial":     `{"someOtherShape": true}`,
		"blank postingUrl": `{"postingUrl": "   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := (&routedHTTP{}).
				route("offset=0", `{"totalFound": 2, "content": [
					{"id": "P1", "name": "Engineer", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "Berlin", "country": "de", "remote": false}},
					{"id": "P2", "name": "Broken", "releasedDate": "2024-06-11T15:19:46.134Z", "location": {"city": "NYC", "country": "us", "remote": false}}
				]}`).
				route("/postings/P2", body).
				route("/postings/P1", detailBody("P1", "P1"))

			jobs, err := NewSmartRecruiters(fake).Fetch(context.Background(), CompanyEntry{
				Company: "Acme", Provider: "smartrecruiters", Board: "Acme",
			})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			read := readPostings(jobs)
			if len(read) != 1 || read[0].ExternalID != "P1" {
				t.Fatalf("read = %v, want only P1 — P2's detail answered with nothing", read)
			}
			markers := unreadableMarkers(jobs)
			if len(markers) != 1 || markers[0].ExternalID != "P2" {
				t.Fatalf("unreadable markers = %v, want one for P2", markers)
			}
		})
	}
}
