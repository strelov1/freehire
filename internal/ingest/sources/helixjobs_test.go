package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

// helixjobsFixture reads a page captured verbatim from helixjobs.com (2026-10-25): apetito's
// list (iid 146, "Suchergebnis: 62 Treffer") over three pages of 25 + 25 + 12 lines, what
// ?page=4 answers (no line), one posting page, and iid 1's list, which has no posting.
func helixjobsFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/helixjobs/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func helixjobsApetito(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route("iid=146&page=1", helixjobsFixture(t, "list_146_p1.html")).
		route("iid=146&page=2", helixjobsFixture(t, "list_146_p2.html")).
		route("iid=146&page=3", helixjobsFixture(t, "list_146_p3.html")).
		route("iid=146&page=4", helixjobsFixture(t, "list_146_p4.html"))
}

func TestHelixjobsProvider(t *testing.T) {
	s, ok := All(nil)["helixjobs"]
	if !ok || s.Provider() != "helixjobs" {
		t.Fatal("All() must register helixjobs")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("helixjobs must be a HydratingSource: the body costs a page request")
	}
	if !FullBoardListingProviders(All(nil))["helixjobs"] {
		t.Error("helixjobs checks its walk against the stated count: it must be fullBoardListing")
	}
}

func TestHelixjobsWalksToTheStatedCount(t *testing.T) {
	fake := helixjobsApetito(t)
	jobs, err := NewHelixjobs(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "apetito AG", Provider: "helixjobs", Board: "146"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 62 {
		t.Fatalf("len(jobs) = %d, want the 62 postings the list states", len(jobs))
	}
	if fake.calls != 3 {
		t.Errorf("requests = %d, want the three list pages alone (the count ends the walk)", fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || !strings.HasPrefix(j.ExternalID, "146P") || j.Title == "" ||
			j.URL != "https://helixjobs.com/_/jobad?prj="+j.ExternalID {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

// A walk that runs out of new lines before the stated count is a truncation, never a board
// that shrank.
func TestHelixjobsShortWalkFails(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", helixjobsFixture(t, "list_146_p1.html")).
		route("page=2", helixjobsFixture(t, "list_146_p4.html"))
	if _, err := NewHelixjobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "helixjobs", Board: "146"}); err == nil {
		t.Fatal("a walk short of the stated count must fail")
	}
}

func TestHelixjobsMapsAPostingPage(t *testing.T) {
	fake := helixjobsApetito(t).route("jobad?prj=146P13181", helixjobsFixture(t, "detail_146P13181.html"))
	jobs, err := NewHelixjobs(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "apetito", Provider: "helixjobs", Board: "146"},
		func(id string) bool { return id != "146P13181" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	i := slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "146P13181" })
	if i < 0 || jobs[i].SeenRefresh || jobs[i].Description == "" {
		t.Fatalf("146P13181 missing or not hydrated: %+v", jobs)
	}
	j := jobs[i]
	if !strings.HasPrefix(j.Title, "Mitarbeiter als Kommissionierer und Fahrer") || j.Company != "apetito AG" {
		t.Errorf("Title/Company = %q/%q, want the posting's own", j.Title, j.Company)
	}
	if j.Location != "Hamburg" || !slices.Equal(j.Countries, []string{"de"}) {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" || j.Remote || j.WorkMode != "" {
		t.Errorf("EmploymentType/Remote/WorkMode = %q/%v/%q", j.EmploymentType, j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-16" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

// A posting the portal relays to another platform redirects there and is dropped; a portal
// page missing its JobPosting is still a marker.
func TestHelixjobsRelayedPostingIsDropped(t *testing.T) {
	fake := helixjobsApetito(t).
		routeRedirect("prj=146P13181", "https://spardabe.pi-asp.de/bewerber-web/?company=50001-FIRMA-ID").
		route("jobad?prj=", "<html><body>no posting here</body></html>")
	jobs, err := NewHelixjobs(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "apetito", Provider: "helixjobs", Board: "146"},
		func(id string) bool { return id != "146P13181" && id != "146P13793" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if slices.ContainsFunc(jobs, func(j Job) bool { return j.ExternalID == "146P13181" }) {
		t.Error("a posting relayed off the portal must be dropped")
	}
	i := slices.IndexFunc(jobs, func(j Job) bool { return j.ExternalID == "146P13793" })
	if i < 0 || !jobs[i].Unreadable {
		t.Errorf("portal page without a JobPosting must be an unreadable marker, got %+v", jobs)
	}
}

func TestHelixjobsNoResultsIsAnEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("iid=1&", helixjobsFixture(t, "list_empty.html"))
	jobs, err := NewHelixjobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "helixjobs", Board: "1"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs/err = %d/%v, want an empty board", len(jobs), err)
	}
}

// A page stating neither a count nor the no-results notice is reshaped, never empty: an empty
// answer would let the sweep close the whole board.
func TestHelixjobsReshapedListFails(t *testing.T) {
	fake := (&routedHTTP{}).route("iid=146", "<html><body><ul></ul></body></html>")
	if _, err := NewHelixjobs(fake).Fetch(context.Background(), CompanyEntry{Provider: "helixjobs", Board: "146"}); err == nil {
		t.Fatal("a list page with no count must fail")
	}
}

func TestHelixjobsRejectsANonIIDBoard(t *testing.T) {
	if _, err := NewHelixjobs(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Provider: "helixjobs", Board: "apetito-ag"}); err == nil {
		t.Fatal("a board that is not a numeric iid must fail")
	}
}

func TestHelixjobsPostingIDFoldsTheSeparator(t *testing.T) {
	for href, want := range map[string]string{
		"/_/jobad?prj=146P13181":                  "146P13181",
		"/_/jobad?prj=100007p36&source=ba":        "100007P36",
		"/_/apply/form?prj=2728P663":              "",
		"/_/jobad?prj=abc":                        "",
		"https://x.helixjobs.com/_/jobad?prj=5P1": "5P1",
	} {
		if got := helixjobsPostingID(href); got != want {
			t.Errorf("helixjobsPostingID(%q) = %q, want %q", href, got, want)
		}
	}
}
