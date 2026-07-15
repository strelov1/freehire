package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// alignerrListingJSON is the /jobs page's __NEXT_DATA__: initialJobs carries each posting's id
// and a truncated preview, so the adapter enumerates ids here and fetches each detail.
func alignerrListingJSON(ids ...string) string {
	var items []string
	for _, id := range ids {
		items = append(items, `{"id":"`+id+`","title":"Preview Title","location":"Remote","applyUrl":"/jobs/`+id+`"}`)
	}
	return `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"initialJobs":[` + strings.Join(items, ",") + `]}}}` +
		`</script>`
}

// alignerrDetailJSON is a detail page's __NEXT_DATA__ job record. The description embeds a
// <script> that sanitizeHTML must strip; jobType is the structured employment enum and
// firstPostDate the publish date.
func alignerrDetailJSON(id, name, jobType string, active bool) string {
	activeStr := "false"
	if active {
		activeStr = "true"
	}
	return `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"job":{"id":"` + id + `","name":"` + name + `",` +
		`"htmlLongDescription":"<p>Build tasks.</p><script>alert(1)</script>",` +
		`"shortDescription":"short","isActive":` + activeStr + `,` +
		`"firstPostDate":"2026-07-07T00:00:01.353Z","createdAt":"2026-06-26T21:41:06.513Z",` +
		`"jobType":"` + jobType + `","location":"United States"}}}}` +
		`</script>`
}

func TestAlignerrProvider(t *testing.T) {
	if got := NewAlignerr(nil).Provider(); got != "alignerr" {
		t.Errorf("Provider() = %q, want %q", got, "alignerr")
	}
}

func TestAlignerrFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/aaa", alignerrDetailJSON("aaa", "Software Engineer Task Author (AI Training)", "CONTRACT", true)).
		route("alignerr.com/jobs", alignerrListingJSON("aaa"))

	jobs, err := NewAlignerr(fake).Fetch(context.Background(), CompanyEntry{Company: "Alignerr", Provider: "alignerr"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "aaa" {
		t.Errorf("ExternalID = %q, want aaa", j.ExternalID)
	}
	if j.URL != "https://www.alignerr.com/jobs/aaa" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Software Engineer Task Author (AI Training)" {
		t.Errorf("Title = %q, want detail name", j.Title)
	}
	if j.Company != "Alignerr" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Remote" || !j.Remote {
		t.Errorf("Location=%q Remote=%v, want listing Remote", j.Location, j.Remote)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j.EmploymentType)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build tasks") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 7, 0, 0, 1, 353000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want firstPostDate 2026-07-07T00:00:01.353Z", j.PostedAt)
	}
}

func TestAlignerrDropsInactiveAndMissingDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/jobs/live", alignerrDetailJSON("live", "Live Role", "CONTRACT", true)).
		route("/jobs/closed", alignerrDetailJSON("closed", "Closed Role", "CONTRACT", false)).
		// no route for "gone" → its detail fetch fails
		route("alignerr.com/jobs", alignerrListingJSON("live", "closed", "gone"))

	jobs, err := NewAlignerr(fake).Fetch(context.Background(), CompanyEntry{Company: "Alignerr"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "live" {
		t.Fatalf("got %v, want only the active posting with a reachable detail", jobs)
	}
}

func TestAlignerrEmploymentType(t *testing.T) {
	cases := map[string]string{
		"CONTRACT": "contract", "FULL_TIME": "full_time",
		"PART_TIME": "part_time", "INTERNSHIP": "internship", "": "", "WEIRD": "",
	}
	for in, want := range cases {
		if got := alignerrEmploymentType(in); got != want {
			t.Errorf("alignerrEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAlignerrRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["alignerr"]
	if !ok {
		t.Fatal("All() missing provider alignerr")
	}
	if s.Provider() != "alignerr" {
		t.Errorf("All()[alignerr].Provider() = %q", s.Provider())
	}
	// Single-company boardless: redundant with the company filter, so excluded from the facet.
	if slices.Contains(FilterableProviders(), "alignerr") {
		t.Error("FilterableProviders() should exclude boardless alignerr")
	}
}
