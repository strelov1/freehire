package sources

import (
	"context"
	"errors"
	"testing"
)

// herpJobHTML is a HERP job page carrying the schema.org JobPosting ld+json block the
// adapter's detail fetch reads — title, description, datePosted, and jobLocation.address.
func herpJobHTML(title, desc, location string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting","title":"` + title +
		`","description":"` + desc + `","datePosted":"2025-04-18T07:44:06.000Z",` +
		`"jobLocation":{"@type":"Place","address":"` + location + `"}}` +
		`</script></head><body>page</body></html>`
}

func TestIsHerpJobLinkMatchesDirectJobPath(t *testing.T) {
	if !isHerpJobLink("a244", "/v1/a244/GnoQonoXGBZi") {
		t.Error("want a direct job link recognized")
	}
}

func TestIsHerpJobLinkMatchesAbsoluteURL(t *testing.T) {
	if !isHerpJobLink("a244", "https://herp.careers/v1/a244/GnoQonoXGBZi") {
		t.Error("want an absolute job link recognized")
	}
}

func TestIsHerpJobLinkRejectsApplySuffix(t *testing.T) {
	if isHerpJobLink("a244", "/v1/a244/GnoQonoXGBZi/apply") {
		t.Error("want the /apply link NOT treated as a separate job")
	}
}

func TestIsHerpJobLinkRejectsRequisitionGroupPath(t *testing.T) {
	if isHerpJobLink("a244", "/v1/a244/requisition-groups/d4917a2b-a689-417c-a27b-2fb1bf8b2759") {
		t.Error("want a requisition-group link NOT treated as a job")
	}
}

// Some companies configure a distinct "top" landing page, whose header carries a link back
// to it (career-page-header__link) on every listing and job page of that board — a single
// path segment indistinguishable in shape from a real opaque HERP job id. Confirmed live
// (e.g. herp.careers/v1/clueitinc/top). Left unexcluded, it would be probed as a job on
// every crawl of that board and its detail page (the listing page itself, no JobPosting
// block) marked Unreadable forever, permanently withholding that board's stale-job close.
func TestIsHerpJobLinkRejectsTopLandingPageLink(t *testing.T) {
	if isHerpJobLink("clueitinc", "/v1/clueitinc/top") {
		t.Error("want the platform's own \"top\" landing-page link NOT treated as a job")
	}
}

func TestIsHerpJobLinkRejectsOtherBoard(t *testing.T) {
	if isHerpJobLink("a244", "/v1/other-board/GnoQonoXGBZi") {
		t.Error("want a link for a different board rejected")
	}
}

func TestIsHerpJobLinkRejectsShareWidgetLink(t *testing.T) {
	href := "https://twitter.com/share?url=https%3A%2F%2Fherp.careers%2Fv1%2Fa244%2FGnoQonoXGBZi"
	if isHerpJobLink("a244", href) {
		t.Error("want a share-widget link on a different host rejected, even though its query string embeds a real herp.careers URL")
	}
}

func TestIsHerpGroupLinkMatchesRequisitionGroupPath(t *testing.T) {
	if !isHerpGroupLink("a244", "/v1/a244/requisition-groups/d4917a2b-a689-417c-a27b-2fb1bf8b2759") {
		t.Error("want a requisition-group link recognized")
	}
}

func TestIsHerpGroupLinkRejectsDirectJobPath(t *testing.T) {
	if isHerpGroupLink("a244", "/v1/a244/GnoQonoXGBZi") {
		t.Error("want a direct job link NOT treated as a group")
	}
}

func TestIsHerpGroupLinkRejectsShareWidgetLink(t *testing.T) {
	href := "https://facebook.com/sharer.php?u=https%3A%2F%2Fherp.careers%2Fv1%2Fa244%2Frequisition-groups%2Fd4917a2b-a689-417c-a27b-2fb1bf8b2759"
	if isHerpGroupLink("a244", href) {
		t.Error("want a share-widget link on a different host rejected")
	}
}

func TestHerpProvider(t *testing.T) {
	if got := NewHerp(nil).Provider(); got != "herp" {
		t.Errorf("Provider() = %q, want %q", got, "herp")
	}
}

// HERP earns fullBoardListing: a failure fetching the company page or any requisition-group
// page fails the whole Fetch, never a silently truncated listing. Detail fetches are
// best-effort per posting.
func TestHerpMarkers(t *testing.T) {
	s := NewHerp(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("herp should implement the fullBoardListing marker")
	}
}

func TestHerpRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["herp"] {
		t.Error("FullBoardListingProviders(All(nil)) should include herp")
	}
}

func TestHerpFetchPropagatesAListingError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewHerp(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestHerpFetchExpandsGroupsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/v1/acme/job1", herpJobHTML("Backend Engineer", "<p>Build things.</p>", "Tokyo")).
		route("/v1/acme/job2", herpJobHTML("Frontend Engineer", "<p>Ship pixels.</p>", "Osaka")).
		route("/requisition-groups/g1", `<html><body>`+
			`<a href="/v1/acme/job2">Frontend Engineer</a>`+
			`</body></html>`).
		route("/v1/acme", `<html><body>`+
			`<a href="/v1/acme/job1">Backend Engineer</a>`+
			`<a href="/v1/acme/requisition-groups/g1">Engineering</a>`+
			`</body></html>`)

	jobs, err := NewHerp(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "herp", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2: %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	job1, ok := byID["job1"]
	if !ok {
		t.Fatal("job1 not found in results")
	}
	if job1.Title != "Backend Engineer" || job1.Location != "Tokyo" || job1.Company != "Acme" {
		t.Errorf("job1 = %+v, want title=Backend Engineer location=Tokyo company=Acme", job1)
	}
	job2, ok := byID["job2"]
	if !ok {
		t.Fatal("job2 (discovered via the requisition group) not found in results")
	}
	if job2.Title != "Frontend Engineer" || job2.Location != "Osaka" {
		t.Errorf("job2 = %+v, want title=Frontend Engineer location=Osaka", job2)
	}
}

func TestHerpFetchPropagatesAGroupError(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("requisition-groups/g1", errors.New("fakeHerp: group boom")).
		route("/v1/acme", `<html><body>`+
			`<a href="/v1/acme/requisition-groups/g1">Engineering</a>`+
			`</body></html>`)

	if _, err := NewHerp(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a failed requisition-group fetch")
	}
}

// A failed job-detail fetch does not drop the posting entirely — a generic failure (not a
// platform-stated 404/410) becomes an Unreadable marker (see Job.Unreadable), carrying the
// identity without the detail, while every other job on the board is unaffected.
func TestHerpFetchMarksOnlyFailedDetailUnreadable(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/v1/acme/job1", herpJobHTML("Backend Engineer", "<p>Build things.</p>", "Tokyo")).
		routeErr("/v1/acme/job2", errors.New("fakeHerp: detail boom")).
		route("/v1/acme", `<html><body>`+
			`<a href="/v1/acme/job1">Backend Engineer</a>`+
			`<a href="/v1/acme/job2">Frontend Engineer</a>`+
			`</body></html>`)

	jobs, err := NewHerp(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (job1 readable, job2 marked unreadable): %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if job1 := byID["job1"]; job1.Unreadable || job1.Title != "Backend Engineer" {
		t.Errorf("job1 = %+v, want a fully readable, non-unreadable job", job1)
	}
	if job2 := byID["job2"]; !job2.Unreadable {
		t.Errorf("job2 = %+v, want Unreadable=true", job2)
	}
}
