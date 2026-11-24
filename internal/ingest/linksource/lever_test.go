package linksource

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

// leverPostingJSON mirrors the public per-posting API (mode=json): a single posting whose
// body is split across description/lists/additional, with createdAt in epoch millis.
const leverPostingJSON = `{
 "id": "52c01c91-582c-42fc-8722-82c3eeb9ed24",
 "text": "Senior Backend Engineer",
 "hostedUrl": "https://jobs.lever.co/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24",
 "createdAt": 1700000000000,
 "description": "<p>Build it.</p>",
 "additional": "<p>Perks.</p><script>evil()</script>",
 "lists": [{"text": "Requirements", "content": "<ul><li>Go</li></ul>"}],
 "categories": {"location": "Remote", "department": "Engineering"}
}`

func TestLeverResolvesAlignedIdentity(t *testing.T) {
	const link = "https://jobs.lever.co/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24/apply?source=web3.career"
	c := (&fakeClient{}).route("api.lever.co/v0/postings/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24", leverPostingJSON, "")

	job, ok, err := NewLever(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	// The ingest pipeline namespaces every posting's external_id by board (the URL's company
	// slug is Lever's board), so the link-source must produce the same "<board>:<id>" key to
	// dedup against an ingested board rather than write a thin telegram-source duplicate.
	if job.ExternalID != "offchainlabs:52c01c91-582c-42fc-8722-82c3eeb9ed24" {
		t.Errorf("ExternalID = %q, want the board-namespaced posting id", job.ExternalID)
	}
	if job.URL != "https://jobs.lever.co/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24" {
		t.Errorf("URL = %q", job.URL)
	}
	if job.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Team != "Engineering" { // no categories.team: falls back to the department
		t.Errorf("Team = %q", job.Team)
	}
	// The per-posting API carries no company name, so the board slug is humanized.
	if job.Company != "Offchainlabs" {
		t.Errorf("Company = %q, want Offchainlabs", job.Company)
	}
	if !job.Remote {
		t.Error("Remote = false, want true (Remote)")
	}
	if strings.Contains(job.Description, "<script>") || strings.Contains(job.Description, "evil()") {
		t.Errorf("Description not sanitized: %q", job.Description)
	}
	for _, want := range []string{"Build it.", "Requirements", "Go", "Perks."} {
		if !strings.Contains(job.Description, want) {
			t.Errorf("Description missing %q: %q", want, job.Description)
		}
	}
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2023-11-14T22:13:20Z", job.PostedAt)
	}
}

func TestLeverMatch(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://jobs.lever.co/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24", true},
		{"https://jobs.lever.co/offchainlabs/52c01c91-582c-42fc-8722-82c3eeb9ed24/apply", true},
		{"https://jobs.lever.co/offchainlabs", false},        // board listing, not one posting
		{"https://job-boards.greenhouse.io/x/jobs/1", false}, // other host
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.raw)
		if got := NewLever(nil).Match(u); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestLeverSourceKeyMatchesIngestProvider(t *testing.T) {
	if got := NewLever(nil).Source(); got != "lever" {
		t.Errorf("Source() = %q, want lever", got)
	}
}

// The link path shares the board adapter's mapper (ats.MapLeverPosting), so a posting the
// per-posting API serves with a workplace type, a commitment, a country and a salary range
// yields the same facets the board crawl would store under the same dedup key.
func TestLeverResolvesTheFacetsTheBoardAdapterYields(t *testing.T) {
	const link = "https://jobs.lever.co/acme/52c01c91-582c-42fc-8722-82c3eeb9ed24"
	const posting = `{
 "id": "52c01c91-582c-42fc-8722-82c3eeb9ed24",
 "text": "Platform Engineer",
 "hostedUrl": "https://jobs.lever.co/acme/52c01c91-582c-42fc-8722-82c3eeb9ed24",
 "createdAt": 1700000000000,
 "description": "<p>Build it.</p>",
 "workplaceType": "hybrid",
 "country": "gb",
 "categories": {"location": "London", "commitment": "Full-Time"},
 "salaryRange": {"min": 90000, "max": 120000, "currency": "GBP", "interval": "per-year-salary"}
}`
	c := (&fakeClient{}).route("api.lever.co/v0/postings/acme/52c01c91-582c-42fc-8722-82c3eeb9ed24", posting, "")

	job, ok, err := NewLever(c).Resolve(context.Background(), link)
	if err != nil || !ok {
		t.Fatalf("Resolve: ok=%v err=%v", ok, err)
	}
	if job.WorkMode != "hybrid" || job.EmploymentType != "full_time" {
		t.Errorf("WorkMode/EmploymentType = %q/%q, want hybrid/full_time", job.WorkMode, job.EmploymentType)
	}
	if len(job.Countries) != 1 || job.Countries[0] != "gb" {
		t.Errorf("Countries = %v, want [gb]", job.Countries)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 90000 || job.SalaryMax == nil || *job.SalaryMax != 120000 ||
		job.SalaryCurrency != "GBP" || job.SalaryPeriod != "year" {
		t.Errorf("salary = %v–%v %q/%q, want 90000–120000 GBP/year", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
}
