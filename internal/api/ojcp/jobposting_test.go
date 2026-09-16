package ojcp

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

const testOrigin = "https://freehire.me"

// sourceJobURL is the posting's URL as the SOURCE published it. What reaches a projection
// is never this string: jobview.FromDomain runs outboundurl.Tag over it, so the served
// value always carries utm_source. Tests that hand-built a jobview.Job skipped that and
// asserted against a value production cannot produce — which is why every fixture here now
// goes through jobview.FromRow, the same path a handler's data takes.
const sourceJobURL = "https://boards.greenhouse.io/acme/jobs/4012"

// openPostingRow is the stored row every fixture starts from. Country codes are stored
// UPPERCASE here because that is how ingest writes them; jobview lowercases the facet on
// the way out, and the projection has to cope with what jobview actually serves.
func openPostingRow() db.Job {
	return db.Job{
		PublicSlug:  "senior-go-engineer-at-acme",
		Source:      "greenhouse",
		URL:         sourceJobURL,
		Title:       "Senior Go Engineer",
		Company:     "Acme Corp",
		CompanySlug: "acme",
		Skills:      []string{"go", "postgresql"},
		PostedAt:    stamp("2026-09-16T08:30:00Z"),
		LastSeenAt:  stamp("2026-09-16T11:00:00Z"),
	}
}

func openPosting() jobview.Job { return viewOf(openPostingRow()) }

// viewOf projects a stored row the way every read path does. Building a jobview.Job by
// hand is what let two production defects pass green: the utm-tagged URL and the
// lowercased country facet.
func viewOf(row db.Job) jobview.Job {
	view, err := jobview.FromRow(row)
	if err != nil {
		panic("jobview.FromRow: " + err.Error())
	}
	return view
}

func stamp(rfc3339 string) pgtype.Timestamptz {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic("bad fixture timestamp: " + rfc3339)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestJobPostingFromProducesAConformingPosting(t *testing.T) {
	posting := jobPostingFrom(openPosting(), testOrigin)

	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("projected posting rejected by the OJCP schema: %v", err)
	}
}

func TestJobPostingFromCarriesTheFieldsWeMeanToEmit(t *testing.T) {
	// The schema does not set additionalProperties:false, so a dropped or misspelt field
	// validates clean (see testdata/schemas/README.md). Presence is asserted here or nowhere.
	posting := jobPostingFrom(openPosting(), testOrigin)

	if posting.OJCPID != "senior-go-engineer-at-acme" {
		t.Errorf("ojcp_id = %q, want the public slug", posting.OJCPID)
	}
	if posting.Title != "Senior Go Engineer" {
		t.Errorf("title = %q", posting.Title)
	}
	if posting.Employer.Name != "Acme Corp" {
		t.Errorf("employer.name = %q", posting.Employer.Name)
	}
	// Skills are NOT asserted here: they come from the posting's stated requirements, not
	// from the skills facet, and this fixture states none. See skills_test.go.
}

func TestJobPostingFromSeparatesOurPageFromTheSourcesOwnLink(t *testing.T) {
	// Conflating the two is how attribution gets lost: `url` is where an agent sends a
	// reader on our site, `official_job_url` is the employer's own posting.
	posting := projector().JobPosting(openPosting(), nil)

	wantOurs := testOrigin + "/jobs/senior-go-engineer-at-acme"
	if posting.URL != wantOurs {
		t.Errorf("url = %q, want %q", posting.URL, wantOurs)
	}
	if posting.OfficialJobURL != sourceJobURL {
		t.Errorf("official_job_url = %q, want the canonical source link", posting.OfficialJobURL)
	}
}

func TestOfficialJobURLCarriesNoTrackingParameter(t *testing.T) {
	// The schema defines this field as the canonical page on the employer's own site, and
	// tells agents to deduplicate and domain-verify against it. Our own utm_source — which
	// jobview stamps on every served URL — defeats both.
	posting := projector().JobPosting(openPosting(), nil)

	if strings.Contains(posting.OfficialJobURL, "utm_source") {
		t.Errorf("official_job_url carries our tracking parameter: %s", posting.OfficialJobURL)
	}
	// Our own page is where a tagged link belongs, and it is a freehire URL, so no tag is
	// added there either — but the distinction must not be lost.
	if posting.URL == posting.OfficialJobURL {
		t.Error("url and official_job_url collapsed into one value")
	}
}

func TestOfficialJobURLIsAbsentForAnAggregatorSource(t *testing.T) {
	// For an aggregator the stored URL is a tracking redirect on the aggregator's own
	// domain, not the employer's page. Publishing it as `official_job_url` would hand an
	// agent a domain-verification anchor that fails by construction; `url` is the field
	// whose own description permits third-party boards, and it still carries the link.
	for _, provider := range []string{"adzuna", "whatjobs"} {
		t.Run(provider, func(t *testing.T) {
			row := openPostingRow()
			row.Source = provider
			row.URL = "https://www.adzuna.com/land/ad/5012345678"

			posting := projector().JobPosting(viewOf(row), nil)

			if posting.OfficialJobURL != "" {
				t.Errorf("official_job_url = %q, want empty for an aggregator", posting.OfficialJobURL)
			}
			if posting.URL == "" {
				t.Error("url is empty; the posting is still reachable through our page")
			}
		})
	}
}

func TestOfficialJobURLIsPresentForADirectATS(t *testing.T) {
	posting := projector().JobPosting(openPosting(), nil)

	if posting.OfficialJobURL != sourceJobURL {
		t.Errorf("official_job_url = %q, want the ATS link for a direct source", posting.OfficialJobURL)
	}
}

func TestJobPostingFromEmitsDatesAsCalendarDatesNotTimestamps(t *testing.T) {
	// `datePosted` and `validThrough` are `format: date` in the schema, while every
	// timestamp we hold is RFC3339. Handing the timestamp through unchanged is the
	// obvious mistake and the schema oracle now catches it — this pins the value too.
	posting := jobPostingFrom(openPosting(), testOrigin)

	if posting.DatePosted != "2026-09-16" {
		t.Errorf("datePosted = %q, want a calendar date", posting.DatePosted)
	}
	// Open posting: last_seen_at + the same 30-day buffer the site's schema.org markup
	// uses, so the two channels never disagree about when a posting expires.
	if posting.ValidThrough != "2026-10-16" {
		t.Errorf("validThrough = %q, want last_seen_at plus the buffer", posting.ValidThrough)
	}
}

func TestJobPostingFromFallsBackToWhenWeFirstSawThePosting(t *testing.T) {
	// `datePosted` is REQUIRED by the schema, so a source that states no publication date
	// cannot simply omit it. The day the catalogue first recorded the posting is the honest
	// lower bound — never later than the day it actually appeared.
	//
	// The fallback itself is jobview's (effectivePosted), not this projection's: a stored
	// row with no posted_at is served with PostedAt already standing in for it. Asserting it
	// from the ROW is what proves the projection inherits the behaviour — an earlier version
	// of this test hand-set PostedAt=nil with CreatedAt set, a combination no read path
	// produces, and so tested a branch that could never run.
	row := openPostingRow()
	row.PostedAt = pgtype.Timestamptz{}
	row.CreatedAt = stamp("2026-09-14T22:15:00Z")

	posting := jobPostingFrom(viewOf(row), testOrigin)

	if posting.DatePosted != "2026-09-14" {
		t.Errorf("datePosted = %q, want the creation date as the fallback", posting.DatePosted)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting dated from creation rejected: %v", err)
	}
}

func TestJobPostingFromLeavesADatelessPostingInvalidRatherThanInventingADate(t *testing.T) {
	// No stored row lacks both timestamps, so this shape means something upstream is
	// wrong. Making the validator quiet by inventing a date would hide it; the projection
	// emits nothing and the schema refuses the result, which is the signal.
	bare := jobview.Job{
		PublicSlug: "unknown-role-at-acme",
		Title:      "Unknown Role",
		Company:    "Acme Corp",
	}

	posting := jobPostingFrom(bare, testOrigin)

	if posting.DatePosted != "" {
		t.Errorf("datePosted = %q, want empty rather than an invented date", posting.DatePosted)
	}
	if posting.ValidThrough != "" {
		t.Errorf("validThrough = %q, want empty with no evidence to estimate from", posting.ValidThrough)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err == nil {
		t.Fatal("a posting with no date at all validated; the required field is not enforced")
	}
}

func TestJobPostingFromTranslatesWorkModeIntoTheStandardsVocabulary(t *testing.T) {
	// remote_policy is a CLOSED enum in the schema, so our "onsite" must become "on_site"
	// or the whole posting is rejected. A work mode we do not hold omits the field —
	// OJCP has no "unknown", and guessing one would assert something the posting never said.
	for _, tc := range []struct{ ours, want string }{
		{"remote", "remote"},
		{"hybrid", "hybrid"},
		{"onsite", "on_site"},
		{"", ""},
		{"something-new", ""},
	} {
		t.Run(tc.ours, func(t *testing.T) {
			j := openPosting()
			j.WorkMode = tc.ours

			posting := jobPostingFrom(j, testOrigin)

			if posting.RemotePolicy != tc.want {
				t.Errorf("remote_policy = %q, want %q", posting.RemotePolicy, tc.want)
			}
			if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
				t.Fatalf("posting rejected: %v", err)
			}
		})
	}
}

func TestJobPostingFromTranslatesOnlyTheExactSeniorityMatches(t *testing.T) {
	// The standard names six levels; we hold eight. Where a word means exactly the same
	// thing it is translated; where the standard has no word for a level, ours is emitted
	// verbatim (the schema allows it), because collapsing principal into senior would make
	// an agent's search for one return the other.
	for _, tc := range []struct{ ours, want string }{
		{"middle", "mid"},
		{"c_level", "executive"},
		{"senior", "senior"},
		{"lead", "lead"},
		{"intern", "intern"},
		{"junior", "junior"},
		{"staff", "staff"},
		{"principal", "principal"},
		{"", ""},
	} {
		t.Run(tc.ours, func(t *testing.T) {
			j := openPosting()
			j.Enrichment.Seniority = tc.ours

			posting := jobPostingFrom(j, testOrigin)

			if posting.ExperienceLevel != tc.want {
				t.Errorf("experienceLevel = %q, want %q", posting.ExperienceLevel, tc.want)
			}
		})
	}
}

func TestJobPostingFromCarriesEmploymentTypeAndSalary(t *testing.T) {
	min, max := 120000, 160000
	j := openPosting()
	j.Enrichment.EmploymentType = "full_time"
	j.Enrichment.SalaryMin = &min
	j.Enrichment.SalaryMax = &max
	j.Enrichment.SalaryCurrency = "USD"
	j.Enrichment.SalaryPeriod = "year"

	posting := jobPostingFrom(j, testOrigin)

	if posting.EmploymentType != "full_time" {
		t.Errorf("employmentType = %q", posting.EmploymentType)
	}
	if posting.BaseSalary == nil {
		t.Fatal("baseSalary is nil; the posting states a salary")
	}
	if posting.BaseSalary.Currency != "USD" || posting.BaseSalary.UnitText != "YEAR" {
		t.Errorf("baseSalary = %+v, want USD/YEAR", posting.BaseSalary)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with salary rejected: %v", err)
	}
}

func TestJobPostingFromOmitsSalaryWhenThePostingStatesNone(t *testing.T) {
	// A zeroed baseSalary block would read as "this job pays nothing", which is a claim
	// the posting never made.
	posting := jobPostingFrom(openPosting(), testOrigin)

	if posting.BaseSalary != nil {
		t.Errorf("baseSalary = %+v, want nil for a posting with no stated pay", posting.BaseSalary)
	}
}

func TestJobPostingFromDatesAClosedPostingByWhenItClosed(t *testing.T) {
	// A closed posting's expiry is a fact, not an estimate, so the buffer must not push
	// it into the future — an agent would read the posting as still open.
	closedAt := "2026-09-15T09:00:00Z"
	j := openPosting()
	j.ClosedAt = &closedAt

	posting := jobPostingFrom(j, testOrigin)

	if posting.ValidThrough != "2026-09-15" {
		t.Errorf("validThrough = %q, want the close date unbuffered", posting.ValidThrough)
	}
}
