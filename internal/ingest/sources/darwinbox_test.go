package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// darwinboxPoster answers the alljobs POST with the page its body asks for.
type darwinboxPoster struct {
	pages map[int]string // page number -> raw JSON answer
	urls  []string
	err   error
}

func (p *darwinboxPoster) PostJSON(_ context.Context, url string, body, v any) error {
	p.urls = append(p.urls, url)
	if p.err != nil {
		return p.err
	}
	page := body.(map[string]any)["page"].(int)
	raw, ok := p.pages[page]
	if !ok {
		raw = `{"status":"success","data":[],"job_counts":0}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func darwinboxFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/darwinbox/alljobs_atherenergy.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDarwinboxRegistered(t *testing.T) {
	s, ok := All(nil)["darwinbox"]
	if !ok || s.Provider() != "darwinbox" {
		t.Fatal("All() must register darwinbox")
	}
	if !FullBoardListingProviders(All(nil))["darwinbox"] {
		t.Error("darwinbox should implement fullBoardListing")
	}
}

func TestDarwinboxFetchMapsTheListing(t *testing.T) {
	p := &darwinboxPoster{pages: map[int]string{1: darwinboxFixture(t)}}
	jobs, err := NewDarwinbox(p).Fetch(context.Background(), CompanyEntry{Board: "atherenergy.darwinbox.in", Company: "Ather Energy Limited"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if p.urls[0] != "https://atherenergy.darwinbox.in/ms/candidateapi/job/alljobs" {
		t.Errorf("listing url = %s", p.urls[0])
	}
	j := jobs[0]
	if j.ExternalID != "a6ad892cd96ba1" || j.Title != "International Business Operations Manager" ||
		j.Company != "Ather Energy Limited" || j.Location != "Bengaluru, Karnataka, India" ||
		j.URL != "https://atherenergy.darwinbox.in/ms/candidatev2/main/careers/jobDetails/a6ad892cd96ba1" {
		t.Errorf("mapped job = %+v", j)
	}
	if j.EmploymentType != "full_time" || j.WorkMode != "" || j.Remote {
		t.Errorf("type/mode = %q/%q/%v, want full_time from \"Permanent\" and no mode from is_remote=0", j.EmploymentType, j.WorkMode, j.Remote)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "in" {
		t.Errorf("countries = %v, want [in]", j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Unix() != 1792607400 {
		t.Errorf("posted_at = %v", j.PostedAt)
	}
	if j.Description == "" || strings.Contains(j.Description, "<script") {
		t.Errorf("description not carried from jd: %q", j.Description)
	}
}

// Paging ends at an empty page, and only a walk that adds up to the declared total succeeds.
func TestDarwinboxFetchWalksPagesToTheDeclaredTotal(t *testing.T) {
	row := func(id string) string {
		return fmt.Sprintf(`{"id":%q,"title":"Engineer %s","jd":"<p>x</p>","country":"India","is_remote":1,"emp_type_name":"Full Time","posted_on":0}`, id, id)
	}
	pages := map[int]string{
		1: `{"status":"success","job_counts":3,"data":[` + row("a") + `,` + row("b") + `]}`,
		2: `{"status":"success","job_counts":3,"data":[` + row("b") + `,` + row("c") + `]}`, // b re-served across the boundary
	}
	jobs, err := NewDarwinbox(&darwinboxPoster{pages: pages}).Fetch(context.Background(), CompanyEntry{Board: "x.darwinbox.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[2].ExternalID != "c" || jobs[0].WorkMode != "remote" {
		t.Errorf("jobs = %+v", jobs)
	}

	short := map[int]string{1: `{"status":"success","job_counts":5,"data":[` + row("a") + `]}`}
	if _, err := NewDarwinbox(&darwinboxPoster{pages: short}).Fetch(context.Background(), CompanyEntry{Board: "x.darwinbox.com"}); err == nil {
		t.Error("a walk short of the declared total must fail, not return a partial board")
	}
}

func TestDarwinboxFetchRefusesWhatIsNotABoard(t *testing.T) {
	for _, board := range []string{"evil.example.com", "x.darwinbox.in/../y", "darwinbox.in", "a.b.darwinbox.in"} {
		p := &darwinboxPoster{}
		if _, err := NewDarwinbox(p).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil || len(p.urls) != 0 {
			t.Errorf("%q: want a refusal before any request, got err=%v urls=%v", board, err, p.urls)
		}
	}
}

func TestDarwinboxFetchFailsOnAListingErrorOrReshapedAnswer(t *testing.T) {
	if _, err := NewDarwinbox(&darwinboxPoster{err: errors.New("403")}).Fetch(context.Background(), CompanyEntry{Board: "x.darwinbox.in"}); err == nil {
		t.Error("a listing error must fail the crawl")
	}
	reshaped := map[int]string{1: `{"status":"success","data":[]}`}
	if _, err := NewDarwinbox(&darwinboxPoster{pages: reshaped}).Fetch(context.Background(), CompanyEntry{Board: "x.darwinbox.in"}); err == nil {
		t.Error("an answer without job_counts is reshaped, not an empty board")
	}
}

func TestDarwinboxEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Permanent": "full_time", "Full-Time": "full_time", "PERMANENT - FULL TIME": "full_time", "FTE": "full_time",
		"Intern": "internship", "Internship": "internship", "Contract": "contract", "Consultant": "contract",
		"Part Time": "part_time", "Probationary": "", "BRANCH RESOURCE": "", "Employee": "", "": "",
	}
	for in, want := range cases {
		if got := darwinboxEmploymentType(in); got != want {
			t.Errorf("darwinboxEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
