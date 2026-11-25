package mcpapp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

const testOrigin = "https://freehire.me"

// aJob builds a fixture the way a read path does, through jobview.FromRow.
//
// NEVER as a jobview.Job literal. The literal skips the facet
// normalisation, so it carries values no production read can produce — which is exactly how
// a utm-tagged employer URL passed green on the OJCP surface until a review caught it
// (internal/api/ojcp/AGENTS.md records the two defects that got through that way).
func aJob(t *testing.T, mutate ...func(*db.Job)) jobview.Job {
	t.Helper()

	row := db.Job{
		PublicSlug:  "senior-go-engineer-acme-abc123",
		Source:      "greenhouse",
		URL:         "https://boards.greenhouse.io/acme/jobs/1",
		Title:       "Senior Go Engineer",
		Company:     "Acme",
		CompanySlug: "acme",
		Location:    "Berlin, Germany",
		Description: "<p>We need <b>Go</b>.</p>",
		Countries:   []string{"DE"},
		WorkMode:    "hybrid",
		Skills:      []string{"go", "kubernetes"},
		// Seniority, category and employment type are the DICTIONARY columns, not the
		// enrichment JSON. jobview folds the columns over the model's values and the column
		// always wins — the dict-only rule this repository holds everywhere — so a fixture
		// that sets them in the JSON sets nothing at all.
		Seniority:      "senior",
		Category:       "backend",
		EmploymentType: "full_time",
		Enrichment:     enrichmentJSON(t, enrich.Enrichment{}),
	}
	for _, m := range mutate {
		m(&row)
	}

	view, err := jobview.FromRow(row)
	if err != nil {
		t.Fatalf("jobview.FromRow: %v", err)
	}
	return view
}

func enrichmentJSON(t *testing.T, e enrich.Enrichment) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshalling enrichment: %v", err)
	}
	return raw
}

func TestEverySearchResultNamesItsSourceAndItsLinks(t *testing.T) {
	// This channel renders results as prose, where a link is the easiest thing to drop. A
	// posting that arrives without its source can be repeated as though freehire were the
	// employer, and one without a link out sends nobody anywhere. The employer's own URL is the
	// Data plan's, so the way out is the click redirect.
	got := NewProjector(testOrigin).JobSummary(aJob(t))

	if got.Source != "greenhouse" {
		t.Errorf("source = %q, want the board it came from", got.Source)
	}
	if got.URL != "https://freehire.me/jobs/senior-go-engineer-acme-abc123" {
		t.Errorf("url = %q, want the posting's page on freehire", got.URL)
	}
	if want := "https://freehire.me/api/v1/go/senior-go-engineer-acme-abc123?src=mcp"; got.ApplyURL != want {
		t.Errorf("apply_url = %q, want %q", got.ApplyURL, want)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "boards.greenhouse.io") {
		t.Errorf("summary carries the employer's own URL: %s", raw)
	}
}

func TestAnUnconfiguredOriginResearchLinkIsTheRedirect(t *testing.T) {
	got := NewProjector("").JobSummary(aJob(t))

	if want := "https://freehire.me/api/v1/go/senior-go-engineer-acme-abc123?src=mcp"; researchURL(got) != want {
		t.Errorf("research url = %q, want %q", researchURL(got), want)
	}
}

func TestAnUnconfiguredOriginPublishesNoFreehireURLRatherThanARelativeOne(t *testing.T) {
	// A relative "/jobs/<slug>" is a link the model will render and nobody can follow. The
	// posting is still reachable through apply_url, which is the better failure.
	got := NewProjector("").JobSummary(aJob(t))

	if got.URL != "" {
		t.Errorf("url = %q, want empty when no origin is configured", got.URL)
	}
	if got.ApplyURL == "" {
		t.Error("apply_url was dropped too; it does not depend on our origin")
	}
}

func TestSeniorityIsPublishedAsOursRatherThanFlattened(t *testing.T) {
	// The reason this server exists at all. OJCP's experience_level cannot name `staff`,
	// and 18.1% of open postings carry a level outside its vocabulary — so a surface that
	// translates into it must drop or distort them. This one does not translate.
	j := aJob(t, func(row *db.Job) { row.Seniority = "staff" })

	if got := NewProjector(testOrigin).JobSummary(j); got.Seniority != "staff" {
		t.Errorf("seniority = %q, want it passed through unchanged", got.Seniority)
	}
}

func TestTheSummaryIsPlainTextNotMarkup(t *testing.T) {
	// The stored description is markup. Handing tags to a model makes it pay tokens to read
	// past them, ten times over in a search result.
	got := NewProjector(testOrigin).JobSummary(aJob(t))

	if strings.Contains(got.Summary, "<") {
		t.Errorf("summary = %q, want markup stripped", got.Summary)
	}
	if !strings.Contains(got.Summary, "Go") {
		t.Errorf("summary = %q, want the text kept", got.Summary)
	}
}

func TestAStatedSalaryTravelsAndAnAbsentOneIsNotZero(t *testing.T) {
	// A posting that states no pay must not read as one offering zero, which is what a
	// non-pointer int would publish.
	if got := NewProjector(testOrigin).JobSummary(aJob(t)); got.SalaryMin != nil || got.SalaryMax != nil {
		t.Error("an unstated salary was published as a figure")
	}

	min := 90000
	j := aJob(t, func(row *db.Job) {
		row.Enrichment = enrichmentJSON(t, enrich.Enrichment{
			SalaryMin: &min, SalaryCurrency: "EUR", SalaryPeriod: "year",
		})
	})

	got := NewProjector(testOrigin).JobSummary(j)
	if got.SalaryMin == nil || *got.SalaryMin != 90000 {
		t.Errorf("salary_min = %v, want 90000", got.SalaryMin)
	}
	if got.SalaryCurrency != "EUR" || got.SalaryPeriod != "year" {
		t.Errorf("salary currency/period = %q/%q, want EUR/year", got.SalaryCurrency, got.SalaryPeriod)
	}
}

func TestDetailCarriesTheFullBodyAndTheSummaryDoesNot(t *testing.T) {
	// Ten full descriptions in one turn spend the context window on text the model reduces
	// to a line each. The full body belongs to the one posting a person picked.
	j := aJob(t, func(row *db.Job) {
		row.Description = "<p>" + strings.Repeat("word ", 400) + "</p>"
	})

	p := NewProjector(testOrigin)
	summary := p.JobSummary(j)
	detail := p.JobDetail(j, nil)

	if len(summary.Summary) >= len(detail.Description) {
		t.Errorf("summary is %d chars and the full body is %d; the preview was not truncated",
			len(summary.Summary), len(detail.Description))
	}
	if !strings.HasPrefix(detail.Description, "word") {
		t.Errorf("description = %.40q, want the full text", detail.Description)
	}
}

func TestDetailSeparatesRequiredFromPreferred(t *testing.T) {
	// The distinction the prose usually buries, and the one a candidate decides on.
	j := aJob(t, func(row *db.Job) {
		row.Enrichment = enrichmentJSON(t, enrich.Enrichment{Requirements: []enrich.Requirement{
			{Text: "5 years of Go", Priority: "required"},
			{Text: "Kubernetes", Priority: "preferred"},
		}})
	})

	got := NewProjector(testOrigin).JobDetail(j, &applyform.Form{Provider: "greenhouse"})

	if len(got.Requirements) != 2 {
		t.Fatalf("got %d requirements, want 2", len(got.Requirements))
	}
	if got.Requirements[0].Priority != "required" || got.Requirements[1].Priority != "preferred" {
		t.Errorf("priorities = %q/%q, want required/preferred",
			got.Requirements[0].Priority, got.Requirements[1].Priority)
	}
	if got.ApplyVia != "greenhouse" {
		t.Errorf("apply_via = %q, want the ATS handling applications", got.ApplyVia)
	}
}

func TestDetailPublishesWhatTheApplicationWillAskFor(t *testing.T) {
	// The part of this catalogue almost nobody else can answer: what the employer's form
	// will actually demand. It is captured in apply_forms, and a candidate deciding whether
	// to start an application wants it BEFORE they open the page — a posting that turns out
	// to want three essays is a different decision from one that wants a CV.
	form := &applyform.Form{
		Provider: "greenhouse",
		Fields: []applyform.Field{
			{ID: "q1", Label: "Why do you want to work here?", RawType: "textarea", Required: true},
			{ID: "q2", Label: "LinkedIn", RawType: "input_text"},
			// The platform's own diversity survey, which ForDisplay drops: it is on every
			// application and tells a candidate nothing about THIS employer.
			{ID: "q3", Label: "Gender", RawType: "select", Required: true, Demographic: true},
		},
	}

	got := NewProjector(testOrigin).JobDetail(aJob(t), form)

	if got.ApplyVia != "greenhouse" {
		t.Errorf("apply_via = %q, want the ATS handling applications", got.ApplyVia)
	}
	if !slices.Contains(got.ApplyRequires, "Why do you want to work here?") {
		t.Errorf("apply_requires = %v, want the required question named", got.ApplyRequires)
	}
	if slices.Contains(got.ApplyRequires, "LinkedIn") {
		t.Errorf("apply_requires = %v, want the OPTIONAL question left out", got.ApplyRequires)
	}
	if slices.Contains(got.ApplyRequires, "Gender") {
		t.Errorf("apply_requires = %v, want the platform's diversity survey left out", got.ApplyRequires)
	}
}

func TestAPostingWithNoCapturedFormPromisesNothingAboutItsApplication(t *testing.T) {
	// Most of the catalogue has no captured form. Saying nothing is the honest answer; an
	// empty list would read as "this employer asks for nothing".
	got := NewProjector(testOrigin).JobDetail(aJob(t), nil)

	if got.ApplyVia != "" || got.ApplyRequires != nil {
		t.Errorf("apply_via = %q, apply_requires = %v; want both absent", got.ApplyVia, got.ApplyRequires)
	}
}

func TestASearchResultSaysWhenThePostingWasFirstSeen(t *testing.T) {
	// The index's own reality class is computed when a document is pushed and goes stale
	// with it — a posting indexed at three days old would still read "fresh" a quarter
	// later. When we first saw the posting is a fact that cannot go stale, so that is
	// what a result carries; the live verdict belongs to get_job.
	seen := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	j := aJob(t, func(row *db.Job) {
		row.CreatedAt = pgtype.Timestamptz{Time: seen, Valid: true}
	})

	if got := NewProjector(testOrigin).JobSummary(j).FirstSeenAt; got != "2026-09-01T08:00:00Z" {
		t.Errorf("first_seen_at = %q, want 2026-09-01T08:00:00Z", got)
	}
}

func TestDetailStatesTheLivenessSignalsItWasHanded(t *testing.T) {
	// The handler computes the verdict; the projection must publish it, criteria included —
	// a level with nothing behind it would be the bare accusation internal/job/ghost exists
	// to avoid.
	seen := "2026-10-14T06:00:00Z"
	j := aJob(t)
	j.LastSeenAt = &seen
	j.Reality = &jobview.Reality{Class: "likely-evergreen", AgeDays: 120, IsFakeFreshness: true}
	j.Ghost = &jobview.Ghost{Level: "possible", Criteria: []string{"evergreen_posting", "ats_absent"}, CriteriaTotal: 4}

	got := NewProjector(testOrigin).JobDetail(j, nil)

	if got.LastSeenAt != seen {
		t.Errorf("last_seen_at = %q, want %q", got.LastSeenAt, seen)
	}
	if got.Reality != "likely-evergreen" || got.FakeFreshness == nil || !*got.FakeFreshness {
		t.Errorf("reality = %q, fake_freshness = %v; want likely-evergreen, true", got.Reality, got.FakeFreshness)
	}
	if got.GhostLevel != "possible" || !slices.Equal(got.GhostCriteria, []string{"evergreen_posting", "ats_absent"}) {
		t.Errorf("ghost = %q %v; want possible with both criteria", got.GhostLevel, got.GhostCriteria)
	}
}

func TestDetailClaimsNothingWhereNoSignalWasComputed(t *testing.T) {
	// A lookup that failed leaves the signal off rather than failing the read. Off must
	// then mean absent on the wire: an empty level or a false flag must not read as a
	// clean bill of health the system never issued.
	got := NewProjector(testOrigin).JobDetail(aJob(t), nil)

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{`"reality"`, `"fake_freshness"`, `"ghost_level"`, `"ghost_criteria"`, `"last_seen_at"`} {
		if strings.Contains(string(raw), field) {
			t.Errorf("%s is on the wire though nothing was computed: %s", field, raw)
		}
	}
}

func TestAComputedFalseFreshnessFlagIsStatedNotDropped(t *testing.T) {
	// Absent means "not computed". Once reality WAS computed, a false flag is an answer and
	// must reach the wire — otherwise a clean result and a failed lookup look the same.
	j := aJob(t)
	j.Reality = &jobview.Reality{Class: "fresh", IsFakeFreshness: false}

	raw, err := json.Marshal(NewProjector(testOrigin).JobDetail(j, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"fake_freshness":false`) {
		t.Errorf("a computed false was dropped from the wire: %s", raw)
	}
}
