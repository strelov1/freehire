package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
)

// greythrHTTP answers the listing POST and the category GET from canned bodies.
type greythrFake struct {
	list, cats string
	listErr    error
	catsErr    error
	urls       []string
}

func (f *greythrFake) PostJSON(_ context.Context, url string, _, v any) error {
	f.urls = append(f.urls, url)
	if f.listErr != nil {
		return f.listErr
	}
	return json.Unmarshal([]byte(f.list), v)
}

func (f *greythrFake) GetJSON(_ context.Context, url string, v any) error {
	f.urls = append(f.urls, url)
	if f.catsErr != nil {
		return f.catsErr
	}
	return json.Unmarshal([]byte(f.cats), v)
}

func greythrFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/greythr/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGreytHRRegistered(t *testing.T) {
	s, ok := All(nil)["greythr"]
	if !ok || s.Provider() != "greythr" {
		t.Fatal("All() must register greythr")
	}
	if !FullBoardListingProviders(All(nil))["greythr"] {
		t.Error("greythr should implement fullBoardListing")
	}
}

func TestGreytHRFetchMapsTheListing(t *testing.T) {
	f := &greythrFake{
		list: greythrFixture(t, "published_jobs_specsmakershrm.json"),
		cats: greythrFixture(t, "emp_category_specsmakershrm.json"),
	}
	jobs, err := NewGreytHR(f).Fetch(context.Background(), CompanyEntry{Board: "specsmakershrm.greythr.com", Company: "Specsmakers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if f.urls[0] != "https://specsmakershrm.greythr.com/hire/api/career/published_jobs/" {
		t.Errorf("listing url = %s", f.urls[0])
	}
	j := jobs[0]
	if j.ExternalID != "f271e475-a4bf-4634-be8c-8da3b57de1be" || j.Title != "OPTOMETRIST" || j.Company != "Specsmakers" ||
		j.Location != "KARAIKUDI" || j.URL != "https://specsmakershrm.greythr.com/hire/jobs/optometrist-147" {
		t.Errorf("mapped job = %+v", j)
	}
	if jobs[1].Location != "BANGALORE" {
		t.Errorf("second location = %q, want the category name for id 4", jobs[1].Location)
	}
	if j.EmploymentType != "full_time" || j.WorkMode != "" || j.Description == "" {
		t.Errorf("type/mode/body = %q/%q/%d chars", j.EmploymentType, j.WorkMode, len(j.Description))
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("posted_at = %v", j.PostedAt)
	}
}

// A category map that cannot be read costs the location, never the board.
func TestGreytHRFetchSurvivesAMissingCategoryMap(t *testing.T) {
	f := &greythrFake{list: greythrFixture(t, "published_jobs_specsmakershrm.json"), catsErr: errors.New("500")}
	jobs, err := NewGreytHR(f).Fetch(context.Background(), CompanyEntry{Board: "specsmakershrm.greythr.com"})
	if err != nil || len(jobs) != 2 || jobs[0].Location != "" {
		t.Fatalf("got %d jobs err=%v location=%q", len(jobs), err, jobs[0].Location)
	}
}

func TestGreytHRFetchFailures(t *testing.T) {
	gone := &greythrFake{listErr: &StatusError{Method: http.MethodPost, Code: http.StatusNotFound, URL: "u"}}
	if _, err := NewGreytHR(gone).Fetch(context.Background(), CompanyEntry{Board: "nobody.greythr.com"}); !errors.Is(err, ErrBoardGone) {
		t.Errorf("a 404 listing must read as ErrBoardGone, got %v", err)
	}
	if _, err := NewGreytHR(&greythrFake{listErr: errors.New("timeout")}).Fetch(context.Background(), CompanyEntry{Board: "x.greythr.com"}); err == nil || errors.Is(err, ErrBoardGone) {
		t.Errorf("a transport failure must fail the crawl without claiming the board is gone, got %v", err)
	}
	if _, err := NewGreytHR(&greythrFake{list: `{"detail":"x"}`}).Fetch(context.Background(), CompanyEntry{Board: "x.greythr.com"}); err == nil {
		t.Error("an answer without data is reshaped, not an empty board")
	}
	for _, board := range []string{"evil.example.com", "greythr.com", "a.b.greythr.com"} {
		f := &greythrFake{}
		if _, err := NewGreytHR(f).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil || len(f.urls) != 0 {
			t.Errorf("%q: want a refusal before any request", board)
		}
	}
}

func TestGreytHREmploymentType(t *testing.T) {
	cases := map[string]string{"Full-time": "full_time", "Part-time": "part_time", "Internship": "internship",
		"Contract": "contract", "Direct Contract": "contract", "Freelance": "", "": ""}
	for in, want := range cases {
		if got := greythrEmploymentType(in); got != want {
			t.Errorf("greythrEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
