package sources

import (
	"context"
	"errors"
	"testing"
)

// hrmosJobHTML is an HRMOS job page carrying the schema.org JobPosting ld+json block the
// adapter's detail fetch reads — title, description, datePosted, jobLocation, employmentType.
func hrmosJobHTML(title, desc, locality, region, employmentType string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting","title":"` + title +
		`","description":"` + desc + `","datePosted":"2025-04-18T07:44:06.000Z",` +
		`"employmentType":"` + employmentType + `",` +
		`"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressRegion":"` + region + `"}}]}` +
		`</script></head><body>page</body></html>`
}

func TestIsHrmosJobLinkMatchesJobPath(t *testing.T) {
	if !isHrmosJobLink("cyberagent-group", "/pages/cyberagent-group/jobs/900100") {
		t.Error("want a job link recognized")
	}
}

func TestIsHrmosJobLinkMatchesAbsoluteURL(t *testing.T) {
	if !isHrmosJobLink("cyberagent-group", "https://hrmos.co/pages/cyberagent-group/jobs/900100") {
		t.Error("want an absolute job link recognized")
	}
}

func TestIsHrmosJobLinkRejectsBoardRoot(t *testing.T) {
	if isHrmosJobLink("cyberagent-group", "/pages/cyberagent-group") {
		t.Error("want the bare board root NOT treated as a job")
	}
}

func TestIsHrmosJobLinkRejectsListingPageWithNoJobID(t *testing.T) {
	if isHrmosJobLink("cyberagent-group", "/pages/cyberagent-group/jobs") {
		t.Error("want the listing page itself (no job id) NOT treated as a job")
	}
}

func TestIsHrmosJobLinkRejectsDeeperPath(t *testing.T) {
	if isHrmosJobLink("cyberagent-group", "/pages/cyberagent-group/jobs/900100/apply") {
		t.Error("want a path deeper than jobs/<id> NOT treated as a job")
	}
}

func TestIsHrmosJobLinkRejectsOtherBoard(t *testing.T) {
	if isHrmosJobLink("cyberagent-group", "/pages/other-board/jobs/900100") {
		t.Error("want a link for a different board rejected")
	}
}

func TestIsHrmosJobLinkRejectsOtherHost(t *testing.T) {
	href := "https://corp.gree.net/pages/cyberagent-group/jobs/900100"
	if isHrmosJobLink("cyberagent-group", href) {
		t.Error("want a link on a different host rejected, even with the same path shape")
	}
}

func TestHrmosProvider(t *testing.T) {
	if got := NewHrmos(nil).Provider(); got != "hrmos" {
		t.Errorf("Provider() = %q, want %q", got, "hrmos")
	}
}

// HRMOS earns fullBoardListing via crawlAllPagedLinks: a listing-page failure at any point,
// not just the first, fails the whole Fetch. Detail fetches are best-effort per posting.
func TestHrmosMarkers(t *testing.T) {
	s := NewHrmos(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("hrmos should implement the fullBoardListing marker")
	}
}

func TestHrmosRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["hrmos"] {
		t.Error("FullBoardListingProviders(All(nil)) should include hrmos")
	}
}

func TestHrmosFetchPropagatesAFirstPageListingError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewHrmos(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestHrmosFetchPagesToExhaustionAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pages/acme/jobs/job1", hrmosJobHTML("Backend Engineer", "<p>Build things.</p>", "Shibuya", "Tokyo", "FULL_TIME")).
		route("/pages/acme/jobs/job2", hrmosJobHTML("Frontend Engineer", "<p>Ship pixels.</p>", "Naka", "Osaka", "CONTRACTOR")).
		route("page=3", `<html><body>no more jobs</body></html>`).
		route("page=2", `<html><body><a href="/pages/acme/jobs/job2">Frontend Engineer</a></body></html>`).
		route("page=1", `<html><body><a href="/pages/acme/jobs/job1">Backend Engineer</a></body></html>`)

	jobs, err := NewHrmos(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "hrmos", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one per page): %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	job1, ok := byID["job1"]
	if !ok {
		t.Fatal("job1 (page 1) not found in results")
	}
	if job1.Title != "Backend Engineer" || job1.Location != "Shibuya, Tokyo" || job1.EmploymentType != "full_time" {
		t.Errorf("job1 = %+v, want title=Backend Engineer location=\"Shibuya, Tokyo\" employment_type=full_time", job1)
	}
	job2, ok := byID["job2"]
	if !ok {
		t.Fatal("job2 (page 2) not found in results")
	}
	if job2.Title != "Frontend Engineer" || job2.EmploymentType != "contract" {
		t.Errorf("job2 = %+v, want title=Frontend Engineer employment_type=contract", job2)
	}
}

func TestHrmosFetchPropagatesALaterPageListingError(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("page=2", errors.New("fakeHrmos: page 2 boom")).
		route("page=1", `<html><body><a href="/pages/acme/jobs/job1">Backend Engineer</a></body></html>`)

	if _, err := NewHrmos(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a page-2 listing error")
	}
}

func TestHrmosFetchMarksOnlyFailedDetailUnreadable(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pages/acme/jobs/job1", hrmosJobHTML("Backend Engineer", "<p>Build things.</p>", "Shibuya", "Tokyo", "FULL_TIME")).
		routeErr("/pages/acme/jobs/job2", errors.New("fakeHrmos: detail boom")).
		route("page=2", `<html><body>no more jobs</body></html>`).
		route("page=1", `<html><body>`+
			`<a href="/pages/acme/jobs/job1">Backend Engineer</a>`+
			`<a href="/pages/acme/jobs/job2">Frontend Engineer</a>`+
			`</body></html>`)

	jobs, err := NewHrmos(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
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
	if job2 := byID["job2"]; !job2.Unreadable {
		t.Errorf("job2 = %+v, want Unreadable=true", job2)
	}
}

func TestHrmosMapsUnrecognizedEmploymentTypeToEmpty(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/pages/acme/jobs/job1", hrmosJobHTML("Volunteer Role", "<p>Help out.</p>", "Shibuya", "Tokyo", "VOLUNTEER")).
		route("page=2", `<html><body>no more jobs</body></html>`).
		route("page=1", `<html><body><a href="/pages/acme/jobs/job1">Volunteer Role</a></body></html>`)

	jobs, err := NewHrmos(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].EmploymentType != "" {
		t.Errorf("jobs = %+v, want one job with empty employment_type (VOLUNTEER has no freehire equivalent)", jobs)
	}
}
