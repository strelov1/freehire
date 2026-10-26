package sources

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func jobindexFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/jobindex/search.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jobindexStashPage renders a minimal search page around a searchResponse JSON object.
func jobindexStashPage(searchResponse string) string {
	return `<html><script>var Stash = {"jobsearch/result_app/list":{"storeData":{"subjobcategory_list":[],"searchResponse":` +
		searchResponse + `}}};</script></html>`
}

// jobindexFake serves the category page and every slice's first page from fixed bodies; a
// later page answers with no results unless pages says otherwise.
type jobindexFake struct {
	first     string
	sliceOne  string
	pages     map[string]string
	failOn    string
	mu        sync.Mutex
	requested []string
}

func (f *jobindexFake) GetText(_ context.Context, u string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, u)
	if u == f.failOn {
		return "", &StatusError{Code: 503}
	}
	if u == jobindexSearchURL {
		return f.first, nil
	}
	if body, ok := f.pages[u]; ok {
		return body, nil
	}
	if strings.HasSuffix(u, "&page=1") {
		return f.sliceOne, nil
	}
	return jobindexStashPage(`{"hitcount":0,"results":[]}`), nil
}

func (f *jobindexFake) count(u string) int {
	n := 0
	for _, r := range f.requested {
		if r == u {
			n++
		}
	}
	return n
}

func TestJobindexFetchMapsAndDeduplicatesAcrossSubcategories(t *testing.T) {
	fixture := jobindexFixture(t)
	f := &jobindexFake{first: fixture, sliceOne: fixture}
	jobs, err := NewJobindex(f).Fetch(context.Background(), CompanyEntry{Provider: "jobindex"})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture lists subcategories 93, 6, 4, 3 and 93 again; every slice answers with the
	// same three postings, which must come out once each.
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3: %+v", len(jobs), jobs)
	}
	for _, id := range []int{93, 6, 4, 3} {
		if got := f.count(fmt.Sprintf(jobindexSliceURL, id, 1)); got != 1 {
			t.Errorf("subcategory %d page 1 requested %d times, want 1", id, got)
		}
	}

	paid := jobs[1]
	if paid.ExternalID != "h1708229" || paid.Title != "AI Engineer til Forsvarets AI Center" ||
		paid.Company != "Forsvaret" || paid.URL != "https://www.jobindex.dk/vis-job/h1708229" ||
		paid.Location != "Kongens Lyngby, Danmark" {
		t.Errorf("paid posting mapped wrong: %+v", paid)
	}
	if paid.PostedAt == nil || paid.PostedAt.Format("2006-01-02") != "2026-10-15" {
		t.Errorf("posted = %v, want 2026-10-15", paid.PostedAt)
	}
	if paid.Description == "" || strings.Contains(paid.Description, "AI Engineer til Forsvarets AI Center</") {
		t.Errorf("description should be the teaser without the title: %q", paid.Description)
	}

	// A posting Jobindex's robot found elsewhere has a null company; companytext names it.
	robot := jobs[2]
	if robot.ExternalID != "r14065717" || robot.Company != "Baseball5 Denmark" {
		t.Errorf("robot posting mapped wrong: %+v", robot)
	}
	if !strings.Contains(robot.Description, "Do you have a handle on what works on young people") {
		t.Errorf("robot teaser missing: %q", robot.Description)
	}
	for _, chrome := range []string{"Se jobbet", "Gem job", "Indrykket", "Hentet fra"} {
		if strings.Contains(robot.Description, chrome) {
			t.Errorf("description carries card chrome %q: %q", chrome, robot.Description)
		}
	}
}

func TestJobindexSliceStopsAtTheDeclaredPageCap(t *testing.T) {
	result := `{"tid":"h%d","headline":"Backend Developer","company":{"name":"Acme"},"area":"Aarhus","firstdate":"2026-10-20","html":""}`
	page := func(n int) string {
		return jobindexStashPage(fmt.Sprintf(`{"hitcount":1500,"total_pages":75,"max_page":2,"results":[`+result+`]}`, n))
	}
	f := &jobindexFake{
		first:    `<script>var Stash = {"jobsearch/result_app/list":{"storeData":{"subjobcategory_list":[["IT",[["Database",93]]]]}}};</script>`,
		sliceOne: page(1),
		pages:    map[string]string{fmt.Sprintf(jobindexSliceURL, 93, 2): page(2)},
	}
	jobs, err := NewJobindex(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if f.count(fmt.Sprintf(jobindexSliceURL, 93, 3)) != 0 {
		t.Error("walked past the max_page the site declared")
	}
}

func TestJobindexSkipsArchivedPostings(t *testing.T) {
	f := &jobindexFake{
		first: `<script>var Stash = {"jobsearch/result_app/list":{"storeData":{"subjobcategory_list":[["IT",[["Database",93]]]]}}};</script>`,
		sliceOne: jobindexStashPage(`{"hitcount":2,"total_pages":1,"max_page":50,"results":[
			{"tid":"h1","headline":"Old","company":{"name":"Acme"},"is_archived":true},
			{"tid":"h2","headline":"New","company":{"name":"Acme"}}]}`),
	}
	jobs, err := NewJobindex(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "h2" {
		t.Fatalf("got %+v, want only h2", jobs)
	}
}

func TestJobindexFailsTheCrawlWhenAnyPageFails(t *testing.T) {
	fixture := jobindexFixture(t)
	f := &jobindexFake{first: fixture, sliceOne: fixture, failOn: fmt.Sprintf(jobindexSliceURL, 4, 2)}
	if _, err := NewJobindex(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a failed slice page must fail the crawl, not shrink it")
	}
}

func TestJobindexFailsWithoutSubcategories(t *testing.T) {
	f := &jobindexFake{first: `<html>no stash here</html>`}
	if _, err := NewJobindex(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a page without the Stash object must fail the crawl")
	}
}
