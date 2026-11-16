package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const wttjTestBoard = "cybersecurity-2MDhm"

// wttjTestEnv is the real /api/env response (2026-10-14), trimmed to a few of its keys, with the
// public search key replaced so a secret scanner has nothing to flag.
const wttjTestEnv = `window.env = {"PUBLIC_ALGOLIA_API_KEY_CLIENT":"public-search-key-redacted-0000000",` +
	`"PUBLIC_WTTJ_ALERTS_API_URL":"https://alerts.welcometothejungle.com","PUBLIC_ALGOLIA_APPLICATION_ID":"CSEKHVMS53",` +
	`"PUBLIC_WTTJ_URL":"https://www.welcometothejungle.com"};`

// wttjFakeHit is one indexed posting: the hit as the index returns it, plus the numeric
// published_at_timestamp the index filters on (not retrieved, so not part of the hit). hasTS
// false models a document carrying no timestamp at all, which no window can reach.
type wttjFakeHit struct {
	raw   json.RawMessage
	ts    int64
	hasTS bool
}

// wttjFakeHTTP plays the three WTTJ endpoints: /api/env, an Algolia index that honours the
// adapter's board + timestamp-window filter, hitsPerPage and the 1,000-hit cap, and the
// posting API keyed by "org/slug".
type wttjFakeHTTP struct {
	mu sync.Mutex

	hits    []wttjFakeHit
	details map[string]string // posting API body keyed by "org/slug"
	// detailErr answers a posting API request with this error instead.
	detailErr map[string]error
	// failQuery fails the n-th Algolia query (1-based); 0 never fails.
	failQuery int

	queries      []url.Values
	headers      map[string]string
	detailsAsked []string
}

var (
	wttjBoardFilterRE  = regexp.MustCompile(`new_profession\.sub_category_reference:"([^"]+)"`)
	wttjWindowFilterRE = regexp.MustCompile(`published_at_timestamp >= (\d+) AND published_at_timestamp < (\d+)`)
	wttjDetailPathRE   = regexp.MustCompile(`/organizations/([^/]+)/jobs/([^/]+)$`)
)

func (f *wttjFakeHTTP) GetText(_ context.Context, u string) (string, error) {
	if u != wttjEnvURL {
		return "", fmt.Errorf("unexpected GetText %s", u)
	}
	return wttjTestEnv, nil
}

func (f *wttjFakeHTTP) PostJSONWithHeaders(_ context.Context, u string, headers map[string]string, body, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u != "https://CSEKHVMS53-dsn.algolia.net/1/indexes/wttj_jobs_production_en/query" {
		return fmt.Errorf("unexpected query URL %s", u)
	}
	f.headers = headers
	p, err := url.ParseQuery(body.(map[string]string)["params"])
	if err != nil {
		return err
	}
	f.queries = append(f.queries, p)
	if f.failQuery == len(f.queries) {
		return &StatusError{Method: http.MethodPost, Code: http.StatusBadGateway, URL: u}
	}
	filter := p.Get("filters")
	m := wttjBoardFilterRE.FindStringSubmatch(filter)
	if m == nil {
		return fmt.Errorf("query without a board filter: %s", filter)
	}
	var lo, hi int64
	windowed := false
	if w := wttjWindowFilterRE.FindStringSubmatch(filter); w != nil {
		windowed = true
		lo, _ = strconv.ParseInt(w[1], 10, 64)
		hi, _ = strconv.ParseInt(w[2], 10, 64)
	}
	per, _ := strconv.Atoi(p.Get("hitsPerPage"))
	if per > wttjHitsCap {
		return &StatusError{Method: http.MethodPost, Code: http.StatusBadRequest, URL: u}
	}
	var matched []json.RawMessage
	if m[1] == wttjTestBoard {
		for _, h := range f.hits {
			if windowed && (!h.hasTS || h.ts < lo || h.ts >= hi) {
				continue
			}
			matched = append(matched, h.raw)
		}
	}
	page := matched
	if len(page) > per {
		page = page[:per]
	}
	out, _ := json.Marshal(map[string]any{"hits": page, "nbHits": len(matched)})
	return json.Unmarshal(out, v)
}

func (f *wttjFakeHTTP) GetJSON(_ context.Context, u string, v any) error {
	m := wttjDetailPathRE.FindStringSubmatch(u)
	if m == nil || !strings.HasPrefix(u, "https://api.welcometothejungle.com/api/v1/") {
		return fmt.Errorf("unexpected GetJSON %s", u)
	}
	key := m[1] + "/" + m[2]
	f.mu.Lock()
	f.detailsAsked = append(f.detailsAsked, key)
	err, body := f.detailErr[key], f.details[key]
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if body == "" {
		return &StatusError{Method: http.MethodGet, Code: http.StatusNotFound, URL: u}
	}
	return json.Unmarshal([]byte(body), v)
}

// newWTTJFixture loads the real listing hits (trimmed from a live cybersecurity query,
// 2026-10-15, plus one decoy with its organization slug stripped) and the matching real
// posting-API bodies.
func newWTTJFixture(t *testing.T) *wttjFakeHTTP {
	t.Helper()
	var list struct {
		Hits []json.RawMessage `json:"hits"`
	}
	readJSONFixture(t, "testdata/wttj/list.json", &list)
	f := &wttjFakeHTTP{details: map[string]string{}, detailErr: map[string]error{}}
	for _, raw := range list.Hits {
		var h wttjHit
		if err := json.Unmarshal(raw, &h); err != nil {
			t.Fatal(err)
		}
		ts, err := time.Parse(time.RFC3339, h.PublishedAt)
		if err != nil {
			t.Fatal(err)
		}
		f.hits = append(f.hits, wttjFakeHit{raw: raw, ts: ts.Unix(), hasTS: true})
	}
	for _, name := range []string{"salary", "remote", "multi", "wkit"} {
		b, err := os.ReadFile("testdata/wttj/detail_" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var d struct {
			Job struct {
				Slug string `json:"slug"`
			} `json:"job"`
		}
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		org := map[string]string{"salary": "edf", "remote": "plain-concepts", "multi": "scribd", "wkit": "syspertec-group"}[name]
		f.details[org+"/"+d.Job.Slug] = string(b)
	}
	return f
}

func readJSONFixture(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func wttjByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestWTTJProviderAndMarkers(t *testing.T) {
	s := NewWTTJ(nil)
	if s.Provider() != "wttj" {
		t.Fatalf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("wttj must be an aggregator")
	}
	if _, ok := s.(boardless); ok {
		t.Error("wttj selects its slice by board and must not be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("wttj proves every window's count and must carry fullBoardListing")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("wttj must be CoverageGated")
	}
}

func TestParseWTTJEnv(t *testing.T) {
	c, err := parseWTTJEnv(wttjTestEnv)
	if err != nil {
		t.Fatal(err)
	}
	if c.appID != "CSEKHVMS53" || c.apiKey != "public-search-key-redacted-0000000" {
		t.Errorf("creds = %+v", c)
	}
	for _, bad := range []string{
		"",
		`window.env = {"PUBLIC_ALGOLIA_API_KEY_CLIENT":"k"};`,
		`window.env = {"PUBLIC_ALGOLIA_APPLICATION_ID":"evil.host/x","PUBLIC_ALGOLIA_API_KEY_CLIENT":"k"};`,
	} {
		if _, err := parseWTTJEnv(bad); err == nil {
			t.Errorf("parseWTTJEnv(%q) accepted", bad)
		}
	}
}

func TestWTTJFetchMapsRealPostings(t *testing.T) {
	f := newWTTJFixture(t)
	jobs, err := NewWTTJ(f).Fetch(context.Background(), CompanyEntry{Provider: "wttj", Board: wttjTestBoard})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (the slug-less decoy dropped)", len(jobs))
	}
	by := wttjByID(jobs)
	if _, ok := by["00000000-decoy-0000-0000-000000000000"]; ok {
		t.Error("decoy without an organization slug was ingested")
	}

	edf := by["b4c67a99-a423-4e69-b3ee-8ca88858efc7"]
	if edf.Company != "EDF" || edf.EmploymentType != "internship" || edf.WorkMode != "" {
		t.Errorf("EDF = %+v", edf)
	}
	if edf.URL != "https://www.edf.fr/edf-recrute/offre/detail/2026-184709?idOrigine=35052" {
		t.Errorf("EDF URL = %q, want the employer's own apply URL", edf.URL)
	}
	if edf.SalaryMin == nil || *edf.SalaryMin != 935 || edf.SalaryMax == nil || *edf.SalaryMax != 1682 ||
		edf.SalaryCurrency != "EUR" || edf.SalaryPeriod != "month" {
		t.Errorf("EDF salary = %v-%v %s/%s", edf.SalaryMin, edf.SalaryMax, edf.SalaryCurrency, edf.SalaryPeriod)
	}
	if !slices.Equal(edf.Countries, []string{"fr"}) || edf.Location != "Marseilles-lès-Aubigny, France" {
		t.Errorf("EDF place = %q %v", edf.Location, edf.Countries)
	}

	pc := by["f5e30c4a-56e0-458d-a133-46079b6de0cb"]
	if pc.WorkMode != "remote" || !pc.Remote || pc.Location != "Spain" || pc.ExperienceYearsMin == nil ||
		*pc.ExperienceYearsMin != 5 || pc.URL != "https://apply.workable.com/plainconcepts/j/BE03646D84/" {
		t.Errorf("Plain Concepts = %+v", pc)
	}
	// The recruitment process is part of the body, after description and profile.
	if !strings.Contains(pc.Description, "Phone screen") || !strings.Contains(pc.Description, "SOAR") {
		t.Errorf("Plain Concepts body misses a section: %q", pc.Description)
	}
	if pc.SalaryCurrency != "" || pc.SalaryMin != nil {
		t.Errorf("Plain Concepts states no salary, got %v %s", pc.SalaryMin, pc.SalaryCurrency)
	}

	sc := by["f1a7304c-fb1c-461f-b40e-a8872a1240e2"]
	if sc.Location != "Dallas, United States; Boston, United States; Austin, United States" ||
		!slices.Equal(sc.Countries, []string{"us"}) || sc.SalaryPeriod != "year" || sc.SalaryCurrency != "USD" {
		t.Errorf("Scribd = %q %v %s/%s", sc.Location, sc.Countries, sc.SalaryCurrency, sc.SalaryPeriod)
	}

	// A posting applied to on WTTJ itself names no apply URL: the WTTJ page stays the URL.
	wk := by["85d938ac-d2cf-45dd-bcdd-5671c8999bcc"]
	if wk.URL != "https://www.welcometothejungle.com/en/companies/syspertec-group/jobs/vp-engineering-h-f_saint-cloud" ||
		wk.WorkMode != "hybrid" || wk.Description == "" {
		t.Errorf("wkit posting = %+v", wk)
	}

	for _, j := range jobs {
		if j.PostedAt == nil || j.Description == "" || j.Title == "" {
			t.Errorf("%s incomplete: %+v", j.ExternalID, j)
		}
	}
	if f.headers["Referer"] != wttjReferer || f.headers["X-Algolia-API-Key"] == "" {
		t.Errorf("query headers = %v", f.headers)
	}
	for _, q := range f.queries {
		if q.Get("distinct") != "false" {
			t.Errorf("query without distinct=false: %v", q)
		}
	}
}

func TestWTTJFetchNewSkipsSeenAndCovered(t *testing.T) {
	f := newWTTJFixture(t)
	seen := func(id string) bool { return id == "b4c67a99-a423-4e69-b3ee-8ca88858efc7" }
	var asked []string
	covered := func(companies []string) map[string]bool {
		asked = companies
		return map[string]bool{"Scribd": true}
	}
	jobs, err := wttj{http: f}.FetchNewGated(context.Background(), CompanyEntry{Board: wttjTestBoard}, seen,
		DetailGate{Covered: covered})
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 4 {
		t.Errorf("coverage asked about %v, want the 4 usable employers", asked)
	}
	by := wttjByID(jobs)
	if j := by["b4c67a99-a423-4e69-b3ee-8ca88858efc7"]; !j.SeenRefresh || j.Description != "" {
		t.Errorf("seen posting = %+v, want a body-less SeenRefresh", j)
	}
	if j, ok := by["f1a7304c-fb1c-461f-b40e-a8872a1240e2"]; !ok || j.Description != "" || j.SeenRefresh {
		t.Errorf("covered posting = %+v, want yielded body-less for the gate", j)
	}
	want := []string{"syspertec-group/vp-engineering-h-f_saint-cloud", "plain-concepts/agentic-detection-response-specialist_es_iri2qklj"}
	if !slices.Equal(f.detailsAsked, want) {
		t.Errorf("detail requests = %v, want only the new uncovered postings", f.detailsAsked)
	}
}

func TestWTTJDetailFailuresDropTheNewPosting(t *testing.T) {
	f := newWTTJFixture(t)
	// Gone: the posting API answers 404.
	delete(f.details, "edf/stage-bac-5-ingenieur-informatique-ingenierie-cybersecurite-f-h_marseille")
	// Unreadable: a 5xx defers the posting rather than storing a body-less row.
	f.detailErr["scribd/staff-cloud-security-engineer_san-francisco_e5e2yuhv"] =
		&StatusError{Method: http.MethodGet, Code: http.StatusServiceUnavailable}
	// A slug answering for a different posting must not lend it its body.
	f.details["syspertec-group/vp-engineering-h-f_saint-cloud"] = strings.Replace(
		f.details["syspertec-group/vp-engineering-h-f_saint-cloud"],
		"85d938ac-d2cf-45dd-bcdd-5671c8999bcc", "11111111-1111-1111-1111-111111111111", 1)

	jobs, err := NewWTTJ(f).Fetch(context.Background(), CompanyEntry{Board: wttjTestBoard})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "f5e30c4a-56e0-458d-a133-46079b6de0cb" {
		t.Errorf("jobs = %v, want only the posting whose detail read cleanly", wttjByID(jobs))
	}
}

// wttjSyntheticHits clones a real hit n times under distinct ids, spreading their timestamps
// over span seconds from start (span 1 puts all of them in one second).
func wttjSyntheticHits(t *testing.T, f *wttjFakeHTTP, n int, start, span int64) []wttjFakeHit {
	t.Helper()
	var base map[string]any
	if err := json.Unmarshal(f.hits[1].raw, &base); err != nil {
		t.Fatal(err)
	}
	out := make([]wttjFakeHit, 0, n)
	for i := range n {
		base["objectID"] = fmt.Sprintf("synthetic-%05d", i)
		raw, _ := json.Marshal(base)
		out = append(out, wttjFakeHit{raw: raw, ts: start + int64(i)%span, hasTS: true})
	}
	return out
}

func TestWTTJListSplitsWindowsPastTheCap(t *testing.T) {
	f := newWTTJFixture(t)
	f.hits = wttjSyntheticHits(t, f, 2600, 1_700_000_000, 90*24*3600)
	hits, err := wttj{http: f}.list(context.Background(), wttjTestBoard)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2600 {
		t.Fatalf("listed %d of 2600 postings", len(hits))
	}
	for _, q := range f.queries {
		if n, _ := strconv.Atoi(q.Get("hitsPerPage")); n > wttjHitsCap {
			t.Fatalf("asked for %d hits in one query", n)
		}
	}
}

func TestWTTJListFailsInsteadOfTruncating(t *testing.T) {
	cases := map[string]func(t *testing.T, f *wttjFakeHTTP) string{
		"more than the cap in one second": func(t *testing.T, f *wttjFakeHTTP) string {
			f.hits = wttjSyntheticHits(t, f, 1001, 1_700_000_000, 1)
			return wttjTestBoard
		},
		"a posting outside every window": func(t *testing.T, f *wttjFakeHTTP) string {
			f.hits[0].hasTS = false
			return wttjTestBoard
		},
		"a later window query fails": func(t *testing.T, f *wttjFakeHTTP) string {
			f.hits = wttjSyntheticHits(t, f, 2600, 1_700_000_000, 90*24*3600)
			f.failQuery = 5
			return wttjTestBoard
		},
		"a board that matches nothing": func(_ *testing.T, _ *wttjFakeHTTP) string {
			return "renamed-sub-category-xXxXx"
		},
		"a board that is not a reference": func(_ *testing.T, _ *wttjFakeHTTP) string {
			return `x" OR organization.name:"y`
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newWTTJFixture(t)
			board := setup(t, f)
			jobs, err := NewWTTJ(f).Fetch(context.Background(), CompanyEntry{Board: board})
			if err == nil {
				t.Fatalf("Fetch succeeded with %d jobs; a walk without proof must fail", len(jobs))
			}
			var se *StatusError
			if name == "a later window query fails" && !errors.As(err, &se) {
				t.Errorf("error %v does not wrap the query failure", err)
			}
		})
	}
}
