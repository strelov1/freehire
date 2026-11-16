package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// herkeyFake serves the real session, listing and detail fixtures (trimmed from responses
// fetched 2026-10-18). A detail id with no fixture answers the platform's 400; ids in fail
// answer the given error instead.
type herkeyFake struct {
	session string
	pages   map[string]string // page_no → listing page
	details map[string]string
	fail    map[string]error

	mu        sync.Mutex
	headers   map[string]map[string]string
	requested []string
}

var (
	herkeyDetailRE = regexp.MustCompile(`/jobs/jobs/(\d+)$`)
	herkeyPageRE   = regexp.MustCompile(`page_no=(\d+)`)
)

func (f *herkeyFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.mu.Lock()
	if f.headers == nil {
		f.headers = map[string]map[string]string{}
	}
	f.headers[u] = headers
	f.mu.Unlock()
	if err := f.fail[u]; err != nil {
		return err
	}
	switch {
	case u == herkeySessionURL:
		return json.Unmarshal([]byte(f.session), v)
	case herkeyDetailRE.MatchString(u):
		id := herkeyDetailRE.FindStringSubmatch(u)[1]
		f.mu.Lock()
		f.requested = append(f.requested, id)
		f.mu.Unlock()
		if err := f.fail[id]; err != nil {
			return err
		}
		raw, ok := f.details[id]
		if !ok {
			return &StatusError{Method: "GET", Code: 400, URL: u}
		}
		return json.Unmarshal([]byte(raw), v)
	case strings.Contains(u, "es_candidate_jobs"):
		page := herkeyPageRE.FindStringSubmatch(u)[1]
		raw, ok := f.pages[page]
		if !ok {
			return fmt.Errorf("herkeyFake: no listing page %s", page)
		}
		return json.Unmarshal([]byte(raw), v)
	}
	return fmt.Errorf("herkeyFake: no route for %s", u)
}

func (f *herkeyFake) detailCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.requested)
	slices.Sort(out)
	return out
}

func herkeyFixtures(t *testing.T) *herkeyFake {
	t.Helper()
	read := func(name string) string {
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	f := &herkeyFake{
		session: `{"status_code":200,"success":true,"body":{"session_id":"test-session"}}`,
		pages:   map[string]string{"1": read("herkey_list_page1.json"), "2": read("herkey_list_page2.json")},
		details: map[string]string{},
		fail:    map[string]error{},
	}
	for _, id := range []string{"194415", "193866", "194409", "194271", "179885"} {
		f.details[id] = read("herkey_detail_" + id + ".json")
	}
	return f
}

func herkeyByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestHerKeyRegistrationAndMarkers(t *testing.T) {
	s := NewHerKey(nil)
	if s.Provider() != "herkey" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	for name, ok := range map[string]bool{
		"boardless":       func() bool { _, ok := s.(boardless); return ok }(),
		"aggregator":      func() bool { _, ok := s.(aggregator); return ok }(),
		"fullCatalog":     func() bool { _, ok := s.(fullCatalog); return ok }(),
		"HydratingSource": func() bool { _, ok := s.(HydratingSource); return ok }(),
		"CoverageGated":   func() bool { _, ok := s.(CoverageGated); return ok }(),
	} {
		if !ok {
			t.Errorf("herkey should implement %s", name)
		}
	}
	if _, ok := All(nil)["herkey"]; !ok {
		t.Error("All() should register herkey")
	}
	if !slices.Contains(FilterableProviders(), "herkey") {
		t.Error("FilterableProviders() should include herkey")
	}
}

func TestHerKeyFetchMapsTheRealFixtures(t *testing.T) {
	f := herkeyFixtures(t)
	jobs, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The listing goes out with the anonymous session the site's own frontend mints.
	if got := f.headers[fmt.Sprintf(herkeyListURL, 1, herkeyPageSize)]["Authorization"]; got != "Token test-session" {
		t.Errorf("listing Authorization = %q, want the minted session", got)
	}
	got := herkeyByID(jobs)
	// 179885 is still in the index, but its posting resource reports it inactive (a JobsPikr
	// copy past its expiry), so it is gone.
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (the inactive posting dropped): %v", len(jobs), got)
	}
	if _, ok := got["179885"]; ok {
		t.Error("inactive posting 179885 was yielded")
	}

	native := got["194415"]
	if native.Title != "Project Manager" || native.Company != "IT Exchange" {
		t.Errorf("194415 identity: %+v", native)
	}
	if native.URL != "https://www.herkey.com/jobs/project-manager/194415" {
		t.Errorf("194415 URL = %q, want the HerKey page (no external link)", native.URL)
	}
	if native.WorkMode != "remote" || !native.Remote || native.Location != "" {
		t.Errorf("194415 WorkMode=%q Remote=%v Location=%q, want remote/true/\"\" (\"Any\" is no place)",
			native.WorkMode, native.Remote, native.Location)
	}
	if native.EmploymentType != "full_time" || native.ExperienceYearsMin == nil || *native.ExperienceYearsMin != 8 {
		t.Errorf("194415 EmploymentType=%q ExperienceYearsMin=%v", native.EmploymentType, native.ExperienceYearsMin)
	}
	// The body joins description, responsibilities and requirements.
	for _, want := range []string{"ITeXchange", "Desired Skills and Experience", "high aptitude"} {
		if !strings.Contains(native.Description, want) {
			t.Errorf("194415 Description missing %q", want)
		}
	}
	if native.PostedAt == nil || !native.PostedAt.Equal(time.Date(2026, 10, 14, 5, 16, 56, 0, time.UTC)) {
		t.Errorf("194415 PostedAt = %v, want published_on read as IST", native.PostedAt)
	}

	// Aristocrat's link is a recruitics redirect around its own Workday posting.
	aristocrat := got["193866"]
	if !strings.HasPrefix(aristocrat.URL, "https://aristocrat.wd3.myworkdayjobs.com/AristocratExternalCareersSite/job/") {
		t.Errorf("193866 URL = %q, want the unwrapped Workday link", aristocrat.URL)
	}
	if aristocrat.WorkMode != "" || aristocrat.ExperienceYearsMin != nil {
		t.Errorf("193866 WorkMode=%q ExperienceYearsMin=%v, want unstated", aristocrat.WorkMode, aristocrat.ExperienceYearsMin)
	}

	// The pharma careers site's body is plain text and is rebuilt into paragraphs.
	cadila := got["194409"]
	if cadila.URL != "https://careers.cadilapharma.com/in/en/job/CPJCPHIN25429EXTERNALENIN/Field-Officer" {
		t.Errorf("194409 URL = %q", cadila.URL)
	}
	if !strings.Contains(cadila.Description, "<p>Role Objective / Major Purpose of the Job<br>") {
		t.Errorf("194409 Description not rebuilt from plain text: %q", cadila.Description)
	}

	if office := got["194271"]; office.WorkMode != "onsite" || office.Location != "Gurgaon" {
		t.Errorf("194271 WorkMode=%q Location=%q, want onsite/Gurgaon", office.WorkMode, office.Location)
	}
}

func TestHerKeySeenPostingsCostNoDetail(t *testing.T) {
	f := herkeyFixtures(t)
	seen := map[string]bool{"194415": true, "193866": true}
	jobs, err := herkey{http: f}.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if calls := f.detailCalls(); !slices.Equal(calls, []string{"179885", "194271", "194409"}) {
		t.Errorf("detail calls = %v, want only the unseen postings", calls)
	}
	got := herkeyByID(jobs)
	if j := got["194415"]; !j.SeenRefresh || j.Description != "" || j.Company != "IT Exchange" {
		t.Errorf("seen 194415 = %+v, want a body-less SeenRefresh", j)
	}
}

func TestHerKeyGatedPostingsCostNoDetail(t *testing.T) {
	f := herkeyFixtures(t)
	gate := DetailGate{
		Covered: func(names []string) map[string]bool {
			return map[string]bool{"Aristocrat": true, "Cadila Pharma": true}
		},
	}
	jobs, err := herkey{http: f}.FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if calls := f.detailCalls(); !slices.Equal(calls, []string{"179885", "194271", "194415"}) {
		t.Errorf("detail calls = %v, want none for the covered postings", calls)
	}
	got := herkeyByID(jobs)
	for _, id := range []string{"193866", "194409"} {
		if j, ok := got[id]; !ok || j.Description != "" {
			t.Errorf("gated %s = %+v (present %v), want yielded body-less for the pipeline to count", id, j, ok)
		}
	}
}

func TestHerKeyFailsAnIncompleteListing(t *testing.T) {
	t.Run("later page fails", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.fail[fmt.Sprintf(herkeyListURL, 2, herkeyPageSize)] = &StatusError{Code: 502}
		if _, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Error("a failed second page must fail the crawl: the provider is swept by source")
		}
	})
	t.Run("short of the total", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.pages["1"] = strings.Replace(f.pages["1"], `"total_items": 5`, `"total_items": 6`, 1)
		f.pages["2"] = strings.Replace(f.pages["2"], `"total_items": 5`, `"total_items": 6`, 1)
		if _, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{}); err == nil ||
			!strings.Contains(err.Error(), "5 distinct postings of 6") {
			t.Errorf("err = %v, want the short walk reported", err)
		}
	})
	t.Run("total moves mid-walk", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.pages["2"] = strings.Replace(f.pages["2"], `"total_items": 5`, `"total_items": 7`, 1)
		if _, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Error("a total that moves mid-walk must fail: offsets shifted under the walk")
		}
	})
	t.Run("no session", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.session = `{"status_code":200,"success":true,"body":{}}`
		if _, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Error("a session answer without an id must fail the crawl")
		}
	})
}

func TestHerKeyDetailAnswers(t *testing.T) {
	t.Run("an unreadable detail defers only its posting", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.fail["194271"] = &StatusError{Code: 503}
		jobs, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if _, ok := herkeyByID(jobs)["194271"]; ok || len(jobs) != 3 {
			t.Errorf("got %d jobs, want 3 with 194271 deferred", len(jobs))
		}
	})
	t.Run("a 400 is the platform's answer that the id is gone", func(t *testing.T) {
		f := herkeyFixtures(t)
		delete(f.details, "194415")
		jobs, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{})
		if err != nil || len(jobs) != 3 {
			t.Errorf("got %d jobs, err %v; want 3 and no error", len(jobs), err)
		}
	})
	t.Run("a placeholder body is no posting", func(t *testing.T) {
		f := herkeyFixtures(t)
		f.details["194409"] = regexp.MustCompile(`"description": "(?:[^"\\]|\\.)*"`).
			ReplaceAllString(f.details["194409"], `"description": "\"TBD\""`)
		jobs, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if _, ok := herkeyByID(jobs)["194409"]; ok {
			t.Error(`a "TBD" body must not be stored as the posting's description`)
		}
	})
	t.Run("no detail answering fails the crawl", func(t *testing.T) {
		f := herkeyFixtures(t)
		for _, id := range []string{"194415", "193866", "194409", "194271", "179885"} {
			f.fail[id] = &StatusError{Code: 503}
		}
		if _, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Error("a crawl that listed new postings and read none must fail")
		}
	})
	t.Run("only expired postings new still passes", func(t *testing.T) {
		f := herkeyFixtures(t)
		_, err := herkey{http: f}.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != "179885" })
		if err != nil {
			t.Errorf("a run whose only new row the platform reports expired must pass: %v", err)
		}
	})
}

func TestHerKeyDetailBudgetBoundsARun(t *testing.T) {
	const n = herkeyDetailBudget + 25
	rows := make([]herkeyListing, n)
	for i := range rows {
		rows[i] = herkeyListing{ID: int64(i + 1), Title: "Software Engineer", CompanyName: "Acme"}
	}
	page, _ := json.Marshal(map[string]any{
		"pagination": map[string]any{"total_items": n, "has_next": false},
		"body":       rows,
	})
	f := &herkeyFake{
		session: `{"body":{"session_id":"s"}}`,
		pages:   map[string]string{"1": string(page)},
		details: map[string]string{},
	}
	for i := 1; i <= n; i++ {
		f.details[fmt.Sprint(i)] = fmt.Sprintf(`{"body":[{"id":%d,"status":"active","description":"<p>Build things.</p>"}]}`, i)
	}
	jobs, err := NewHerKey(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if calls := len(f.detailCalls()); calls != herkeyDetailBudget || len(jobs) != herkeyDetailBudget {
		t.Errorf("detail calls %d, jobs %d; want both %d", calls, len(jobs), herkeyDetailBudget)
	}
}

func TestHerKeyPicklists(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"full time"}, "full_time"},
		{[]string{"full_time", "returnee_program"}, "full_time"},
		{[]string{"freelance"}, "contract"},
		{[]string{"full_time", "part_time"}, ""},
		{[]string{""}, ""},
	} {
		if got := herkeyEmploymentType(c.in); got != c.want {
			t.Errorf("herkeyEmploymentType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"work_from_home"}, "remote"},
		{[]string{"hybrid"}, "hybrid"},
		{[]string{"work_from_home", "hybrid"}, ""},
		{nil, ""},
	} {
		if got := herkeyWorkMode(c.in); got != c.want {
			t.Errorf("herkeyWorkMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHerKeySharedLinkKeepsTheHerKeyPage(t *testing.T) {
	r := herkeyListing{ID: 7, Title: "Analyst", CompanyName: "FIS Global", RedirectURL: "https://careers.fisglobal.com/us/en/job/JR1"}
	j, ok := r.toJob(map[string]int{"https://careers.fisglobal.com/us/en/job/JR1": 2})
	if !ok || j.URL != "https://www.herkey.com/jobs/analyst/7" {
		t.Errorf("URL = %q, want the HerKey page for a link two postings share", j.URL)
	}
}
