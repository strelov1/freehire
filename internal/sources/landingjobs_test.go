package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLandingJobsProvider(t *testing.T) {
	if got := NewLandingJobs(nil).Provider(); got != "landingjobs" {
		t.Errorf("Provider() = %q, want landingjobs", got)
	}
}

func TestLandingJobsIsBoardlessAggregator(t *testing.T) {
	s := NewLandingJobs(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("landingjobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("landingjobs should implement the aggregator marker")
	}
}

func TestLandingJobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["landingjobs"]; !ok {
		t.Error("All() should register provider landingjobs")
	}
	if !slices.Contains(FilterableProviders(), "landingjobs") {
		t.Error("FilterableProviders() should include landingjobs")
	}
}

func TestLandingJobsBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/landingjobs.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/landingjobs.yml fails validation: %v", err)
	}
}

// The feed is a top-level array with no company and no id, so the mapping rests entirely on
// the URL. This covers the whole happy path in one posting.
func TestLandingJobsFetchMapsAPosting(t *testing.T) {
	feed := `[
{"title":"Senior Go Engineer",
 "url":"https://landing.jobs/at/acme-corp/senior-go-engineer",
 "locations":[{"city":"Lisbon","country_code":"PT"}],
 "remote":false,
 "published_at":"2026-08-01T09:30:00.000Z",
 "created_at":"2026-07-20T09:30:00.000Z",
 "role_description":"<p>Build things.</p>",
 "main_requirements":"<ul><li>Go</li></ul>",
 "nice_to_have":"<p>Kubernetes.</p>",
 "perks":"<p>Lunch.</p>",
 "tags":["go","docker"],
 "type":"full-time",
 "gross_salary_low":50000,
 "gross_salary_high":70000,
 "currency_code":"EUR"}
]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Acme Corp" {
		t.Errorf("Company = %q, want Acme Corp — humanized from the URL slug", j.Company)
	}
	// Both slugs, because a job slug alone is not unique across employers.
	if j.ExternalID != "acme-corp/senior-go-engineer" {
		t.Errorf("ExternalID = %q, want acme-corp/senior-go-engineer", j.ExternalID)
	}
	if j.Location != "Lisbon, PT" {
		t.Errorf("Location = %q, want %q", j.Location, "Lisbon, PT")
	}
	if !slices.Equal(j.Countries, []string{"pt"}) {
		t.Errorf("Countries = %v, want [pt] from the structured country_code", j.Countries)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want published_at 2026-08-01T09:30:00Z", j.PostedAt)
	}
	for _, want := range []string{"Build things.", "Requirements", "Nice to have", "Perks"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %s", want, j.Description)
		}
	}
	// tags/type/salary are deliberately undeclared; the decode must ignore them rather than fail.
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
}

// locations is null for a fully-remote role, which must not panic and must still produce a
// usable location string plus the structured work mode.
func TestLandingJobsRemoteWithNoLocations(t *testing.T) {
	feed := `[{"title":"Remote Engineer","url":"https://landing.jobs/at/globex/remote-engineer",
"locations":null,"remote":true,"published_at":"2026-08-02T00:00:00.000Z",
"role_description":"<p>Anywhere.</p>"}]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Location != "Remote" {
		t.Errorf("Location = %q, want Remote", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/remote from the structured flag", j.Remote, j.WorkMode)
	}
	if j.Countries != nil {
		t.Errorf("Countries = %v, want nil with no locations", j.Countries)
	}
}

// remote=false is "not flagged remote", not "onsite": the board carries hybrid roles, so the
// mode must be left for the pipeline rather than asserted.
func TestLandingJobsDoesNotAssertOnsite(t *testing.T) {
	feed := `[{"title":"Engineer","url":"https://landing.jobs/at/acme/engineer",
"locations":[{"city":"Porto","country_code":"PT"}],"remote":false,
"published_at":"2026-08-02T00:00:00.000Z"}]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, _ := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty — remote=false states nothing about onsite vs hybrid", jobs[0].WorkMode)
	}
}

// The id is the dedup key and the company rides the same URL, so a posting whose URL has no
// /at/<company>/<job> pair is dropped rather than ingested under a fabricated identity.
func TestLandingJobsSkipsPostingsWithoutAResolvableURL(t *testing.T) {
	feed := `[
{"title":"No at-path","url":"https://landing.jobs/jobs/12345","published_at":"2026-08-02T00:00:00.000Z"},
{"title":"Company only","url":"https://landing.jobs/at/acme-corp","published_at":"2026-08-02T00:00:00.000Z"},
{"title":"","url":"https://landing.jobs/at/acme-corp/untitled","published_at":"2026-08-02T00:00:00.000Z"},
{"title":"Good","url":"https://landing.jobs/at/acme-corp/good-role","published_at":"2026-08-02T00:00:00.000Z"}
]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Good" {
		t.Fatalf("jobs = %+v, want only the posting with a resolvable URL", jobs)
	}
}

// One request per crawl. `?page=N` is ignored by the endpoint (identical bodies), so a page
// walk would re-fetch the same postings; this pins the adapter against someone "fixing"
// pagination back in without evidence that a depth parameter exists.
func TestLandingJobsMakesExactlyOneRequest(t *testing.T) {
	feed := `[{"title":"A","url":"https://landing.jobs/at/acme/a","published_at":"2026-08-02T00:00:00.000Z"}]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	if _, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("made %d requests, want exactly 1 — the feed is not paginated", fake.calls)
	}
}

func TestLandingJobsListFailureIsABoardError(t *testing.T) {
	fake := &routedHTTP{}
	if _, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a failed list should be a board-level error, not an empty success")
	}
}

func TestLandingJobsCompanyHumanizesSlugs(t *testing.T) {
	cases := map[string]string{
		"acme-corp":     "Acme Corp",
		"acme":          "Acme",
		"some_company":  "Some Company",
		"multi-word-co": "Multi Word Co",
		// Internal capitals the slug preserves are not flattened.
		"gitHub": "GitHub",
	}
	for slug, want := range cases {
		if got := landingjobsCompany(slug); got != want {
			t.Errorf("landingjobsCompany(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestLandingJobsIdentityIgnoresQueryAndFragment(t *testing.T) {
	_, id, ok := landingjobsIdentity("https://landing.jobs/at/acme-corp/go-dev?utm_source=x#apply")
	if !ok || id != "acme-corp/go-dev" {
		t.Errorf("id = %q (ok=%v), want acme-corp/go-dev — the query must not enter the dedup key", id, ok)
	}
}
