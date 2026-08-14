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

// The feed is a top-level array carrying a native id but no company, so identity is split: the
// id is the dedup key and the URL supplies the employer. This covers the whole happy path.
func TestLandingJobsFetchMapsAPosting(t *testing.T) {
	feed := `[
{"id":48231,
 "title":"Senior Go Engineer",
 "url":"https://landing.jobs/at/acme-corp/senior-go-engineer-in-lisbon-2025",
 "locations":[{"city":"Lisbon","country_code":"PT"}],
 "remote":false,
 "published_at":"2026-08-01T09:30:00.000Z",
 "created_at":"2026-07-20T09:30:00.000Z",
 "role_description":"<p>Build things.</p>",
 "main_requirements":"<ul><li>Go</li></ul>",
 "nice_to_have":"<p>Kubernetes.</p>",
 "perks":"<p>Lunch.</p>",
 "tags":["go","docker"],
 "type":"Full-time",
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
	// The native numeric id, not the slug pair: live slugs bake in a year, so a regenerated
	// slug would duplicate the posting rather than update it.
	if j.ExternalID != "48231" {
		t.Errorf("ExternalID = %q, want the posting's own numeric id 48231", j.ExternalID)
	}
	if j.Location != "Lisbon, PT" {
		t.Errorf("Location = %q, want %q", j.Location, "Lisbon, PT")
	}
	if !slices.Equal(j.Countries, []string{"pt"}) {
		t.Errorf("Countries = %v, want [pt] from the structured country_code", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time from the structured %q", j.EmploymentType, "Full-time")
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want published_at 2026-08-01T09:30:00Z", j.PostedAt)
	}
	for _, want := range []string{"Build things.", "Requirements", "Nice to have", "Perks"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %s", want, j.Description)
		}
	}
	// tags/salary are deliberately undeclared; the decode must ignore them rather than fail.
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
}

// 28% of a live sample carry more than one location. Keeping only the first would hide the
// posting from a search for any of the other cities or countries it is genuinely open in.
func TestLandingJobsKeepsEveryLocation(t *testing.T) {
	feed := `[{"id":9001,"title":"Platform Engineer",
"url":"https://landing.jobs/at/acme-corp/platform-engineer",
"locations":[{"city":"Munich","country_code":"DE"},
             {"city":"Lisbon","country_code":"PT"},
             {"city":"Cologne","country_code":"DE"}],
"remote":false,"published_at":"2026-08-02T00:00:00.000Z"}]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Location != "Munich, DE; Lisbon, PT; Cologne, DE" {
		t.Errorf("Location = %q, want every stated place", j.Location)
	}
	// Deduped and in first-seen order: Germany is named twice but is one country.
	if !slices.Equal(j.Countries, []string{"de", "pt"}) {
		t.Errorf("Countries = %v, want [de pt] — every country, deduped", j.Countries)
	}
}

// locations is null for a fully-remote role, which must not panic and must still produce a
// usable location string plus the structured work mode.
func TestLandingJobsRemoteWithNoLocations(t *testing.T) {
	feed := `[{"id":9002,"title":"Remote Engineer","url":"https://landing.jobs/at/globex/remote-engineer",
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

// A remote posting that also names places keeps both: the cities stay searchable and the
// remote flag is not lost to them.
func TestLandingJobsRemoteWithLocationsKeepsBoth(t *testing.T) {
	feed := `[{"id":9005,"title":"Hybrid Engineer","url":"https://landing.jobs/at/acme/hybrid",
"locations":[{"city":"Porto","country_code":"PT"}],"remote":true,
"published_at":"2026-08-02T00:00:00.000Z"}]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, _ := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Location != "Porto, PT; Remote" {
		t.Errorf("Location = %q, want the place and the remote marker", jobs[0].Location)
	}
}

// remote=false is "not flagged remote", not "onsite": the board carries hybrid roles, so the
// mode must be left for the pipeline rather than asserted.
func TestLandingJobsDoesNotAssertOnsite(t *testing.T) {
	feed := `[{"id":9003,"title":"Engineer","url":"https://landing.jobs/at/acme/engineer",
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

// The employer backs the company slug and the id is the dedup key, so a posting missing either
// is dropped rather than ingested under a fabricated identity.
func TestLandingJobsSkipsPostingsMissingAnIdentity(t *testing.T) {
	feed := `[
{"id":1,"title":"No at-path","url":"https://landing.jobs/jobs/12345","published_at":"2026-08-02T00:00:00.000Z"},
{"id":2,"title":"Company only","url":"https://landing.jobs/at/acme-corp","published_at":"2026-08-02T00:00:00.000Z"},
{"id":3,"title":"","url":"https://landing.jobs/at/acme-corp/untitled","published_at":"2026-08-02T00:00:00.000Z"},
{"id":0,"title":"No id","url":"https://landing.jobs/at/acme-corp/no-id","published_at":"2026-08-02T00:00:00.000Z"},
{"id":4,"title":"Separators only","url":"https://landing.jobs/at/---/a-role","published_at":"2026-08-02T00:00:00.000Z"},
{"id":5,"title":"Good","url":"https://landing.jobs/at/acme-corp/good-role","published_at":"2026-08-02T00:00:00.000Z"}
]`
	fake := (&routedHTTP{}).route("/api/v1/jobs", feed)
	jobs, err := NewLandingJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Good" {
		t.Fatalf("jobs = %+v, want only the posting carrying both an id and an employer", jobs)
	}
}

// One request per crawl. `?page=N` is ignored by the endpoint (identical bodies, verified
// live), so a page walk would re-fetch the same postings; this pins the adapter against someone
// "fixing" pagination back in without evidence that a depth parameter exists.
func TestLandingJobsMakesExactlyOneRequest(t *testing.T) {
	feed := `[{"id":9004,"title":"A","url":"https://landing.jobs/at/acme/a","published_at":"2026-08-02T00:00:00.000Z"}]`
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

func TestLandingJobsCompanyFromURLIgnoresQueryAndFragment(t *testing.T) {
	company, ok := landingjobsCompanyFromURL("https://landing.jobs/at/acme-corp/go-dev?utm_source=x#apply")
	if !ok || company != "Acme Corp" {
		t.Errorf("company = %q (ok=%v), want Acme Corp", company, ok)
	}
}

// A slug of nothing but separators clears every path check and humanizes to "". Reporting ok
// for that would hand the caller an empty employer, which is the one thing it must not do.
func TestLandingJobsCompanyFromURLRejectsAnEmptyName(t *testing.T) {
	for _, url := range []string{
		"https://landing.jobs/at/---/a-role",
		"https://landing.jobs/at/___/a-role",
	} {
		if company, ok := landingjobsCompanyFromURL(url); ok || company != "" {
			t.Errorf("landingjobsCompanyFromURL(%q) = %q, %v; want \"\", false", url, company, ok)
		}
	}
}

// countriesFromCodes is shared, so its contract is pinned here rather than only through the
// adapter that first needed it.
func TestCountriesFromCodesDedupesAndDropsUnresolved(t *testing.T) {
	got := countriesFromCodes([]string{"DE", "PT", "de", "", "ZZZZ"})
	if !slices.Equal(got, []string{"de", "pt"}) {
		t.Errorf("countriesFromCodes = %v, want [de pt]", got)
	}
	if countriesFromCodes(nil) != nil {
		t.Error("countriesFromCodes(nil) should be nil, so it wires straight into Job.Countries")
	}
}
