package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

func onlyfyFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/onlyfy/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const onlyfyJungheinrich = "jungheinrich.onlyfy.jobs/candidate/job/ajax_list?display_length=100&page="

func TestOnlyfyProvider(t *testing.T) {
	if got := NewOnlyfy(nil).Provider(); got != "onlyfy" {
		t.Errorf("Provider() = %q, want onlyfy", got)
	}
}

// TestOnlyfyWalksListingAndHydratesNewPosting reads the real Jungheinrich site captured
// 2026-10-25 (whitespace collapsed): 185 postings over two pages, and one posting page.
func TestOnlyfyWalksListingAndHydratesNewPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route(onlyfyJungheinrich+"1&", onlyfyFixture(t, "jungheinrich_page1.html")).
		route(onlyfyJungheinrich+"2&", onlyfyFixture(t, "jungheinrich_page2.html")).
		route("/job/qqeva17cjz1xgg6jzmc9hw4jnsc68o7", onlyfyFixture(t, "jungheinrich_detail.html"))
	jobs, err := NewOnlyfy(fake).(onlyfy).FetchNew(context.Background(),
		CompanyEntry{Provider: "onlyfy", Board: "jungheinrich", Company: "Jungheinrich"},
		func(id string) bool { return id != "qqeva17cjz1xgg6jzmc9hw4jnsc68o7" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 185 {
		t.Fatalf("got %d jobs, want the 185 the listing states", len(jobs))
	}
	var j Job
	for _, x := range jobs {
		if !x.SeenRefresh {
			j = x
		}
	}
	if j.URL != "https://jungheinrich.onlyfy.jobs/job/qqeva17cjz1xgg6jzmc9hw4jnsc68o7" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Cybersecurity Engineer (m/w/d) Elektrische Antriebe" || j.Company != "Jungheinrich" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Norderstedt, DE" || len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	// TELECOMMUTE on this posting is not taken as remote (see onlyfy.detail).
	if j.WorkMode != "" || j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want unset", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if deref(j.SalaryMin) != 88000 || deref(j.SalaryMax) != 93000 || j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", deref(j.SalaryMin), deref(j.SalaryMax), j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Norderstedt") {
		t.Errorf("Description lacks the body: %.200q", j.Description)
	}
}

// A walk that ends short of the stated count fails the board rather than passing a partial
// listing on to the sweep.
func TestOnlyfyShortWalkFails(t *testing.T) {
	fake := (&routedHTTP{}).
		route(onlyfyJungheinrich+"1&", onlyfyFixture(t, "jungheinrich_page1.html")).
		route(onlyfyJungheinrich+"2&", onlyfyFixture(t, "empty_page1.html"))
	if _, err := NewOnlyfy(fake).Fetch(context.Background(), CompanyEntry{Board: "jungheinrich"}); err == nil {
		t.Fatal("100 of 185 postings passed as the whole board")
	}
	failing := (&routedHTTP{}).
		route(onlyfyJungheinrich+"1&", onlyfyFixture(t, "jungheinrich_page1.html")).
		routeErr(onlyfyJungheinrich+"2&", &StatusError{Code: http.StatusBadGateway})
	if _, err := NewOnlyfy(failing).Fetch(context.Background(), CompanyEntry{Board: "jungheinrich"}); err == nil {
		t.Fatal("a failed second page passed as a complete listing")
	}
}

func TestOnlyfyEmptyAndGoneBoards(t *testing.T) {
	empty := (&routedHTTP{}).route("cms-rrh.onlyfy.jobs/candidate/job/ajax_list", onlyfyFixture(t, "empty_page1.html"))
	if jobs, err := NewOnlyfy(empty).Fetch(context.Background(), CompanyEntry{Board: "cms-rrh"}); err != nil || len(jobs) != 0 {
		t.Errorf("empty board: jobs=%d err=%v", len(jobs), err)
	}
	gone := (&routedHTTP{}).routeErr("idealo.onlyfy.jobs", &StatusError{Code: http.StatusNotFound})
	if _, err := NewOnlyfy(gone).Fetch(context.Background(), CompanyEntry{Board: "idealo"}); !errors.Is(err, ErrBoardGone) {
		t.Errorf("404 board: err=%v, want ErrBoardGone", err)
	}
	// A fragment without the pager's count is not a listing.
	foreign := (&routedHTTP{}).route("x.onlyfy.jobs/candidate/job/ajax_list", "<html><body>maintenance</body></html>")
	if _, err := NewOnlyfy(foreign).Fetch(context.Background(), CompanyEntry{Board: "x"}); err == nil {
		t.Error("a page with no posting count read as an empty board")
	}
}

func TestOnlyfyDetailFailures(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/job/bare", "<html><body><h1>x</h1></body></html>").
		routeErr("/job/down", &StatusError{Code: http.StatusServiceUnavailable}).
		routeErr("/job/gone", &StatusError{Code: http.StatusNotFound})
	s := NewOnlyfy(fake).(onlyfy)
	for _, id := range []string{"bare", "down"} {
		if j, ok := s.detail(context.Background(), CompanyEntry{}, id, "https://x.onlyfy.jobs/job/"+id); !ok || !j.Unreadable {
			t.Errorf("%s: want an unreadable marker, got ok=%v %+v", id, ok, j)
		}
	}
	if _, ok := s.detail(context.Background(), CompanyEntry{}, "gone", "https://x.onlyfy.jobs/job/gone"); ok {
		t.Error("a 404 posting must be dropped")
	}
}

func TestOnlyfyRejectsBadBoard(t *testing.T) {
	for _, b := range []string{"x.onlyfy.jobs", "https://x", "../x", ""} {
		if _, err := NewOnlyfy(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
}
