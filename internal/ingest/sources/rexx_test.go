package sources

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

func rexxFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/rexx/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// rexxZieglersche routes the real Zieglersche portal captured 2026-10-25 (styles and scripts
// stripped): 108 postings over two listing pages, an empty third page, and one posting page.
func rexxZieglersche(t *testing.T) *routedHTTP {
	const listing = "zieglersche-portal.rexx-recruitment.com/stellenangebote.html?start="
	return (&routedHTTP{}).
		route(listing+"0", rexxFixture(t, "zieglersche_start0.html")).
		route(listing+"100", rexxFixture(t, "zieglersche_start100.html")).
		route(listing+"200", rexxFixture(t, "zieglersche_start200.html")).
		route("-de-j3040.html", rexxFixture(t, "zieglersche_detail_3040.html"))
}

func TestRexxProvider(t *testing.T) {
	if got := NewRexx(nil).Provider(); got != "rexx" {
		t.Errorf("Provider() = %q, want rexx", got)
	}
}

func TestRexxWalksListingAndHydratesNewPosting(t *testing.T) {
	fake := rexxZieglersche(t)
	jobs, err := NewRexx(fake).(rexx).FetchNew(context.Background(),
		CompanyEntry{Provider: "rexx", Board: "zieglersche-portal.rexx-recruitment.com", Company: "Die Zieglerschen"},
		func(id string) bool { return id != "3040" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// Every posting on both pages, each once, though every page minted a fresh session id.
	if len(jobs) != 108 {
		t.Fatalf("got %d jobs, want 108", len(jobs))
	}
	var hydrated *Job
	for i, j := range jobs {
		if strings.Contains(j.URL, "sid=") {
			t.Fatalf("URL %q keeps the session id", j.URL)
		}
		if !j.SeenRefresh {
			hydrated = &jobs[i]
		}
	}
	if hydrated == nil {
		t.Fatal("the unseen posting was not hydrated")
	}
	j := *hydrated
	// Linked on the employer's own domain, which is where the posting lives.
	if j.ExternalID != "3040" || j.URL != "https://jobs.zieglersche.de/Altenpflegehelfer-mwd-de-j3040.html" {
		t.Errorf("id/URL = %q/%q", j.ExternalID, j.URL)
	}
	if j.Title != "Altenpflegehelfer (m/w/d)" || j.Company != "Die Zieglerschen" {
		t.Errorf("Title/Company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Bad Waldsee, Baden-Württemberg, DE" || len(j.Countries) != 1 || j.Countries[0] != "de" {
		t.Errorf("Location/Countries = %q/%v", j.Location, j.Countries)
	}
	if j.EmploymentType != "full_time" || j.WorkMode != "" {
		t.Errorf("EmploymentType/WorkMode = %q/%q", j.EmploymentType, j.WorkMode)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-23" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	// The JobPosting description already carries the page's sections.
	if !strings.Contains(j.Description, "Altenpflegehelfer") || len(j.Description) < 1000 {
		t.Errorf("Description too thin: %d bytes", len(j.Description))
	}
}

// The portal's name is not the employer: hiringOrganization reads "... GmbH Jobportal".
func TestRexxKeepsCatalogueEmployer(t *testing.T) {
	fake := (&routedHTTP{}).route("-j3700.html", rexxFixture(t, "hse24_detail_3700.html"))
	j, ok := NewRexx(fake).(rexx).detail(context.Background(), CompanyEntry{Company: "HSE"},
		rexxLink{id: "3700", url: "https://karriere-hse24-portal.rexx-systems.com/Head-of-IT-de-j3700.html"})
	if !ok || j.Unreadable {
		t.Fatalf("detail: ok=%v unreadable=%v", ok, j.Unreadable)
	}
	if j.Company != "HSE" || j.Title != "Head of IT & Media Technology (m/w/d)" {
		t.Errorf("Company/Title = %q/%q", j.Company, j.Title)
	}
}

// A portal that renders no JobPosting (voss) and a failed request are both unreadable, so the
// posting is protected from the sweep rather than silently missing. A 404 is the portal's
// answer that the posting is gone.
func TestRexxDetailFailures(t *testing.T) {
	fake := (&routedHTTP{}).
		route("-j2630.html", rexxFixture(t, "voss_detail_2630.html")).
		routeErr("-j1.html", &StatusError{Code: http.StatusServiceUnavailable}).
		routeErr("-j2.html", &StatusError{Code: http.StatusNotFound})
	s := NewRexx(fake).(rexx)
	for _, id := range []string{"2630", "1"} {
		j, ok := s.detail(context.Background(), CompanyEntry{}, rexxLink{id: id, url: "https://x.rexx-systems.com/a-de-j" + id + ".html"})
		if !ok || !j.Unreadable || j.ExternalID != id {
			t.Errorf("%s: got ok=%v %+v, want an unreadable marker", id, ok, j)
		}
	}
	if _, ok := s.detail(context.Background(), CompanyEntry{}, rexxLink{id: "2", url: "https://x.rexx-systems.com/a-de-j2.html"}); ok {
		t.Error("a 404 posting must be dropped")
	}
}

// A later listing page failing fails the board: the postings past it would otherwise read as
// unseen and be closed.
func TestRexxLaterPageFailureFailsBoard(t *testing.T) {
	fake := rexxZieglersche(t).routeErr("start=100", &StatusError{Code: http.StatusBadGateway})
	if _, err := NewRexx(fake).Fetch(context.Background(), CompanyEntry{Board: "zieglersche-portal.rexx-recruitment.com"}); err == nil {
		t.Fatal("a failed second page passed as a complete listing")
	}
}

// A later page that is not a rexx listing (a maintenance page under HTTP 200) must not read as
// the listing's end.
func TestRexxForeignPageFailsBoard(t *testing.T) {
	const listing = "zieglersche-portal.rexx-recruitment.com/stellenangebote.html?start="
	fake := (&routedHTTP{}).
		route(listing+"0", rexxFixture(t, "zieglersche_start0.html")).
		route(listing+"100", "<html><body><h1>Wartungsarbeiten</h1></body></html>")
	if _, err := NewRexx(fake).Fetch(context.Background(), CompanyEntry{Board: "zieglersche-portal.rexx-recruitment.com"}); err == nil {
		t.Fatal("a maintenance page passed as the end of the listing")
	}
}

func TestRexxBoardShapes(t *testing.T) {
	// A portal under a path on the employer's own domain is listed beneath that path.
	fake := (&routedHTTP{}).route("jobs.newyorker.de/karriere/stellenangebote.html?start=0", `<html><body><a href="https://www.rexx-systems.com">rexx</a></body></html>`)
	if _, err := NewRexx(fake).Fetch(context.Background(), CompanyEntry{Board: "Jobs.NewYorker.de/karriere/"}); err != nil {
		t.Errorf("host/path board: %v", err)
	}
	for _, b := range []string{"zieglersche", "https://x.rexx-systems.com", "x.rexx-systems.com/a?b=c", ""} {
		if _, err := NewRexx(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
}
