package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func cakeFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/cake/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// cakeFake answers searches through search and details from fixtures keyed by posting path.
type cakeFake struct {
	t       *testing.T
	search  func(r cakeSearchRequest) (any, error)
	details map[string]string
	mu      sync.Mutex
	posts   []cakeSearchRequest
	gets    []string
}

func (f *cakeFake) PostJSON(_ context.Context, _ string, body, v any) error {
	r := body.(cakeSearchRequest)
	f.mu.Lock()
	f.posts = append(f.posts, r)
	f.mu.Unlock()
	resp, err := f.search(r)
	if err != nil {
		return err
	}
	if raw, ok := resp.([]byte); ok {
		return json.Unmarshal(raw, v)
	}
	b, _ := json.Marshal(resp)
	return json.Unmarshal(b, v)
}

func (f *cakeFake) GetJSON(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	name, ok := f.details[strings.TrimPrefix(u, cakeAPI+"/")]
	if !ok {
		return &StatusError{Code: 404}
	}
	return json.Unmarshal(cakeFixture(f.t, name), v)
}

const (
	cakeRecruiterPath = "3eb7ac95-a9bf-4bd3-8977-3a2ea9eb5cbf-recruiter-fcbd1cb24a19fe3be7fbaf582b8827"
	cakeRetailPath    = "2c3e7ed2-4eaa-4107-8265-f4aa4cf70368-retail-staff-omnichannel-sales-7ac56134e02813a96925401db1d809"
)

func cakeOnePage(t *testing.T, name string) func(cakeSearchRequest) (any, error) {
	return func(r cakeSearchRequest) (any, error) {
		if r.Page != 1 {
			return map[string]any{"total_entries": 0, "total_relation": "eq", "data": []any{}}, nil
		}
		return cakeFixture(t, name), nil
	}
}

func TestCakeMapsAListedPostingAndItsDetail(t *testing.T) {
	f := &cakeFake{t: t, search: cakeOnePage(t, "search_slice.json"), details: map[string]string{
		cakeRecruiterPath: "detail_1.json",
	}}
	jobs, err := newTestCake(f).Fetch(context.Background(), CompanyEntry{Provider: "cake"})
	if err != nil {
		t.Fatal(err)
	}
	// The slice lists three postings; only one has a detail, the other two answer 404 (gone).
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != cakeRecruiterPath || j.Title != "Recruiter" || j.Company != "Micron Technology" {
		t.Errorf("identity = %q %q %q", j.ExternalID, j.Title, j.Company)
	}
	if j.URL != "https://www.cake.me/companies/micron-technology/jobs/"+cakeRecruiterPath {
		t.Errorf("url = %q", j.URL)
	}
	if !reflect.DeepEqual(j.Countries, []string{"jp"}) {
		t.Errorf("countries = %v", j.Countries)
	}
	if !strings.HasPrefix(j.Location, "Hiroshima, Hiroshima-shi, Hiroshima, Japan; Hiroshima, Japan") {
		t.Errorf("location = %q", j.Location)
	}
	if j.WorkMode != "onsite" || j.EmploymentType != "full_time" || j.Seniority != "" {
		t.Errorf("facets = %q %q %q", j.WorkMode, j.EmploymentType, j.Seniority)
	}
	// A 0.00-0.00 range is the platform's "not stated".
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" {
		t.Errorf("salary = %v %v %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-22" {
		t.Errorf("posted = %v", j.PostedAt)
	}
	if j.IsTechHint {
		t.Error("an hr_ posting must not carry the tech hint")
	}
	if !strings.Contains(j.Description, "<p>Our vision is to transform") {
		t.Errorf("description = %.120q", j.Description)
	}
}

func TestCakeMapsAnHourlyChinesePosting(t *testing.T) {
	f := &cakeFake{t: t, search: cakeOnePage(t, "search_zh.json"), details: map[string]string{
		cakeRetailPath: "detail_zh.json",
	}}
	jobs, err := newTestCake(f).Fetch(context.Background(), CompanyEntry{Provider: "cake"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %d, err = %v", len(jobs), err)
	}
	j := jobs[0]
	if j.Company != "Decathlon Taiwan 台灣迪卡儂" || j.EmploymentType != "part_time" || j.Seniority != "junior" {
		t.Errorf("got %q %q %q", j.Company, j.EmploymentType, j.Seniority)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 210 || j.SalaryMax == nil || *j.SalaryMax != 210 ||
		j.SalaryCurrency != "TWD" || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %v %v %q %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !reflect.DeepEqual(j.Countries, []string{"tw"}) {
		t.Errorf("countries = %v", j.Countries)
	}
}

func TestCakeDetailSectionsAndHiddenSalary(t *testing.T) {
	l := cakeListing{Path: "p", Title: "Backend Engineer"}
	l.Page.Path, l.Page.Name = "acme", "Acme"
	j := cakeJob(l, cakeDetail{
		Path: "p", Profession: "it_back-end-engineer", Remote: "partial_remote_work",
		Description: "<p>About</p>", Requirements: "<ul><li>Go</li></ul>",
		SalaryMin: "60000", SalaryMax: "90000", Currency: "twd", SalaryType: "per_month", HideSalaryMax: true,
	})
	if !j.IsTechHint || j.WorkMode != "hybrid" {
		t.Errorf("hint/workmode = %v %q", j.IsTechHint, j.WorkMode)
	}
	if !strings.Contains(j.Description, "<h3>Requirements</h3>") || !strings.Contains(j.Description, "Go") {
		t.Errorf("description = %q", j.Description)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 60000 || j.SalaryMax != nil || j.SalaryCurrency != "TWD" {
		t.Errorf("salary = %v %v %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	hidden := cakeJob(l, cakeDetail{SalaryMin: "1", SalaryMax: "2", Currency: "TWD", SalaryType: "per_month", HideSalary: true})
	if hidden.SalaryMin != nil || hidden.SalaryCurrency != "" {
		t.Errorf("a hidden salary was stated: %v %q", hidden.SalaryMin, hidden.SalaryCurrency)
	}
}

// cakeRow builds a listing row as the search returns it.
func cakeRow(path string) map[string]any {
	return map[string]any{"path": path, "title": "T " + path, "page": map[string]any{"path": "c", "name": "C"}}
}

func cakeRows(prefix string, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = cakeRow(prefix + "-" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}
	return out
}

// A slice the window cannot hold is split on the next facet, using only leaf professions, and
// every slice that fits is read to its last page.
func TestCakeSplitsSlicesUntilEachFitsTheWindow(t *testing.T) {
	facets := map[string]any{
		"professions": []string{"it", "it_back-end-engineer", "hr_recruiter"},
		"job_types":   []string{"full_time", "part_time"},
	}
	f := &cakeFake{t: t, details: map[string]string{}}
	f.search = func(r cakeSearchRequest) (any, error) {
		prof, jt := "", ""
		if v := r.Filters["professions"]; len(v) == 1 {
			prof = v[0]
		}
		if v := r.Filters["job_types"]; len(v) == 1 {
			jt = v[0]
		}
		switch {
		case prof == "it":
			t.Errorf("a family profession was queried: %v", r.Filters)
		case prof == "" || (prof == "it_back-end-engineer" && jt == ""):
			return map[string]any{"total_entries": 500, "total_relation": "gte", "available_facets": facets, "data": cakeRows("cap", cakePerPage)}, nil
		case prof == "it_back-end-engineer" && jt == "full_time":
			// 50 rows: two pages.
			if r.Page == 1 {
				return map[string]any{"total_entries": 50, "total_relation": "eq", "available_facets": facets, "data": cakeRows("ft", cakePerPage)}, nil
			}
			return map[string]any{"total_entries": 50, "total_relation": "eq", "data": cakeRows("ft2", 5)}, nil
		case prof == "it_back-end-engineer" && jt == "part_time":
			return map[string]any{"total_entries": 1, "total_relation": "eq", "data": []any{cakeRow("pt")}}, nil
		case prof == "hr_recruiter":
			return map[string]any{"total_entries": 2, "total_relation": "eq", "data": []any{cakeRow("hr1"), cakeRow("hr2")}}, nil
		}
		return map[string]any{"total_entries": 0, "total_relation": "eq", "data": []any{}}, nil
	}
	jobs, err := newTestCake(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("%s: a seen posting must be a liveness refresh", j.ExternalID)
		}
		got[j.ExternalID] = true
	}
	for _, want := range []string{"ft-aa", "ft2-aa", "pt", "hr1", "hr2"} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
	if len(f.gets) != 0 {
		t.Errorf("seen postings were hydrated: %v", f.gets)
	}
}

func TestCakeHydratesOnlyUnseenPostingsWithinTheBudget(t *testing.T) {
	f := &cakeFake{t: t, search: cakeOnePage(t, "search_slice.json"), details: map[string]string{
		cakeRecruiterPath: "detail_1.json",
	}}
	src := newTestCake(f)
	src.maxNewPerRun = 1
	jobs, err := src.FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != cakeRecruiterPath })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.gets) != 1 {
		t.Errorf("details fetched = %v, want only the unseen one", f.gets)
	}
	var fresh, refreshed int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		} else {
			fresh++
		}
	}
	if fresh != 1 || refreshed != 2 {
		t.Errorf("fresh = %d, refreshed = %d", fresh, refreshed)
	}
}

// Any search failing fails the crawl: a partial listing would read as postings gone.
func TestCakeFailsTheCrawlWhenASearchFails(t *testing.T) {
	f := &cakeFake{t: t, details: map[string]string{}}
	f.search = func(r cakeSearchRequest) (any, error) {
		if r.Page == 2 {
			return nil, errors.New("boom")
		}
		return map[string]any{"total_entries": 60, "total_relation": "eq", "data": cakeRows("x", cakePerPage)}, nil
	}
	if _, err := newTestCake(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a failed page must fail the crawl")
	}
}

func newTestCake(f *cakeFake) cake {
	s := NewCake(f).(cake)
	s.backoff = 0
	return s
}

// One search lost to a slow exit must not cost the whole listing: it is repeated.
func TestCakeRetriesATransientSearchFailure(t *testing.T) {
	calls := 0
	f := &cakeFake{t: t, details: map[string]string{}}
	f.search = func(cakeSearchRequest) (any, error) {
		calls++
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		return map[string]any{"total_entries": 1, "total_relation": "eq", "data": []any{cakeRow("x")}}, nil
	}
	jobs, err := newTestCake(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %d, err = %v", len(jobs), err)
	}
}
