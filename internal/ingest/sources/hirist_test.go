package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// hiristFake serves the real fixtures (fetched 2026-10-25: the first three rows of hirist.tech's
// category 16, Full Stack, and the posting API's answer for two of them, trimmed to the fields
// the adapter reads). A posting id with no fixture answers the API's 404 JOB_NOT_FOUND, and ids
// in fail answer the given error instead.
type hiristFake struct {
	pages map[int]string
	fail  map[string]error

	mu        sync.Mutex
	headers   []map[string]string
	requested []string
}

var (
	hiristCategoryRE = regexp.MustCompile(`^https://gladiator\.hirist\.tech/job/category/\?categoryId=16&page=(\d+)&size=100$`)
	hiristDetailRE   = regexp.MustCompile(`^https://gladiator\.hirist\.tech/job/detail\?jobcode=(\d+)$`)
)

func (f *hiristFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.headers = append(f.headers, headers)
	f.mu.Unlock()
	if m := hiristDetailRE.FindStringSubmatch(u); m != nil {
		id := m[1]
		f.mu.Lock()
		f.requested = append(f.requested, id)
		f.mu.Unlock()
		if err := f.fail[id]; err != nil {
			return err
		}
		raw, err := os.ReadFile("testdata/hirist/detail_" + id + ".json")
		if err != nil {
			return &StatusError{Method: "GET", Code: http.StatusNotFound, URL: u}
		}
		return json.Unmarshal(raw, v)
	}
	if m := hiristCategoryRE.FindStringSubmatch(u); m != nil {
		if err := f.fail["page"+m[1]]; err != nil {
			return err
		}
		page, err := strconv.Atoi(m[1])
		if err != nil {
			return err
		}
		raw, ok := f.pages[page]
		if !ok {
			return fmt.Errorf("hiristFake: no category page %d", page)
		}
		return json.Unmarshal([]byte(raw), v)
	}
	return fmt.Errorf("hiristFake: no route for %s", u)
}

func (f *hiristFake) detailCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.requested)
	slices.Sort(out)
	return out
}

func hiristCategoryFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/hirist/category.json")
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is the first page of an 830-page walk; one page is the whole category here.
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	p["hasMore"] = false
	out, _ := json.Marshal(p)
	return string(out)
}

func TestHiristFetchHydratesNewPostings(t *testing.T) {
	fake := &hiristFake{pages: map[int]string{0: hiristCategoryFixture(t)}}
	src := NewHirist(fake).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "16"}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	// 1678906 has no posting fixture, so the API answers 404: the posting is gone, not stored.
	if got, want := fake.detailCalls(), []string{"1678906", "1679150", "1679176"}; !slices.Equal(got, want) {
		t.Fatalf("detail calls = %v, want %v", got, want)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	h := byID["1679176"]
	if h.Title != "Senior Director" || h.Company != "The Hartford" {
		t.Errorf("title/company = %q / %q", h.Title, h.Company)
	}
	if h.URL != "https://www.hirist.tech/j/senior-director-1679176" {
		t.Errorf("url = %q", h.URL)
	}
	if h.Location != "Hyderabad; Telangana" || h.WorkMode != "" {
		t.Errorf("location/mode = %q / %q", h.Location, h.WorkMode)
	}
	if !h.IsTechHint || !slices.Equal(h.FallbackCountries, []string{"in"}) {
		t.Errorf("tech hint %v, fallback countries %v", h.IsTechHint, h.FallbackCountries)
	}
	if h.ExperienceYearsMin == nil || *h.ExperienceYearsMin != 18 {
		t.Errorf("experience min = %v, want 18", h.ExperienceYearsMin)
	}
	if h.SalaryMin != nil {
		t.Errorf("a hidden salary was published: %v", *h.SalaryMin)
	}
	if h.PostedAt == nil || h.PostedAt.UnixMilli() != 1792385495193 {
		t.Errorf("posted at = %v", h.PostedAt)
	}
	if !strings.Contains(h.Description, "Sr. Director - Engineering") || strings.Contains(h.Description, "<script") {
		t.Errorf("description = %.120q", h.Description)
	}
	e := byID["1679150"]
	if e.Title != "Manager/Senior Manager - Full Stack & Generative AI" {
		t.Errorf("a title whose head is not the employer lost it: %q", e.Title)
	}
	if e.Location != "Gurgaon/Gurugram; Bangalore" {
		t.Errorf("location = %q", e.Location)
	}
	// The listing is read with the web app's version header; the posting API needs none.
	if fake.headers[0]["version"] != "2" {
		t.Errorf("listing headers = %v", fake.headers[0])
	}
}

func TestHiristSeenPostingIsRefreshedWithoutDetail(t *testing.T) {
	fake := &hiristFake{pages: map[int]string{0: hiristCategoryFixture(t)}}
	src := NewHirist(fake).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{Board: "16"}, func(id string) bool { return id != "1679150" })
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.detailCalls(); !slices.Equal(got, []string{"1679150"}) {
		t.Fatalf("detail calls = %v, want only the unseen posting", got)
	}
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.Description != "" {
				t.Errorf("a seen refresh carried a body")
			}
		}
	}
	if refreshed != 2 || len(jobs) != 3 {
		t.Errorf("got %d jobs with %d refreshes, want 3 with 2", len(jobs), refreshed)
	}
}

func TestHiristFailsWhenNoDetailAnswers(t *testing.T) {
	down := errors.New("connection reset")
	fake := &hiristFake{
		pages: map[int]string{0: hiristCategoryFixture(t)},
		fail:  map[string]error{"1679176": down, "1679150": down, "1678906": down},
	}
	if _, err := NewHirist(fake).Fetch(context.Background(), CompanyEntry{Board: "16"}); err == nil {
		t.Fatal("a crawl whose every detail failed reported success")
	}
}

func TestHiristFirstPageFailureFailsTheBoard(t *testing.T) {
	fake := &hiristFake{fail: map[string]error{"page0": errors.New("503")}}
	if _, err := NewHirist(fake).Fetch(context.Background(), CompanyEntry{Board: "16"}); err == nil {
		t.Fatal("an unreachable category reported success")
	}
}

func TestHiristRejectsANonNumericBoard(t *testing.T) {
	if _, err := NewHirist(&hiristFake{}).Fetch(context.Background(), CompanyEntry{Board: "backend"}); err == nil {
		t.Fatal("a non-numeric board was accepted")
	}
}

func TestHiristBoardShareDefersTheRest(t *testing.T) {
	rows := make([]map[string]any, 0, hiristBoardDetailShare+5)
	for i := range hiristBoardDetailShare + 5 {
		rows = append(rows, map[string]any{
			"id": 2000000 + i, "title": "Backend Engineer", "createdTimeMs": 1792000000000 + int64(i),
			"companyData": map[string]any{"companyName": "Acme"},
		})
	}
	page, _ := json.Marshal(map[string]any{"data": rows, "hasMore": false})
	fake := &hiristFake{pages: map[int]string{0: string(page)}}
	if _, err := NewHirist(fake).Fetch(context.Background(), CompanyEntry{Board: "16"}); err != nil {
		t.Fatal(err)
	}
	calls := fake.detailCalls()
	if len(calls) != hiristBoardDetailShare {
		t.Fatalf("asked %d details, want the board share %d", len(calls), hiristBoardDetailShare)
	}
	// Newest first: the five oldest postings are the ones deferred.
	if slices.Contains(calls, "2000004") || !slices.Contains(calls, "2000005") || !slices.Contains(calls, fmt.Sprint(2000000+hiristBoardDetailShare+4)) {
		t.Errorf("the budget did not buy the newest postings first: %v", calls[:3])
	}
}

func TestHiristToJob(t *testing.T) {
	s := NewIIMJobs(nil).(hirist)
	row := hiristListing{
		ID: 1741769, Title: "Angel One - Lead - US Stocks", MinExp: 3, MaxExp: 5, MinSal: 25, MaxSal: 37.5,
		Locations: []hiristLocation{{Name: "Remote"}},
	}
	row.CompanyData.CompanyName = "Angel One"
	j, ok := s.toJob(row, false)
	if !ok {
		t.Fatal("row rejected")
	}
	if j.Title != "Lead - US Stocks" || j.URL != "https://www.iimjobs.com/j/lead-us-stocks-1741769" {
		t.Errorf("title/url = %q / %q", j.Title, j.URL)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 2_500_000 || *j.SalaryMax != 3_750_000 || j.SalaryCurrency != "INR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.WorkMode != "remote" || !j.Remote || j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 3 {
		t.Errorf("mode %q remote %v exp %v", j.WorkMode, j.Remote, j.ExperienceYearsMin)
	}
	row.Confidential = 1
	if _, ok := s.toJob(row, false); ok {
		t.Error("a confidential posting was kept")
	}
}

func TestHiristPlaces(t *testing.T) {
	cases := []struct {
		in       []string
		loc, mod string
	}{
		{[]string{"Bangalore"}, "Bangalore", ""},
		{[]string{"Remote"}, "Remote", "remote"},
		{[]string{"Bangalore", "Remote"}, "Bangalore; Remote", ""},
		{[]string{"Anywhere in India/Multiple Locations", "Delhi NCR"}, "India; Delhi", ""},
		{[]string{"Others", "Metros"}, "", ""},
	}
	for _, c := range cases {
		var locs []hiristLocation
		for _, n := range c.in {
			locs = append(locs, hiristLocation{Name: n})
		}
		if loc, mode := hiristPlaces(locs); loc != c.loc || mode != c.mod {
			t.Errorf("%v => %q/%q, want %q/%q", c.in, loc, mode, c.loc, c.mod)
		}
	}
}

func TestHiristTitle(t *testing.T) {
	cases := []struct{ title, company, want string }{
		{"PTC Software - Technical Lead - Full Stack Development", "PTC", "Technical Lead - Full Stack Development"},
		{"Hartford - Senior Director", "The Hartford", "Senior Director"},
		{"Manager/Senior Manager - Full Stack & Generative AI", "EXL Services", "Manager/Senior Manager - Full Stack & Generative AI"},
		{"Java Developer", "Acme", "Java Developer"},
		{"Acme -", "Acme", "Acme -"},
	}
	for _, c := range cases {
		if got := hiristTitle(c.title, c.company); got != c.want {
			t.Errorf("hiristTitle(%q, %q) = %q, want %q", c.title, c.company, got, c.want)
		}
	}
}

// The listing's totalPages rounds down (103 postings answer totalPages 1 while page 1 still
// serves the last 3), so the walk follows hasMore and an empty page instead.
func TestHiristWalksUntilHasMoreEnds(t *testing.T) {
	page := func(from, n int, more bool) string {
		var rows []map[string]any
		for i := range n {
			rows = append(rows, map[string]any{"id": from + i, "title": "QA Engineer", "companyData": map[string]any{"companyName": "Acme"}})
		}
		out, _ := json.Marshal(map[string]any{"data": rows, "hasMore": more, "totalPages": 1})
		return string(out)
	}
	fake := &hiristFake{pages: map[int]string{0: page(3000000, 100, true), 1: page(3000100, 3, false)}}
	if _, err := NewHirist(fake).Fetch(context.Background(), CompanyEntry{Board: "16"}); err != nil {
		t.Fatal(err)
	}
	if got := len(fake.detailCalls()); got != 103 {
		t.Errorf("asked %d details, want all 103 listed postings", got)
	}
}

// A posting whose employer is already covered, or whose title the catalogue filter rejects, is
// yielded body-less without a detail request, so the budget goes to postings that will be kept.
func TestHiristGateSkipsTheDetailOfADiscardedPosting(t *testing.T) {
	fake := &hiristFake{pages: map[int]string{0: hiristCategoryFixture(t)}}
	src := NewHirist(fake).(CoverageGated)
	gate := DetailGate{
		Covered:  func(cs []string) map[string]bool { return map[string]bool{"The Hartford": true} },
		Rejected: func(j Job) bool { return j.ExternalID == "1678906" },
	}
	jobs, err := src.FetchNewGated(context.Background(), CompanyEntry{Board: "16"}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.detailCalls(); !slices.Equal(got, []string{"1679150"}) {
		t.Fatalf("detail calls = %v, want only the posting neither answer condemns", got)
	}
	bodyless := 0
	for _, j := range jobs {
		if j.Description == "" && !j.SeenRefresh {
			bodyless++
		}
	}
	if len(jobs) != 3 || bodyless != 2 {
		t.Errorf("got %d jobs, %d body-less; want 3 with the 2 condemned ones body-less", len(jobs), bodyless)
	}
}
