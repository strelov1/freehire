package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSelfRecruitProvider(t *testing.T) {
	if got := NewSelfRecruit(nil).Provider(); got != "selfrecruit" {
		t.Errorf("Provider() = %q, want %q", got, "selfrecruit")
	}
}

func TestSelfRecruitJobID(t *testing.T) {
	cases := map[string]string{
		"https://dressup.selfrecruit.ge/a7cdcc00-1c9c-464c-8960-945af0c0e0a4":            "a7cdcc00-1c9c-464c-8960-945af0c0e0a4",
		"/a7cdcc00-1c9c-464c-8960-945af0c0e0a4":                                          "a7cdcc00-1c9c-464c-8960-945af0c0e0a4",
		"https://dressup.selfrecruit.ge/a7cdcc00-1c9c-464c-8960-945af0c0e0a4?tmpl=modal": "a7cdcc00-1c9c-464c-8960-945af0c0e0a4",
		// A CMS "article" page shares the site but is not a posting.
		"https://dressup.selfrecruit.ge/articles/09f57c76-bd0c-465c-982b-1dc644d0c3e4": "",
		"https://dressup.selfrecruit.ge/cv":                                            "", // static page, not a posting
		"https://dressup.selfrecruit.ge/vacancies/10":                                  "", // pagination link, not a posting
		"https://dressup.selfrecruit.ge/":                                              "",
		"https://dressup.selfrecruit.ge":                                               "",
	}
	for loc, want := range cases {
		if got := selfrecruitJobID(loc); got != want {
			t.Errorf("selfrecruitJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func selfrecruitListingHTML(uuids ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><a href="/cv">Submit CV</a>`)
	for _, u := range uuids {
		b.WriteString(`<a href="https://dressup.selfrecruit.ge/` + u + `">A Role</a>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

const selfrecruitEmptyListingHTML = `<html><body><a href="/cv">Submit CV</a></body></html>`

func selfrecruitDetailHTML(title, description string) string {
	return `<html><body>
<div class="vacancy_title_inner">` + title + `</div>
<div class="pub_vac_text_detail">` + description + `</div>
</body></html>`
}

func TestSelfRecruitFetchListingThenDetailAndMaps(t *testing.T) {
	id := "a7cdcc00-1c9c-464c-8960-945af0c0e0a4"
	fake := (&routedHTTP{}).
		route("https://dressup.selfrecruit.ge/"+id, selfrecruitDetailHTML(
			"Digital Marketing Lead", "<p>Build campaigns.</p><script>alert(1)</script>")).
		route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitListingHTML(id)).
		route("https://dressup.selfrecruit.ge/vacancies/10", selfrecruitEmptyListingHTML)

	jobs, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dressup", Provider: "selfrecruit", Board: "dressup",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, id)
	}
	if j.Title != "Digital Marketing Lead" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Dressup" {
		t.Errorf("Company = %q", j.Company)
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build campaigns") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
}

func TestSelfRecruitPaginatesAcrossPages(t *testing.T) {
	u1, u2, u3 := "11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333"
	fake := (&routedHTTP{}).
		route("https://dressup.selfrecruit.ge/"+u1, selfrecruitDetailHTML("Role 1", "<p>1</p>")).
		route("https://dressup.selfrecruit.ge/"+u2, selfrecruitDetailHTML("Role 2", "<p>2</p>")).
		route("https://dressup.selfrecruit.ge/"+u3, selfrecruitDetailHTML("Role 3", "<p>3</p>")).
		route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitListingHTML(u1, u2)).
		route("https://dressup.selfrecruit.ge/vacancies/10", selfrecruitListingHTML(u3)).
		// Past the end: the platform redirects to the root/offset-0 content, so this page's
		// links are all already seen and the walk stops naturally with no special casing.
		route("https://dressup.selfrecruit.ge/vacancies/20", selfrecruitListingHTML(u1, u2))

	jobs, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "dressup"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (across two listing pages)", len(jobs))
	}
	ids := []string{jobs[0].ExternalID, jobs[1].ExternalID, jobs[2].ExternalID}
	for _, want := range []string{u1, u2, u3} {
		if !slices.Contains(ids, want) {
			t.Errorf("missing job %q in %v", want, ids)
		}
	}
}

// A later-page failure must abort the whole Fetch, never return a partial result as
// success — the property the fullBoardListing marker rests on.
func TestSelfRecruitFetchFailsWholeBoardOnLaterPageError(t *testing.T) {
	u1 := "11111111-1111-4111-8111-111111111111"
	fake := (&routedHTTP{}).
		route("https://dressup.selfrecruit.ge/"+u1, selfrecruitDetailHTML("Role 1", "<p>1</p>")).
		route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitListingHTML(u1)).
		routeErr("https://dressup.selfrecruit.ge/vacancies/10", errors.New("boom"))

	if _, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "dressup"}); err == nil {
		t.Fatal("Fetch succeeded despite a later-page listing error")
	}
}

func TestSelfRecruitRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["selfrecruit"] {
		t.Error("FullBoardListingProviders(All(nil)) should include selfrecruit")
	}
}

func TestSelfRecruitEmptyListingYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitEmptyListingHTML)
	jobs, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{Board: "dressup"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// A page the crawl could not READ must be marked, not dropped: the detail page is this
// adapter's only source for the posting, so a silently dropped one is indistinguishable
// from a posting taken down.
func TestSelfRecruitUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	kept := "a7cdcc00-1c9c-464c-8960-945af0c0e0a4"
	lost := "b8ddee11-2d0d-575b-9071-a56c1b1d5b21"
	fake := (&routedHTTP{}).
		route("https://dressup.selfrecruit.ge/"+kept, selfrecruitDetailHTML("Kept Role", "<p>Body</p>")).
		route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitListingHTML(kept, lost)).
		route("https://dressup.selfrecruit.ge/vacancies/10", selfrecruitEmptyListingHTML)
		// no route for "lost" → GetHTML errors with a transport failure

	jobs, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dressup", Board: "dressup",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != kept {
		t.Fatalf("read = %v, want only the posting whose page answered", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != lost {
		t.Fatalf("unreadable markers = %v, want one for the posting whose page did not", markers)
	}
	if markers[0].Company != "Dressup" {
		t.Errorf("marker Company = %q, want the board's employer", markers[0].Company)
	}
}

// A page that answers successfully but carries neither classed element (a markup change,
// or a page shape this adapter has never observed live) must be marked Unreadable, not
// crash the crawl by calling textContent/innerHTML on the nil node firstByClass returns.
func TestSelfRecruitMissingMarkupIsMarkedUnreadable(t *testing.T) {
	id := "a7cdcc00-1c9c-464c-8960-945af0c0e0a4"
	fake := (&routedHTTP{}).
		route("https://dressup.selfrecruit.ge/"+id, `<html><body><p>no posting markup here</p></body></html>`).
		route("https://dressup.selfrecruit.ge/vacancies/0", selfrecruitListingHTML(id)).
		route("https://dressup.selfrecruit.ge/vacancies/10", selfrecruitEmptyListingHTML)

	jobs, err := NewSelfRecruit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dressup", Board: "dressup",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != id {
		t.Fatalf("unreadable markers = %v, want one for the posting with no classed markup", markers)
	}
}

func TestSelfRecruitRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["selfrecruit"]
	if !ok {
		t.Fatal("All() missing provider selfrecruit")
	}
	if s.Provider() != "selfrecruit" {
		t.Errorf("All()[selfrecruit].Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "selfrecruit") {
		t.Error("FilterableProviders() should include selfrecruit")
	}
}
