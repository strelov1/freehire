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

func polymerFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/polymer/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// polymerMantel routes the real Mantel Capture board captured 2026-10-24: five postings, of
// which 41420 is answered as gone (the platform's 422) and 41421 as unreadable (a 500).
func polymerMantel(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		routeErr("/jobs/41420", &StatusError{Code: http.StatusUnprocessableEntity}).
		routeErr("/jobs/41421", &StatusError{Code: http.StatusInternalServerError}).
		route("/mantel-capture-inc/jobs/41648", polymerFixture(t, "mantel_detail_41648.json")).
		route("/mantel-capture-inc/jobs/40659", polymerFixture(t, "mantel_detail_40659.json")).
		route("/mantel-capture-inc/jobs/25713", polymerFixture(t, "mantel_detail_25713.json")).
		route("/mantel-capture-inc/jobs?page=1", polymerFixture(t, "mantel_listing.json"))
}

func TestPolymerProvider(t *testing.T) {
	if got := NewPolymer(nil).Provider(); got != "polymer" {
		t.Errorf("Provider() = %q, want polymer", got)
	}
}

func TestPolymerFetchMapsListingAndDetail(t *testing.T) {
	jobs, err := NewPolymer(polymerMantel(t)).Fetch(context.Background(), CompanyEntry{Provider: "polymer", Board: "mantel-capture-inc", Company: "Mantel"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (41420 is gone, 41421 is an unreadable marker): %v", len(jobs), byID)
	}
	if _, ok := byID["41420"]; ok {
		t.Error("a posting the platform answers 422 for must be dropped")
	}
	if !byID["41421"].Unreadable {
		t.Error("a posting whose detail failed must be yielded as unreadable, not dropped")
	}

	remote := byID["41648"]
	if remote.URL != "https://jobs.polymer.co/mantel-capture-inc/41648" {
		t.Errorf("URL = %q", remote.URL)
	}
	if remote.Title != "Director of Product Engineering" || remote.Company != "Mantel Capture, Inc" {
		t.Errorf("Title/Company = %q/%q", remote.Title, remote.Company)
	}
	if remote.WorkMode != "remote" || !remote.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want remote/true", remote.WorkMode, remote.Remote)
	}
	if !slices.Equal(remote.Countries, []string{"ca", "us"}) {
		t.Errorf("Countries = %v, want the residency list [ca us]", remote.Countries)
	}
	if remote.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", remote.EmploymentType)
	}
	if remote.Description == "" || strings.Contains(remote.Description, "<script") {
		t.Errorf("Description = %q, want the sanitized detail body", remote.Description)
	}
	if remote.PostedAt == nil || remote.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v", remote.PostedAt)
	}

	onsite := byID["40659"]
	if onsite.WorkMode != "onsite" || onsite.Location != "Cambridge, MA, US" || !slices.Equal(onsite.Countries, []string{"us"}) {
		t.Errorf("onsite = %q/%q/%v, want onsite/Cambridge, MA, US/[us]", onsite.WorkMode, onsite.Location, onsite.Countries)
	}
	// "Remote friendly" names no arrangement: the description decides, and a posting that is
	// not remote takes its office's country.
	if friendly := byID["25713"]; friendly.WorkMode != "" || friendly.Remote || !slices.Equal(friendly.Countries, []string{"us"}) {
		t.Errorf("Remote friendly = %q/%v/%v, want \"\"/false/[us]", friendly.WorkMode, friendly.Remote, friendly.Countries)
	}
}

func TestPolymerUnknownBoardIsGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("/nope/jobs", &StatusError{Code: http.StatusUnprocessableEntity})
	_, err := NewPolymer(fake).Fetch(context.Background(), CompanyEntry{Board: "nope", Company: "x"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

// The walk must reach the count the board states: a pager that stops short would let the
// board-scoped sweep close the postings it never listed.
func TestPolymerListingShortOfItsCountFails(t *testing.T) {
	fake := (&routedHTTP{}).route("/acme/jobs?page=1",
		`{"items":[{"id":1,"title":"A"}],"meta":{"total":1,"is_last":true,"count":2}}`)
	if _, err := NewPolymer(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"}); err == nil {
		t.Fatal("a listing that read fewer postings than it states must fail")
	}
}

func TestPolymerListingWithoutItemsOrMetaFails(t *testing.T) {
	for name, body := range map[string]string{
		"no items": `{"meta":{"total":1,"is_last":true,"count":0}}`,
		"no count": `{"items":[{"id":1,"title":"A"}],"meta":{"total":1,"is_last":true}}`,
		"no total": `{"items":[{"id":1,"title":"A"}],"meta":{"is_last":false,"count":1}}`,
	} {
		fake := (&routedHTTP{}).route("/acme/jobs/1", `{"description":"<p>x</p>"}`).route("/acme/jobs?page=1", body)
		if _, err := NewPolymer(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"}); err == nil {
			t.Errorf("%s: a reshaped page must fail, not pass for a complete board", name)
		}
	}
}

func TestPolymerEmptyBoardIsEmpty(t *testing.T) {
	fake := (&routedHTTP{}).route("/acme/jobs?page=1", `{"items":[],"meta":{"total":0,"is_last":true,"count":0}}`)
	jobs, err := NewPolymer(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs, %v; want an empty board", len(jobs), err)
	}
}

func TestPolymerWalksPages(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/acme/jobs/1", `{"description":"<p>One</p>","kind":"contract"}`).
		route("/acme/jobs/2", `{"description":"<p>Two</p>","kind":"part_time_or_full_time"}`).
		route("/acme/jobs?page=1", `{"items":[{"id":1,"title":"A"}],"meta":{"total":2,"is_last":false,"count":2}}`).
		route("/acme/jobs?page=2", `{"items":[{"id":2,"title":"B"}],"meta":{"total":2,"is_last":true,"count":2}}`)
	jobs, err := NewPolymer(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || jobs[0].EmploymentType != "contract" || jobs[1].EmploymentType != "" {
		t.Fatalf("jobs = %+v", jobs)
	}
	if jobs[0].Company != "Acme" {
		t.Errorf("Company = %q, want the board's when the posting names none", jobs[0].Company)
	}
}

func TestPolymerRejectsMalformedBoard(t *testing.T) {
	if _, err := NewPolymer(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Fatal("a board that is not a slug must be refused before any request")
	}
}
