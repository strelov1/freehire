package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// homerunFixture reads a document captured verbatim on 2026-10-25: autokan's feed (four
// vacancies on the employer's own domain, werkenbijautokan.nl, two of them located by the
// employer as "Hybride" and "Remote") with each vacancy's page; bluegem's lobster-consultant, a
// page whose JobPosting states TELECOMMUTE; and acato's new-business-developer, whose office has
// no address and whose page carries an empty ld+json block.
func homerunFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/homerun/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

var homerunAutokanPages = []string{"sales-ondersteuning", "front-end-developer", "student-bijbaan-3", "sales-medewerker-remote-3"}

func homerunAutokanHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, p := range homerunAutokanPages {
		r.route("www.werkenbijautokan.nl/"+p, homerunFixture(t, "posting_autokan_"+p+".html"))
	}
	return r.route("feed.homerun.co/autokan", homerunFixture(t, "feed_autokan.xml"))
}

var homerunAutokanEntry = CompanyEntry{Company: "Autokan", Provider: "homerun", Board: "autokan"}

func TestHomerunProvider(t *testing.T) {
	s, ok := All(nil)["homerun"]
	if !ok || s.Provider() != "homerun" {
		t.Fatal("All() must register homerun")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("homerun must be a HydratingSource: the address costs a page request")
	}
	if !FullBoardListingProviders(All(nil))["homerun"] {
		t.Error("homerun's feed is one unpaginated document: it must be fullBoardListing")
	}
}

func TestHomerunFeedRefreshesSeenVacancies(t *testing.T) {
	fake := homerunAutokanHTTP(t)
	jobs, err := NewHomerun(fake).(HydratingSource).FetchNew(context.Background(), homerunAutokanEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the feed's 4 vacancies from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || !strings.HasPrefix(j.ExternalID, "job_") || !strings.HasPrefix(j.URL, "https://www.werkenbijautokan.nl/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestHomerunMapsAFeedEntryAndItsPage(t *testing.T) {
	jobs, err := NewHomerun(homerunAutokanHTTP(t)).Fetch(context.Background(), homerunAutokanEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 4 {
		t.Fatalf("jobs = %d, want 4", len(byID))
	}
	f := byID["job_rqQlBEIMndKkNPX3gyPm"]
	if f.Title != "Frontend Developer (Medior / Senior)" || f.URL != "https://www.werkenbijautokan.nl/front-end-developer" || f.Company != "Autokan" {
		t.Errorf("Title/URL/Company = %q/%q/%q", f.Title, f.URL, f.Company)
	}
	// The employer's name for the office stays; the page's address adds the town and country.
	if f.Location != "Hybride; Groningen, Groningen" || !slices.Equal(f.Countries, []string{"nl"}) {
		t.Errorf("Location/Countries = %q/%v", f.Location, f.Countries)
	}
	if f.Description == "" || strings.Contains(f.Description, "<script") {
		t.Errorf("Description = %.200q, want the feed's body", f.Description)
	}
	if f.EmploymentType != "full_time" || f.Remote || f.WorkMode != "" {
		t.Errorf("EmploymentType/Remote/WorkMode = %q/%v/%q", f.EmploymentType, f.Remote, f.WorkMode)
	}
	if f.PostedAt == nil || f.PostedAt.Format("2006-01-02 15:04") != "2025-05-21 11:17" {
		t.Errorf("PostedAt = %v", f.PostedAt)
	}
	// An office named after its town is not repeated.
	if s := byID["job_MBIMyGSONWuZ6DEp5qPx"]; s.Location != "Groningen" || s.EmploymentType != "part_time" {
		t.Errorf("student Location/EmploymentType = %q/%q", s.Location, s.EmploymentType)
	}
}

func TestHomerunPageStatesTelecommute(t *testing.T) {
	root, err := html.Parse(strings.NewReader(homerunFixture(t, "posting_bluegem_lobster-consultant.html")))
	if err != nil {
		t.Fatal(err)
	}
	j := Job{Location: "Groningen"}
	homerunAddPage(&j, root)
	if !j.Remote || j.WorkMode != "remote" || !slices.Equal(j.Countries, []string{"nl"}) || j.Location != "Groningen" {
		t.Errorf("job = %+v, want remote, nl, the location unchanged", j)
	}
}

func TestHomerunPageWithoutJobPostingLeavesTheEntry(t *testing.T) {
	root, err := html.Parse(strings.NewReader(homerunFixture(t, "posting_acato_new-business-developer.html")))
	if err != nil {
		t.Fatal(err)
	}
	j := Job{Location: "Other"}
	homerunAddPage(&j, root)
	if j.Location != "Other" || len(j.Countries) != 0 || j.Remote {
		t.Errorf("job = %+v, want the feed's entry untouched", j)
	}
}

// The page is enrichment: a refused one leaves the feed's entry standing, while a 404 is
// Homerun's own answer for a vacancy the feed still lists after it was taken down.
func TestHomerunPageFailures(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("www.werkenbijautokan.nl/sales-ondersteuning", &StatusError{Method: http.MethodGet, Code: http.StatusTooManyRequests}).
		routeErr("www.werkenbijautokan.nl/front-end-developer", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route("www.werkenbijautokan.nl/", homerunFixture(t, "posting_autokan_student-bijbaan-3.html")).
		route("feed.homerun.co/autokan", homerunFixture(t, "feed_autokan.xml"))
	jobs, err := NewHomerun(fake).Fetch(context.Background(), homerunAutokanEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if r := byID["job_W4O6B45ovsKe11fvXCMV"]; r.Unreadable || r.Description == "" || r.Location != "Groningen" || len(r.Countries) != 0 {
		t.Errorf("refused page = %+v, want the feed's entry alone", r)
	}
	if _, ok := byID["job_rqQlBEIMndKkNPX3gyPm"]; ok {
		t.Error("a 404 on the page must drop the vacancy")
	}
	if len(byID) != 3 {
		t.Errorf("jobs = %d, want 3", len(byID))
	}
}

func TestHomerunEmptyAndGoneTenants(t *testing.T) {
	empty := (&routedHTTP{}).route("feed.homerun.co/quiet",
		`<?xml version="1.0" encoding="UTF-8" ?><feed xmlns="http://www.w3.org/2005/Atom"><title type="text">Quiet</title></feed>`)
	if jobs, err := NewHomerun(empty).Fetch(context.Background(), CompanyEntry{Provider: "homerun", Board: "quiet"}); err != nil || len(jobs) != 0 {
		t.Errorf("empty feed: jobs/err = %d/%v, want an empty board", len(jobs), err)
	}
	gone := (&routedHTTP{}).routeErr("feed.homerun.co/nope", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	if _, err := NewHomerun(gone).Fetch(context.Background(), CompanyEntry{Provider: "homerun", Board: "nope"}); !errors.Is(err, ErrBoardGone) {
		t.Errorf("gone feed: err = %v, want ErrBoardGone", err)
	}
	reshaped := (&routedHTTP{}).route("feed.homerun.co/x", `<html><body>maintenance</body></html>`)
	if _, err := NewHomerun(reshaped).Fetch(context.Background(), CompanyEntry{Provider: "homerun", Board: "x"}); err == nil {
		t.Error("an answer that is not the feed must fail, not read as an empty board")
	}
}
