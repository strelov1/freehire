package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// arcFake serves the real listing and detail fixtures (trimmed from responses fetched
// 2026-10-30, the signed-in user's own fields dropped). A detail key with no fixture answers
// what Arc answers for an unknown posting: a redirect to the bare listing.
type arcFake struct {
	lists   map[string]string // job_type → listing body
	details map[string]string // random key → detail page
	fail    map[string]error  // job_type or random key → error

	mu        sync.Mutex
	posted    []map[string]any
	headers   []map[string]string
	requested []string
}

var arcDetailRE = regexp.MustCompile(`/remote-jobs/j/[a-z0-9-]*-([a-z0-9]+)$`)

func (f *arcFake) PostJSONWithHeaders(_ context.Context, u string, headers map[string]string, body, v any) error {
	if u != arcSearchURL {
		return fmt.Errorf("arcFake: no POST route for %s", u)
	}
	raw, _ := json.Marshal(body)
	var req map[string]any
	_ = json.Unmarshal(raw, &req)
	f.mu.Lock()
	f.posted = append(f.posted, req)
	f.headers = append(f.headers, headers)
	f.mu.Unlock()
	jobType, _ := req["job_type"].(string)
	if err := f.fail[jobType]; err != nil {
		return err
	}
	list, ok := f.lists[jobType]
	if !ok {
		return fmt.Errorf("arcFake: no listing for %q", jobType)
	}
	// The fixtures hold one page; every later page answers empty, as the API does.
	if page, _ := req["page"].(float64); page > 1 {
		var p struct {
			Total int `json:"total_job_count"`
		}
		_ = json.Unmarshal([]byte(list), &p)
		list = fmt.Sprintf(`{"total_job_count":%d,"jobs":[]}`, p.Total)
	}
	return json.Unmarshal([]byte(list), v)
}

func (f *arcFake) GetHTMLResolved(_ context.Context, u string) (*html.Node, string, error) {
	m := arcDetailRE.FindStringSubmatch(u)
	if m == nil {
		return nil, "", fmt.Errorf("arcFake: no GET route for %s", u)
	}
	key := m[1]
	f.mu.Lock()
	f.requested = append(f.requested, key)
	f.mu.Unlock()
	if err := f.fail[key]; err != nil {
		return nil, "", err
	}
	page, ok := f.details[key]
	if !ok {
		root, _ := html.Parse(strings.NewReader(`<html><body>listing</body></html>`))
		return root, "https://arc.dev/remote-jobs", nil
	}
	root, err := html.Parse(strings.NewReader(page))
	return root, u, err
}

func (f *arcFake) detailCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.requested)
	slices.Sort(out)
	return out
}

func arcFixtures(t *testing.T) *arcFake {
	t.Helper()
	read := func(name string) string {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return &arcFake{
		lists: map[string]string{
			"fulltime":  read("arc_list_fulltime.json"),
			"freelance": read("arc_list_freelance.json"),
		},
		details: map[string]string{
			"pvbgk22pxc": read("arc_detail_pvbgk22pxc.html"),
			"pvbutr8j4z": read("arc_detail_pvbutr8j4z.html"),
		},
		fail: map[string]error{},
	}
}

func arcByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestArcListsBothJobTypesAndHydratesNewPostings(t *testing.T) {
	f := arcFixtures(t)
	jobs, err := NewArc(f, "session=abc").(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	got := arcByID(jobs)
	// Two of the four listed postings have a detail page; the other two answer Arc's
	// redirect for a posting it no longer carries, and a gone posting is not yielded.
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2: %v", len(got), slices.Collect(maps.Keys(got)))
	}

	k := got["pvbgk22pxc"]
	if k.Title != "Senior AI Software Engineer, Agentic Systems & Data Fabric" || k.Company != "Kinaxis" {
		t.Errorf("identity = %q at %q", k.Title, k.Company)
	}
	if k.URL != "https://arc.dev/remote-jobs/j/kinaxis-senior-ai-software-engineer-agentic-systems-data-fabric-pvbgk22pxc" {
		t.Errorf("URL = %q", k.URL)
	}
	if !slices.Equal(k.Countries, []string{"ca"}) {
		t.Errorf("Countries = %v, want [ca]", k.Countries)
	}
	if k.WorkMode != "remote" || !k.Remote {
		t.Errorf("work mode = %q remote=%v, want remote", k.WorkMode, k.Remote)
	}
	if k.EmploymentType != "full_time" || k.Seniority != "senior" {
		t.Errorf("employment = %q seniority = %q", k.EmploymentType, k.Seniority)
	}
	if k.PostedAt == nil || !k.PostedAt.Equal(time.Unix(1793316305, 0)) {
		t.Errorf("PostedAt = %v", k.PostedAt)
	}
	if !strings.Contains(k.Description, "<strong>About Kinaxis") || strings.Contains(k.Description, "raw HTML omitted") {
		t.Errorf("description lost its inline markup: %.200q", k.Description)
	}
	if !strings.Contains(k.Description, "<p>") {
		t.Errorf("description was not rendered from Markdown: %.200q", k.Description)
	}
	// LinkedIn's bodies close a bold run AFTER its paragraph break ("<strong>About X  \n
	// \n</strong>Text"); rendered as is, the paragraph closes inside the bold run.
	if !strings.HasPrefix(k.Description, "<p><strong>About Kinaxis</strong></p>") || strings.Contains(k.Description, "<p></strong>") {
		t.Errorf("bold run split across paragraphs: %.200q", k.Description)
	}

	if c := got["pvbutr8j4z"]; c.EmploymentType != "contract" || c.Company != "Kelly Services" {
		t.Errorf("freelance posting = %q at %q", c.EmploymentType, c.Company)
	}
}

// The session cookie rides the listing only: the detail pages are public, and a cookie sent
// where none is needed is a cookie leaked to whatever answers there.
func TestArcSendsTheSessionOnlyToTheListing(t *testing.T) {
	f := arcFixtures(t)
	if _, err := NewArc(f, "session=abc").Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatal(err)
	}
	if len(f.headers) == 0 {
		t.Fatal("no listing request")
	}
	for _, h := range f.headers {
		if h["Cookie"] != "session=abc" || h["X-Requested-From"] != "arc-next" || h["Origin"] != "https://arc.dev" {
			t.Errorf("listing headers = %v", h)
		}
	}
	for _, req := range f.posted {
		if req["per_page"] != float64(arcPageSize) || req["sort_by"] != "latest" {
			t.Errorf("listing body = %v", req)
		}
	}
}

func TestArcRefreshesSeenPostingsWithoutADetailRequest(t *testing.T) {
	f := arcFixtures(t)
	seen := func(id string) bool { return id != "pvbgk22pxc" }
	jobs, err := NewArc(f, "s").(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if calls := f.detailCalls(); !slices.Equal(calls, []string{"pvbgk22pxc"}) {
		t.Errorf("detail calls = %v, want only the unseen posting", calls)
	}
	got := arcByID(jobs)
	if len(got) != 4 {
		t.Fatalf("got %d jobs, want 4", len(got))
	}
	for id, j := range got {
		if want := id != "pvbgk22pxc"; j.SeenRefresh != want {
			t.Errorf("%s SeenRefresh = %v, want %v", id, j.SeenRefresh, want)
		}
	}
}

// The provider is swept by source, so a listing that read fewer postings than Arc states
// must fail rather than let the sweep close what the walk never reached.
func TestArcFailsAListingShortOfItsStatedTotal(t *testing.T) {
	f := arcFixtures(t)
	f.lists["freelance"] = strings.Replace(f.lists["freelance"], `"total_job_count": 1`, `"total_job_count": 2`, 1)
	_, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{})
	if err == nil || !strings.Contains(err.Error(), "1 distinct postings of 2") {
		t.Fatalf("err = %v, want a short-listing failure", err)
	}
}

func TestArcFailsAnEmptyCatalogue(t *testing.T) {
	f := arcFixtures(t)
	f.lists["fulltime"] = `{"total_job_count":0,"jobs":[]}`
	if _, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an empty full-time catalogue passed")
	}
}

// An expired session answers 401. It must fail the crawl and say which credential to
// renew, never read as a quiet day.
func TestArcFailsLoudlyOnAnExpiredSession(t *testing.T) {
	f := arcFixtures(t)
	f.fail["fulltime"] = &StatusError{Method: "POST", Code: 401, URL: arcSearchURL}
	_, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{})
	if err == nil || !strings.Contains(err.Error(), arcSessionEnv) {
		t.Fatalf("err = %v, want a failure naming %s", err, arcSessionEnv)
	}
}

// A detail that could not be read defers its posting: stored body-less, it would never be
// hydrated again. A run in which no detail answered at all is broken, not quiet.
func TestArcDefersUnreadableDetailsAndFailsWhenNoneAnswer(t *testing.T) {
	f := arcFixtures(t)
	f.fail["pvbgk22pxc"] = errors.New("timeout")
	jobs, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := arcByID(jobs)["pvbgk22pxc"]; ok {
		t.Error("an unreadable posting was yielded")
	}

	f = arcFixtures(t)
	for _, k := range []string{"pvbgk22pxc", "pvbutr8j4z", "pvbgcmy1fa", "p8c2klll2v"} {
		f.fail[k] = errors.New("timeout")
	}
	if _, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a run where no detail answered passed")
	}
}

func TestArcDropsAClosedPosting(t *testing.T) {
	f := arcFixtures(t)
	f.details["pvbgk22pxc"] = strings.Replace(f.details["pvbgk22pxc"], `"closed": false`, `"closed": true`, 1)
	jobs, err := NewArc(f, "s").Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := arcByID(jobs)["pvbgk22pxc"]; ok {
		t.Error("a posting Arc marks closed was yielded")
	}
}

func TestArcGatedSkipsTheDetailOfACoveredEmployer(t *testing.T) {
	f := arcFixtures(t)
	gate := DetailGate{
		Covered: func(names []string) map[string]bool {
			return map[string]bool{"Kinaxis": slices.Contains(names, "Kinaxis")}
		},
	}
	jobs, err := NewArc(f, "s").(CoverageGated).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.detailCalls(), "pvbgk22pxc") {
		t.Error("bought the detail of a covered employer's posting")
	}
	k, ok := arcByID(jobs)["pvbgk22pxc"]
	if !ok || k.Description != "" {
		t.Errorf("covered posting = %+v, want it yielded body-less", k)
	}
}

func TestArcSeniority(t *testing.T) {
	for _, tc := range []struct {
		levels []string
		want   string
	}{
		{[]string{"senior"}, "senior"},
		{[]string{"junior"}, "junior"},
		{[]string{"staff"}, "staff"},
		{[]string{"principal"}, "principal"},
		{[]string{"lead"}, "lead"},
		{[]string{"cto"}, "c_level"},
		{[]string{"senior", "senior"}, "senior"},
		// A seniority-less role name, or two levels at once, states no single level.
		{[]string{"manager"}, ""},
		{[]string{"director"}, ""},
		{[]string{"senior", "lead"}, ""},
		{nil, ""},
	} {
		if got := arcSeniority(tc.levels); got != tc.want {
			t.Errorf("arcSeniority(%v) = %q, want %q", tc.levels, got, tc.want)
		}
	}
}

// Arc derives required_countries from LinkedIn's "City, ST, United States", so beside "US" a
// state abbreviation arrives as a country code: ["US","IL"] is Illinois, not Israel. Such a
// list is demoted to the fallback and stripped of every code a state could have produced.
func TestArcCountries(t *testing.T) {
	for _, tc := range []struct {
		codes              []string
		countries, fallbck []string
	}{
		{[]string{"CA"}, []string{"ca"}, nil},
		{[]string{"US"}, []string{"us"}, nil},
		{[]string{"CA", "GB"}, []string{"ca", "gb"}, nil},
		{[]string{"PL", "GB", "US"}, []string{"pl", "gb", "us"}, nil},
		// DE is Delaware as well as Germany, so even an international-looking list is demoted.
		{[]string{"DE", "PL", "GB", "US"}, nil, []string{"pl", "gb", "us"}},
		{[]string{"US", "IL"}, nil, []string{"us"}},
		{[]string{"US", "VA"}, nil, []string{"us"}},
		{[]string{"US", "CA", "GB"}, nil, []string{"us", "gb"}},
		{[]string{"US", "GE", "JE"}, nil, []string{"us"}},
		{nil, nil, nil},
	} {
		c, f := arcCountries(tc.codes)
		if !slices.Equal(c, tc.countries) || !slices.Equal(f, tc.fallbck) {
			t.Errorf("arcCountries(%v) = %v, %v; want %v, %v", tc.codes, c, f, tc.countries, tc.fallbck)
		}
	}
}
