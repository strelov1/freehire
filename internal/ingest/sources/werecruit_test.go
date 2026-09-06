package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// werecruitListingHTML mirrors a tenant's listing page: window.allOffers embeds the tenant's
// whole open-postings list server-side, no pagination.
const werecruitListingHTML = `<html><body><script>
window.allOffers = [{"Id":"49895d7f-fe57-4d0d-92d7-126837be0ea3","TitleTranslated":"Postdoctoral Researcher","Url":"https://careers.werecruit.io/en/idiap/offers/postdoctoral-researcher-49895d","Address_City":"Martigny","Address_Region":"Valais","Address_State":"CH","TimeTranslated":"Full time","PublicationStartDate":"2026-08-20T15:07:34.2606458+00:00"}];
window.offersConfig = {"locale":"en-gb"};
</script></body></html>`

// werecruitDetailHTML mirrors a posting's own page: the body lives in a server-rendered
// "description" block, alongside an unrelated "description-blocks" div that must not be
// mistaken for it (an exact class-token match, not a prefix one).
const werecruitDetailHTML = `<html><body>
<div class="description rich-text border-color1 mb-5"><p>Join our research team.</p><script>alert(1)</script></div>
<div class="description-blocks rgpd-block"><p>Unrelated GDPR block.</p></div>
</body></html>`

func newWerecruitFake() *routedHTTP {
	// The detail route is registered first: routedHTTP matches by URL substring in
	// registration order, and the listing URL ("/en/idiap") is itself a prefix of the detail
	// URL, so the more specific route must be checked first or every request would answer with
	// the listing page.
	return (&routedHTTP{}).
		route("/offers/postdoctoral-researcher-49895d", werecruitDetailHTML).
		route("/en/idiap", werecruitListingHTML)
}

func TestWerecruitProvider(t *testing.T) {
	if got := NewWerecruit(nil).Provider(); got != "werecruit" {
		t.Errorf("Provider() = %q, want %q", got, "werecruit")
	}
}

func TestWerecruitFetchListsAndHydrates(t *testing.T) {
	jobs, err := NewWerecruit(newWerecruitFake()).Fetch(context.Background(),
		CompanyEntry{Company: "IDIAP", Board: "en/idiap"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "49895d7f-fe57-4d0d-92d7-126837be0ea3" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "Postdoctoral Researcher" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "IDIAP" {
		t.Errorf("Company = %q, want the board's configured company", j.Company)
	}
	if j.Location != "Martigny, Valais" {
		t.Errorf("Location = %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "ch" {
		t.Errorf("Countries = %v, want [ch]", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if want := "https://careers.werecruit.io/en/idiap/offers/postdoctoral-researcher-49895d"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt not parsed")
	}
	if !strings.Contains(j.Description, "Join our research team.") {
		t.Errorf("Description = %q, want it to contain the body", j.Description)
	}
	if strings.Contains(j.Description, "alert(1)") || strings.Contains(j.Description, "Unrelated GDPR") {
		t.Errorf("Description not sanitized/scoped: %q", j.Description)
	}
}

func TestWerecruitFetchUnconfiguredLocaleIsEmpty(t *testing.T) {
	// /fr/idiap is not routed at all here, the same shape the platform answers for a locale a
	// tenant never configured (an empty listing, not an error) — modeled as a listing page with
	// no allOffers assignment at all.
	fake := (&routedHTTP{}).route("/fr/idiap", `<html><body>no offers here</body></html>`)
	jobs, err := NewWerecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "fr/idiap"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestWerecruitFetchEmptyOffersArray(t *testing.T) {
	fake := (&routedHTTP{}).route("/en/nobody", `<html><body><script>window.allOffers = [];</script></body></html>`)
	jobs, err := NewWerecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "en/nobody"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestWerecruitListingTransportErrorFailsBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("/en/idiap", errors.New("werecruit_test: unreachable"))
	if _, err := NewWerecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "en/idiap"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestWerecruitDropsPostingWhoseDetailFetchFails(t *testing.T) {
	// The listing succeeds but the posting's own page errors; that one posting is dropped
	// rather than stored with an empty body.
	fake := (&routedHTTP{}).
		route("/en/idiap", werecruitListingHTML).
		routeErr("/offers/", errors.New("werecruit_test: detail unreachable"))
	jobs, err := NewWerecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "en/idiap"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (detail fetch failed, posting dropped)", len(jobs))
	}
}

func TestWerecruitBoardValidation(t *testing.T) {
	cases := []struct {
		board   string
		wantErr bool
	}{
		{"en/idiap", false},
		{"", true},
		{"en", true},
		{"en/", true},
		{"/idiap", true},
	}
	for _, c := range cases {
		_, _, err := parseWerecruitBoard(c.board)
		if (err != nil) != c.wantErr {
			t.Errorf("parseWerecruitBoard(%q) error = %v, wantErr %v", c.board, err, c.wantErr)
		}
	}
}

func TestWerecruitMalformedBoardIssuesNoRequest(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewWerecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "not-well-formed"}); err == nil {
		t.Fatal("Fetch: want error for malformed board, got nil")
	}
}

func TestWerecruitRegisteredInAll(t *testing.T) {
	if _, ok := All(nil)["werecruit"]; !ok {
		t.Fatal(`All(nil)["werecruit"] missing`)
	}
}
