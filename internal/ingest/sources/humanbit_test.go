package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func humanbitListingURL() string { return "https://jobs.humanbit.ai/scrabble-jigsaw" }
func humanbitDetailURL(id string) string {
	return "https://jobs.humanbit.ai/scrabble-jigsaw/jobs/" + id
}

const humanbitJob1ID = "job-0000-0000-0000-000000000001"
const humanbitJob2ID = "job-0000-0000-0000-000000000002"

// humanbitListingHTML is a trimmed but byte-faithful RSC-flight fixture modeled directly on
// a live-captured HumanBit listing page (jobs.humanbit.ai/scrabble-jigsaw): two postings,
// each carrying its own company name (org_name) and a description reference that resolves
// only against THIS page's own text rows — confirmed live NOT to be the same numbering the
// per-posting detail page uses.
const humanbitListingHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"jobBoard\":\"scrabble-jigsaw\",\"jobs\":[{\"id\":\"job-0000-0000-0000-000000000001\",\"title\":\"Senior Cost Accountant\",\"status\":\"published\",\"location\":\"Noida\",\"created_at\":\"2026-08-15T09:01:37.375291+00:00\",\"salary_max\":3000000,\"salary_min\":2000000,\"salary_currency\":\"INR\",\"description\":\"$28\",\"org_name\":\"Scrabble & Jigsaw\"},{\"id\":\"job-0000-0000-0000-000000000002\",\"title\":\"Regional Head - Sales\",\"status\":\"published\",\"location\":\"Hyderabad\",\"created_at\":\"2026-07-17T07:32:27.849002+00:00\",\"salary_max\":4500000,\"salary_min\":3000000,\"salary_currency\":\"INR\",\"description\":\"$29\",\"org_name\":\"Scrabble & Jigsaw\"}]}\n28:T19,<p>Own the cost base.</p>\n29:T1a,<p>Lead the sales org.</p>\n"])</script>
</body></html>`

const humanbitEmptyListingHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"jobBoard\":\"scrabble-jigsaw\",\"jobs\":[]}\n"])</script>
</body></html>`

// humanbitDetail1HTML is job 1's own detail page: a "work-from-office" posting, its own
// "$19" description reference resolving only against this page's own text row (a
// DIFFERENT numbering than the listing's "$28"/"$29" for the same field name).
const humanbitDetail1HTML = `<html><body>
<script>self.__next_f.push([1,"18:{\"job\":{\"id\":\"job-0000-0000-0000-000000000001\",\"title\":\"Senior Cost Accountant\",\"description\":\"$19\",\"location\":\"Noida\",\"employment_type\":[\"full-time\"],\"remote\":false,\"skills\":[\"SQL\",\"Cost Accounting\"],\"work_mode\":null,\"seniority_level\":100,\"created_at\":\"2026-08-15T09:01:37.375291+00:00\"}}\n19:T23,<p>Own the cost base in detail.</p>\n"])</script>
</body></html>`

// humanbitDetail2HTML is job 2's detail page: remote=true, employment_type=["contract"],
// to exercise the Remote/WorkMode and employment-type mapping.
const humanbitDetail2HTML = `<html><body>
<script>self.__next_f.push([1,"18:{\"job\":{\"id\":\"job-0000-0000-0000-000000000002\",\"title\":\"Regional Head - Sales\",\"description\":\"$19\",\"location\":\"Hyderabad\",\"employment_type\":[\"contract\"],\"remote\":true,\"skills\":[],\"work_mode\":null,\"seniority_level\":200,\"created_at\":\"2026-07-17T07:32:27.849002+00:00\"}}\n19:T24,<p>Lead the sales org in detail.</p>\n"])</script>
</body></html>`

// humanbitSoftNotFoundHTML is the shape a stale/removed posting answers with: a real,
// decodable RSC flight (HTTP 200) whose tree renders a client "not found" component rather
// than a job object — confirmed live against the original board_submissions capture, which
// had gone stale by the time it was investigated.
const humanbitSoftNotFoundHTML = `<html><body>
<script>self.__next_f.push([1,"a:{\"metadata\":[[\"$\",\"title\",\"0\",{\"children\":\"HumanBit\"}]]}\n"])</script>
</body></html>`

func TestHumanBitProvider(t *testing.T) {
	if got := NewHumanBit(nil).Provider(); got != "humanbit" {
		t.Errorf("Provider() = %q, want %q", got, "humanbit")
	}
}

func TestHumanBitFetchListsAndHydrates(t *testing.T) {
	fake := (&routedHTTP{}).
		route(humanbitDetailURL(humanbitJob1ID), humanbitDetail1HTML).
		route(humanbitDetailURL(humanbitJob2ID), humanbitDetail2HTML).
		route(humanbitListingURL(), humanbitListingHTML)

	jobs, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Provider: "humanbit", Board: "scrabble-jigsaw",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j1, ok := byID[humanbitJob1ID]
	if !ok {
		t.Fatalf("missing job 1 in %v", jobs)
	}
	if j1.Title != "Senior Cost Accountant" {
		t.Errorf("Title = %q", j1.Title)
	}
	// The company comes from the LISTING's org_name, which the detail page never carries.
	if j1.Company != "Scrabble & Jigsaw" {
		t.Errorf("Company = %q, want the listing's org_name", j1.Company)
	}
	if j1.Location != "Noida" {
		t.Errorf("Location = %q", j1.Location)
	}
	// The description must come from the DETAIL page's own row ("in detail"), never the
	// listing's row for the same posting — the two pages use independent numbering.
	if j1.Description != "<p>Own the cost base in detail.</p>" {
		t.Errorf("Description = %q, want the detail page's own resolved row", j1.Description)
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j1.EmploymentType)
	}
	if j1.Remote {
		t.Errorf("Remote = true, want false (remote:false in source)")
	}
	if j1.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (remote:false defers to the heuristic)", j1.WorkMode)
	}
	// "Cost Accounting" is not in the skill dictionary (an IT-focused vocabulary) and is
	// correctly dropped rather than passed through raw; "SQL" resolves to its canonical
	// name, proving the dictionary pass actually runs rather than being bypassed.
	if strings.Join(j1.Skills, ",") != "sql" {
		t.Errorf("Skills = %v, want only the dictionary-recognized term", j1.Skills)
	}
	if j1.SalaryMin != nil || j1.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil (no confirmed period signal)", j1.SalaryMin, j1.SalaryMax)
	}

	j2, ok := byID[humanbitJob2ID]
	if !ok {
		t.Fatalf("missing job 2 in %v", jobs)
	}
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract", j2.EmploymentType)
	}
	if !j2.Remote {
		t.Errorf("Remote = false, want true (remote:true in source)")
	}
	if j2.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j2.WorkMode)
	}
}

// A listing whose entries all carry an empty id (a markup change on the listing's own
// shape) must yield no jobs and no detail requests, not pass an empty id through to
// fetchDetails.
const humanbitListingAllEmptyIDsHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"jobBoard\":\"scrabble-jigsaw\",\"jobs\":[{\"id\":\"\",\"title\":\"No ID\",\"org_name\":\"Scrabble & Jigsaw\"}]}\n"])</script>
</body></html>`

func TestHumanBitListingWithNoUsableIDsYieldsNoJobs(t *testing.T) {
	fake := (&routedHTTP{}).route(humanbitListingURL(), humanbitListingAllEmptyIDsHTML)
	jobs, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{Board: "scrabble-jigsaw"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (every listing entry had an empty id)", len(jobs))
	}
}

// humanbitEmploymentType must pick the first RECOGNIZED element, skipping an unrecognized
// one ahead of it rather than stopping there.
func TestHumanBitEmploymentTypeSkipsUnrecognizedElements(t *testing.T) {
	got := humanbitEmploymentType([]string{"freelance-adjacent", "contract", "full-time"})
	if got != "contract" {
		t.Errorf("humanbitEmploymentType = %q, want contract (first recognized element, skipping the unrecognized one ahead of it)", got)
	}
}

func TestHumanBitEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route(humanbitListingURL(), humanbitEmptyListingHTML)
	jobs, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{Board: "scrabble-jigsaw"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestHumanBitFetchFailsWholeBoardOnListingError(t *testing.T) {
	fake := (&routedHTTP{}).routeErr(humanbitListingURL(), errors.New("boom"))
	if _, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{Board: "scrabble-jigsaw"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing fetch error")
	}
}

// A transport failure fetching one posting's detail must mark only that posting
// Unreadable, never drop it or fail the whole board — the detail page is this adapter's
// only source for the structured fields, so a silently dropped one is indistinguishable
// from a posting taken down.
func TestHumanBitUnreadableDetailIsMarkedNotDropped(t *testing.T) {
	// job 2's detail URL is deliberately routed to an explicit error rather than left
	// unrouted: the listing URL is a PREFIX of every detail URL on this board, and
	// routedHTTP matches by substring, so an unrouted detail request would otherwise fall
	// through to the listing's own route instead of failing as intended.
	fake := (&routedHTTP{}).
		route(humanbitDetailURL(humanbitJob1ID), humanbitDetail1HTML).
		routeErr(humanbitDetailURL(humanbitJob2ID), errors.New("boom")).
		route(humanbitListingURL(), humanbitListingHTML)

	jobs, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Board: "scrabble-jigsaw",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	read := readPostings(jobs)
	if len(read) != 1 || read[0].ExternalID != humanbitJob1ID {
		t.Fatalf("read = %v, want only job 1", read)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != humanbitJob2ID {
		t.Fatalf("unreadable markers = %v, want one for job 2", markers)
	}
	if markers[0].Company != "Scrabble & Jigsaw" {
		t.Errorf("marker Company = %q, want the listing's org_name", markers[0].Company)
	}
}

// A stale posting answers with a real, decodable flight that simply carries no job object
// (the platform's client-rendered "not found" state, HTTP 200) — this must be dropped like
// an ordinary gone posting, never marked Unreadable, since the page WAS read successfully
// and it told us the posting isn't there.
func TestHumanBitSoftNotFoundDropsThePosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route(humanbitDetailURL(humanbitJob1ID), humanbitSoftNotFoundHTML).
		route(humanbitDetailURL(humanbitJob2ID), humanbitDetail2HTML).
		route(humanbitListingURL(), humanbitListingHTML)

	jobs, err := NewHumanBit(fake).Fetch(context.Background(), CompanyEntry{Board: "scrabble-jigsaw"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != humanbitJob2ID {
		t.Fatalf("got %v, want only job 2 — job 1's soft-404 dropped, not marked unreadable", jobs)
	}
}

func TestHumanBitRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["humanbit"] {
		t.Error("FullBoardListingProviders(All(nil)) should include humanbit")
	}
}

func TestHumanBitRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["humanbit"]
	if !ok {
		t.Fatal("All() missing provider humanbit")
	}
	if s.Provider() != "humanbit" {
		t.Errorf("All()[humanbit].Provider() = %q", s.Provider())
	}
}
