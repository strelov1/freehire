package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// concludisFixture reads a page captured verbatim from hhla.concludis.de (2026-10-25): the
// complete list's two pages ("46 Stellen gefunden", 25 + 21 lines), what ?page=3 answers (the
// last page again), one posting page, and dlr.concludis.de's list, which has no opening.
func concludisFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/concludis/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func concludisHHLA(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route("GesamtlisteOffenePositionen.htm?page=1", concludisFixture(t, "list_hhla_p1.html")).
		route("GesamtlisteOffenePositionen.htm?page=2", concludisFixture(t, "list_hhla_p2.html")).
		route("GesamtlisteOffenePositionen.htm?page=3", concludisFixture(t, "list_hhla_past_end.html"))
}

func TestConcludisProvider(t *testing.T) {
	s, ok := All(nil)["concludis"]
	if !ok || s.Provider() != "concludis" {
		t.Fatal("All() must register concludis")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("concludis must be a HydratingSource: the body costs a page request")
	}
	if !FullBoardListingProviders(All(nil))["concludis"] {
		t.Error("concludis checks its walk against the stated count: it must be fullBoardListing")
	}
}

func TestConcludisWalksToTheStatedCount(t *testing.T) {
	fake := concludisHHLA(t)
	jobs, err := NewConcludis(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "HHLA", Provider: "concludis", Board: "hhla"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 46 {
		t.Fatalf("len(jobs) = %d, want the 46 positions the list states", len(jobs))
	}
	if fake.calls != 2 {
		t.Errorf("requests = %d, want the two list pages alone (the count ends the walk)", fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "https://hhla.concludis.de/prj/shw/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

// A walk that runs out of new lines before the stated count is a truncation, never a board
// that shrank: page 2 failing to add anything must fail the crawl.
func TestConcludisShortWalkFails(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", concludisFixture(t, "list_hhla_p1.html")).
		route("?page=2", concludisFixture(t, "list_hhla_p1.html"))
	if _, err := NewConcludis(fake).Fetch(context.Background(), CompanyEntry{Provider: "concludis", Board: "hhla"}); err == nil {
		t.Fatal("a walk short of the stated count must fail")
	}
}

func TestConcludisMapsAPostingPage(t *testing.T) {
	fake := concludisHHLA(t).route("/prj/shw/0b94ce08688c6389ce7b68c52ce3f8c7_0/6635/", concludisFixture(t, "detail_hhla.html"))
	jobs, err := NewConcludis(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "HHLA", Provider: "concludis", Board: "hhla"},
		func(id string) bool { return id != "6635" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var j *Job
	for i := range jobs {
		if jobs[i].ExternalID == "6635" {
			j = &jobs[i]
		}
	}
	if j == nil || j.SeenRefresh || j.Description == "" {
		t.Fatalf("6635 = %+v, want a hydrated posting", j)
	}
	if j.Title != "Assistenz - Teamleitung Rangierbetrieb" || j.Company != "METRANS Rail (Deutschland) GmbH" {
		t.Errorf("Title/Company = %q/%q, want the posting's own employer", j.Title, j.Company)
	}
	if j.Location != "Hamburg" || !slices.Equal(j.Countries, []string{"de"}) {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" || j.Remote || j.WorkMode != "" {
		t.Errorf("EmploymentType/Remote/WorkMode = %q/%v/%q", j.EmploymentType, j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-03-18" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if !strings.Contains(j.URL, "lang=de_DE") || strings.Contains(j.URL, "&amp;") {
		t.Errorf("URL = %q, want the line's address, entity-decoded", j.URL)
	}
}

// A posting the tenant publishes on its own site redirects there; with no JobPosting on that
// page it is unreadable by configuration, not by accident, so it is dropped rather than marked
// (a marker would recur every run and hold the board's sweep shut). A page on the tenant's own
// host missing its JobPosting is still a marker.
func TestConcludisPostingRedirectedAwayIsDropped(t *testing.T) {
	fake := concludisHHLA(t).
		routeRedirect("/6635/", "https://www.hhla.de/karriere/jobs#job=6635").
		route("/prj/shw/", "<html><body>no posting here</body></html>")
	jobs, err := NewConcludis(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "HHLA", Provider: "concludis", Board: "hhla"},
		func(id string) bool { return id != "6635" && id != "7339" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var redirected, onHost *Job
	for i := range jobs {
		switch jobs[i].ExternalID {
		case "6635":
			redirected = &jobs[i]
		case "7339":
			onHost = &jobs[i]
		}
	}
	if redirected != nil {
		t.Errorf("redirected posting = %+v, want it dropped", *redirected)
	}
	if onHost == nil || !onHost.Unreadable {
		t.Errorf("on-host posting without a JobPosting = %+v, want an unreadable marker", onHost)
	}
}

func TestConcludisNoOpeningsIsAnEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("dlr.concludis.de", concludisFixture(t, "list_empty.html"))
	jobs, err := NewConcludis(fake).Fetch(context.Background(), CompanyEntry{Provider: "concludis", Board: "dlr"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs/err = %d/%v, want an empty board", len(jobs), err)
	}
}

// A tenant that moved its jobs to its own site redirects the list there: nothing this adapter
// reads is served any more.
func TestConcludisRedirectAwayIsGone(t *testing.T) {
	fake := (&routedHTTP{}).routeRedirect("edeka.concludis.de", "https://verbund.edeka/karriere/stellenb%C3%B6rse/")
	_, err := NewConcludis(fake).Fetch(context.Background(), CompanyEntry{Provider: "concludis", Board: "edeka"})
	if !errors.Is(err, ErrBoardGone) || !strings.Contains(err.Error(), "verbund.edeka") {
		t.Fatalf("err = %v, want ErrBoardGone naming the destination", err)
	}
}

// A page stating neither a count nor the no-openings notice is a reshaped page.
func TestConcludisPageWithoutCountFails(t *testing.T) {
	fake := (&routedHTTP{}).route("hhla.concludis.de", "<html><body><div class=\"stellen list\"></div></body></html>")
	if _, err := NewConcludis(fake).Fetch(context.Background(), CompanyEntry{Provider: "concludis", Board: "hhla"}); err == nil {
		t.Fatal("a list page without a count must fail, not read as an empty board")
	}
}

func TestConcludisEmploymentTypeMapsOnlyASingleValue(t *testing.T) {
	for in, want := range map[string]string{
		`"FULL_TIME"`: "full_time", `["PART_TIME"]`: "part_time", `"OTHER"`: "",
		`["FULL_TIME","TEMPORARY"]`: "", `null`: "",
	} {
		if got := concludisEmploymentType([]byte(in)); got != want {
			t.Errorf("concludisEmploymentType(%s) = %q, want %q", in, got, want)
		}
	}
}
