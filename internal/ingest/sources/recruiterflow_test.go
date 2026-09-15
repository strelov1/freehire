package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func recruiterflowListingURL(board string) string {
	return "https://recruiterflow.com/" + board + "/jobs"
}

// recruiterflowListingHTML is a trimmed but byte-faithful fixture modeled on a live capture
// of recruiterflow.com/radhires/jobs: the whole board embedded as a bare JS variable
// assignment, grouped by department, no separate network call at all.
const recruiterflowListingHTML = `<html><body>
<script>window.jobsList = {"department": [
["Administration", [{"apply_link": "radhires/jobs/431", "details": "LATAM", "employment_type": "Full time", "job_id": 431, "job_name": "Executive Assistant", "last_opened": "2026-09-03T19:43:26+0000", "remote_type": "Remote"}]],
["Engineering", [{"apply_link": "radhires/jobs/369", "details": "LATAM - Buenos Aires, LATAM - Sao Paulo", "employment_type": "Contract", "job_id": 369, "job_name": "AI Engineer - LATAM", "last_opened": "2026-03-02T14:48:07+0000", "remote_type": null}]]
], "group": [], "location": []};</script>
</body></html>`

const recruiterflowEmptyListingHTML = `<html><body>
<script>window.jobsList = {"department": [], "group": [], "location": []};</script>
</body></html>`

func recruiterflowDetailHTML(company, title, description string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"JobPosting","title":"` + title +
		`","description":"` + description +
		`","hiringOrganization":{"@type":"Organization","name":"` + company + `"}}` +
		`</script></head><body></body></html>`
}

func TestRecruiterflowProvider(t *testing.T) {
	if got := NewRecruiterflow(nil).Provider(); got != "recruiterflow" {
		t.Errorf("Provider() = %q, want %q", got, "recruiterflow")
	}
}

func TestRecruiterflowMarkers(t *testing.T) {
	s := NewRecruiterflow(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("recruiterflow should implement the fullBoardListing marker")
	}
}

func TestRecruiterflowRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["recruiterflow"] {
		t.Error("FullBoardListingProviders(All(nil)) should include recruiterflow")
	}
}

func TestRecruiterflowRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["recruiterflow"]
	if !ok {
		t.Fatal("All() missing provider recruiterflow")
	}
	if s.Provider() != "recruiterflow" {
		t.Errorf("All()[recruiterflow].Provider() = %q", s.Provider())
	}
}

func TestRecruiterflowFetchListsAcrossDepartmentsAndHydrates(t *testing.T) {
	board := "radhires"
	fake := (&routedHTTP{}).
		route("radhires/jobs/431", recruiterflowDetailHTML("Rad Hires", "Executive Assistant", "<p>Support the team.</p>")).
		route("radhires/jobs/369", recruiterflowDetailHTML("Rad Hires", "AI Engineer - LATAM", "<p>Build AI systems.</p>")).
		route(recruiterflowListingURL(board), recruiterflowListingHTML)

	jobs, err := NewRecruiterflow(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Rad Hires"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (union across both departments)", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "431" {
		t.Errorf("ExternalID = %q, want 431", j1.ExternalID)
	}
	if j1.Title != "Executive Assistant" {
		t.Errorf("Title = %q", j1.Title)
	}
	if j1.Company != "Rad Hires" {
		t.Errorf("Company = %q, want the configured CompanyEntry.Company", j1.Company)
	}
	if j1.Location != "LATAM" {
		t.Errorf("Location = %q, want the listing's own \"details\" verbatim", j1.Location)
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (Full time)", j1.EmploymentType)
	}
	if j1.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j1.WorkMode)
	}
	if j1.Description != "<p>Support the team.</p>" {
		t.Errorf("Description = %q", j1.Description)
	}
	if j1.URL != "https://recruiterflow.com/radhires/jobs/431" {
		t.Errorf("URL = %q", j1.URL)
	}
	if j1.PostedAt == nil || j1.PostedAt.Format("2006-01-02") != "2026-09-03" {
		t.Errorf("PostedAt = %v", j1.PostedAt)
	}

	j2 := jobs[1]
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (Contract)", j2.EmploymentType)
	}
	if j2.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (remote_type was null)", j2.WorkMode)
	}
	if !strings.Contains(j2.Location, "LATAM - Buenos Aires") {
		t.Errorf("Location = %q", j2.Location)
	}
}

func TestRecruiterflowEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	board := "empty-co"
	fake := (&routedHTTP{}).route(recruiterflowListingURL(board), recruiterflowEmptyListingHTML)
	jobs, err := NewRecruiterflow(fake).Fetch(context.Background(), CompanyEntry{Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestRecruiterflowFetchFailsWholeBoardOnListingError(t *testing.T) {
	board := "boom-co"
	fake := (&routedHTTP{}).routeErr(recruiterflowListingURL(board), errors.New("boom"))
	if _, err := NewRecruiterflow(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

// recruiterflowNoJobsListHTML carries no window.jobsList assignment at all — the shape a
// template change on the platform's side would produce.
const recruiterflowNoJobsListHTML = `<html><body><p>Careers coming soon.</p></body></html>`

func TestRecruiterflowFetchFailsWhenJobsListMissing(t *testing.T) {
	board := "boom-co"
	fake := (&routedHTTP{}).route(recruiterflowListingURL(board), recruiterflowNoJobsListHTML)
	if _, err := NewRecruiterflow(fake).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
		t.Fatal("Fetch succeeded despite no window.jobsList on the page")
	}
}

func TestRecruiterflowUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	board := "radhires"
	listing := `<html><body><script>window.jobsList = {"department": [
["Engineering", [{"apply_link": "radhires/jobs/1", "details": "Remote", "employment_type": "Full time", "job_id": 1, "job_name": "Engineer", "last_opened": "2026-01-01T00:00:00+0000", "remote_type": "Remote"}]]
]};</script></body></html>`
	fake := (&routedHTTP{}).
		routeErr("radhires/jobs/1", errors.New("timeout")).
		route(recruiterflowListingURL(board), listing)

	jobs, err := NewRecruiterflow(fake).Fetch(context.Background(), CompanyEntry{Board: board, Company: "Rad Hires"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (an unreadable marker, not a drop)", len(jobs))
	}
	if jobs[0].ExternalID != "1" {
		t.Errorf("ExternalID = %q, want 1", jobs[0].ExternalID)
	}
}
