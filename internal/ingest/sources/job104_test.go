package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func job104Fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/job104/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// job104Fake answers searches through search and the code tables from fixtures.
type job104Fake struct {
	t       *testing.T
	search  func(q url.Values) (any, error)
	jobCats []job104Code
	mu      sync.Mutex
	queries []url.Values
	referer []string
}

func (f *job104Fake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	q := parsed.Query()
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.referer = append(f.referer, headers["Referer"])
	f.mu.Unlock()
	resp, err := f.search(q)
	if err != nil {
		return err
	}
	b, ok := resp.([]byte)
	if !ok {
		b, _ = json.Marshal(resp)
	}
	return json.Unmarshal(b, v)
}

func (f *job104Fake) GetJSON(_ context.Context, u string, v any) error {
	switch u {
	case job104AreaURL:
		return json.Unmarshal(job104Fixture(f.t, "area.json"), v)
	case job104JobCatURL:
		b, _ := json.Marshal(f.jobCats)
		return json.Unmarshal(b, v)
	}
	return &StatusError{Code: 404}
}

func newTestJob104(f *job104Fake) job104 {
	if f.jobCats == nil {
		f.jobCats = []job104Code{{No: "2001000000"}}
	}
	s := NewJob104(f, f).(job104)
	s.limiter = noWait{}
	s.backoff = 0
	return s
}

func TestJob104MapsASearchRow(t *testing.T) {
	f := &job104Fake{t: t, search: func(url.Values) (any, error) { return job104Fixture(t, "search_lienchiang.json"), nil }}
	jobs, err := newTestJob104(f).Fetch(context.Background(), CompanyEntry{Provider: "job104", Board: "6001023000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "959wn" || j.URL != "https://www.104.com.tw/job/959wn" || j.Title != "工地主任" {
		t.Errorf("identity = %q %q %q", j.ExternalID, j.URL, j.Title)
	}
	if !strings.HasSuffix(j.Location, ", Taiwan") || !reflect.DeepEqual(j.Countries, []string{"tw"}) {
		t.Errorf("location = %q, countries = %v", j.Location, j.Countries)
	}
	// 9999999 is the platform's "and up": a floor with no ceiling.
	if j.SalaryMin == nil || *j.SalaryMin != 65000 || j.SalaryMax != nil || j.SalaryCurrency != "TWD" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v %v %q %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	// period 6 is "5 years or more".
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 5 || j.EmploymentType != "full_time" {
		t.Errorf("experience = %v, type = %q", j.ExperienceYearsMin, j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-26" || j.Description == "" {
		t.Errorf("posted = %v, description empty = %v", j.PostedAt, j.Description == "")
	}
	if j.IsTechHint {
		t.Error("a construction posting must not carry the tech hint")
	}
	hourly := jobs[1]
	if hourly.SalaryPeriod != "hour" || hourly.SalaryMin == nil || *hourly.SalaryMin != 196 || hourly.EmploymentType != "part_time" || hourly.ExperienceYearsMin != nil {
		t.Errorf("hourly = %v %q %q %v", hourly.SalaryMin, hourly.SalaryPeriod, hourly.EmploymentType, hourly.ExperienceYearsMin)
	}
	if f.referer[0] != job104Referer || f.queries[0].Get("area") != "6001023000" {
		t.Errorf("request = %v referer %q", f.queries[0], f.referer[0])
	}
}

// Promoted rows from other areas ride along on a page and must not be filed under this board.
func TestJob104DropsRowsAddressedOutsideTheBoard(t *testing.T) {
	f := &job104Fake{t: t, search: func(url.Values) (any, error) { return job104Fixture(t, "search_promoted.json"), nil }}
	jobs, err := newTestJob104(f).Fetch(context.Background(), CompanyEntry{Board: "6002006000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want the 2 addressed in the board's area", len(jobs))
	}
	for _, j := range jobs {
		if j.Countries != nil {
			t.Errorf("%s: an overseas posting must not be stated Taiwanese", j.ExternalID)
		}
		if j.SalaryMin != nil {
			t.Errorf("%s: a negotiable salary states no figure", j.ExternalID)
		}
	}
}

func job104Page(addr int64, codes ...string) map[string]any {
	rows := make([]any, len(codes))
	for i, c := range codes {
		rows[i] = map[string]any{"jobName": "Job " + c, "custName": "Co", "jobAddrNo": addr, "link": map[string]any{"job": "https://www.104.com.tw/job/" + c}}
	}
	return map[string]any{"data": rows}
}

func withLastPage(p map[string]any, last int) map[string]any {
	p["metadata"] = map[string]any{"pagination": map[string]any{"lastPage": last}}
	return p
}

// A board at the page cap is split down the category tree and then over the experience
// brackets; a posting met in two slices is kept once.
func TestJob104SplitsCappedSlices(t *testing.T) {
	f := &job104Fake{t: t, jobCats: []job104Code{
		{No: "2007000000", Children: []job104Code{{No: "2007001000"}, {No: "2007002000"}}},
		{No: "2001000000"},
	}}
	const addr = 6001001005
	f.search = func(q url.Values) (any, error) {
		page, _ := strconv.Atoi(q.Get("page"))
		switch cat, exp := q.Get("jobcat"), q.Get("jobexp"); {
		case cat == "" || cat == "2007000000" || (cat == "2007002000" && exp == ""):
			return withLastPage(job104Page(addr, "capped"), job104MaxPage), nil
		case cat == "2007001000":
			return withLastPage(job104Page(addr, "a"+strconv.Itoa(page), "shared"), 2), nil
		case cat == "2007002000" && exp == "3":
			return withLastPage(job104Page(addr, "b", "shared"), 1), nil
		case cat == "2001000000":
			return withLastPage(job104Page(addr, "c"), 1), nil
		}
		return withLastPage(job104Page(addr), 0), nil
	}
	jobs, err := newTestJob104(f).Fetch(context.Background(), CompanyEntry{Board: "6001001005"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, j := range jobs {
		got = append(got, j.ExternalID)
	}
	want := []string{"a1", "shared", "a2", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobs = %v, want %v", got, want)
	}
}

func TestJob104FailsTheBoardWhenAPageFails(t *testing.T) {
	f := &job104Fake{t: t, search: func(q url.Values) (any, error) {
		if q.Get("page") == "2" {
			return nil, errors.New("boom")
		}
		return withLastPage(job104Page(6001023001, "x"), 3), nil
	}}
	if _, err := newTestJob104(f).Fetch(context.Background(), CompanyEntry{Board: "6001023000"}); err == nil {
		t.Fatal("a failed page must fail the board")
	}
}

func TestJob104AreaPrefix(t *testing.T) {
	for area, want := range map[string]string{
		"6001001000": "6001001",
		"6001001005": "6001001005",
		"6003000000": "6003",
		"taipei":     "",
		"600100100":  "",
	} {
		if got := job104AreaPrefix(area); got != want {
			t.Errorf("job104AreaPrefix(%q) = %q, want %q", area, got, want)
		}
	}
}

func TestJob104TechHintFollowsTheITFamily(t *testing.T) {
	if !job104IsTech([]int64{2012002001, 2007001004}) || job104IsTech([]int64{2008001001}) {
		t.Error("only the 2007 family is technical")
	}
}

// The platform answers an occasional 429 with no Retry-After; a repeat is served, so a 429
// must not fail the board.
func TestJob104RetriesARateLimitedSearch(t *testing.T) {
	calls := 0
	f := &job104Fake{t: t, search: func(url.Values) (any, error) {
		calls++
		if calls <= 2 {
			return nil, &StatusError{Code: 429}
		}
		return withLastPage(job104Page(6001023001, "x"), 1), nil
	}}
	jobs, err := newTestJob104(f).Fetch(context.Background(), CompanyEntry{Board: "6001023000"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %d, err = %v", len(jobs), err)
	}
}
