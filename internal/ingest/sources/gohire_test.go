package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// gohireFixture reads a page captured verbatim from jobs.gohire.io (2026-10-24). The site is
// dexerto-de5jlhjo, whose listing states "Page 1 of 3, Total 21 jobs"; list_dexerto_past_end is
// what it answers for ?page=99 — the stated count, and no card.
func gohireFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/gohire/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// gohireDexerto routes the dexerto-de5jlhjo listing: two full pages, then a page carrying no
// card where the third (one posting) would be.
func gohireDexerto(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route("dexerto-de5jlhjo/?page=1", gohireFixture(t, "list_dexerto_p1.html")).
		route("dexerto-de5jlhjo/?page=2", gohireFixture(t, "list_dexerto_p2.html")).
		route("dexerto-de5jlhjo/?page=3", gohireFixture(t, "list_dexerto_past_end.html"))
}

func TestGoHireProvider(t *testing.T) {
	s, ok := All(nil)["gohire"]
	if !ok || s.Provider() != "gohire" {
		t.Fatal("All() must register gohire")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("gohire must be a HydratingSource: the body costs a page request")
	}
	if !slices.Contains(FilterableProviders(), "gohire") {
		t.Error("FilterableProviders() should include gohire")
	}
}

func TestGoHireListWalksEveryPage(t *testing.T) {
	fake := gohireDexerto(t)
	// The walk ends on the empty page; it never asks past the listing's own page count.
	fake.routeErr("?page=4", errors.New("page 4 must not be requested"))
	jobs, err := NewGoHire(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Dexerto", Provider: "gohire", Board: "dexerto-de5jlhjo"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 20 {
		t.Fatalf("len(jobs) = %d, want the two pages' 20 cards", len(jobs))
	}
	if fake.calls != 3 {
		t.Errorf("requests = %d, want the three listing pages alone", fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "https://jobs.gohire.io/dexerto-de5jlhjo/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestGoHireFetchMapsAPosting(t *testing.T) {
	fake := gohireDexerto(t).route("marketing-manager-304991", gohireFixture(t, "detail_304991.html"))
	// Every other posting is already stored, so only this one costs a page request.
	jobs, err := NewGoHire(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Dexerto", Provider: "gohire", Board: "dexerto-de5jlhjo"},
		func(id string) bool { return id != "304991" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var j *Job
	for i := range jobs {
		if jobs[i].ExternalID == "304991" {
			j = &jobs[i]
		}
	}
	if j == nil || j.SeenRefresh {
		t.Fatalf("posting 304991 was not hydrated: %+v", jobs)
	}
	if j.URL != "https://jobs.gohire.io/dexerto-de5jlhjo/marketing-manager-304991/" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Marketing Manager" || j.Company != "Dexerto" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Remote, United Kingdom" || !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Location/Remote/WorkMode = %q/%v/%q, want the card's remote UK location", j.Location, j.Remote, j.WorkMode)
	}
	if !slices.Equal(j.Countries, []string{"gb"}) {
		t.Errorf("Countries = %v, want [gb] from applicantLocationRequirements", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 50000 || j.SalaryMax == nil || *j.SalaryMax != 60000 ||
		j.SalaryCurrency != "GBP" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %q/%q, want 50000-60000 GBP/year", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-22" {
		t.Errorf("PostedAt = %v, want 2026-10-22", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Dexerto") {
		t.Errorf("Description = %.200q", j.Description)
	}
}

func TestGoHireRefusalStopsTheRun(t *testing.T) {
	fake := gohireDexerto(t)
	fake.routeErr("/marketing-manager-304991/", &StatusError{Method: http.MethodGet, Code: http.StatusTooManyRequests})
	src := NewGoHire(fake).(HydratingSource)
	jobs, err := src.FetchNew(context.Background(),
		CompanyEntry{Company: "Dexerto", Provider: "gohire", Board: "dexerto-de5jlhjo"},
		func(id string) bool { return id != "304991" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var marker bool
	for _, j := range jobs {
		marker = marker || (j.ExternalID == "304991" && j.Unreadable)
	}
	if !marker {
		t.Error("a refused posting page must come back as an Unreadable marker, not be dropped")
	}
	// The refusal is about the IP: the next board in the same run asks for nothing and fails.
	before := fake.calls
	if _, err := src.Fetch(context.Background(), CompanyEntry{Board: "other-aaaaaaaa"}); err == nil {
		t.Error("Fetch after a refusal succeeded, want the board to fail")
	}
	if fake.calls != before {
		t.Errorf("requests after the refusal = %d, want none", fake.calls-before)
	}
}

func TestGoHirePageCount(t *testing.T) {
	fake := gohireDexerto(t)
	for page, want := range map[string]int{"?page=1": 3, "?page=3": 3} {
		root, err := fake.GetHTML(context.Background(), "https://jobs.gohire.io/dexerto-de5jlhjo/"+page)
		if err != nil {
			t.Fatal(err)
		}
		if got := gohirePageCount(root); got != want {
			t.Errorf("gohirePageCount(%s) = %d, want %d", page, got, want)
		}
	}
}

func TestGoHireListingFailures(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("?page=1", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	if _, err := NewGoHire(fake).Fetch(context.Background(), CompanyEntry{Board: "dexerto-de5jlhjo"}); err == nil {
		t.Error("Fetch succeeded, want an unknown site to fail the board")
	}
	if _, err := NewGoHire(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "dexerto-19265/../x"}); err == nil {
		t.Error("Fetch accepted a board that is not a <slug>-<code> segment")
	}
	// A later page failing fails the board too: a salvaged first page would leave the rest of
	// the site unseen, and the unseen sweep would close live postings.
	partial := (&routedHTTP{}).route("?page=1", gohireFixture(t, "list_dexerto_p1.html"))
	partial.routeErr("?page=2", errors.New("timeout"))
	if _, err := NewGoHire(partial).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Board: "dexerto-de5jlhjo"}, func(string) bool { return true }); err == nil {
		t.Error("FetchNew succeeded with page 2 unread, want the board to fail")
	}
}

func TestGoHireBoardIdentityFoldsTheSlug(t *testing.T) {
	a, _ := BoardDedupeKey(CompanyEntry{Provider: "gohire", Board: "dexerto-de5jlhjo"})
	b, _ := BoardDedupeKey(CompanyEntry{Provider: "gohire", Board: "dexerto-media-de5jlhjo"})
	c, _ := BoardDedupeKey(CompanyEntry{Provider: "gohire", Board: "dexerto-zzzzzzzz"})
	if a != b {
		t.Error("two slugs in front of one site code must be one crawl target")
	}
	if a == c {
		t.Error("different site codes must stay distinct")
	}
}
