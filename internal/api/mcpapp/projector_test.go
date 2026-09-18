package mcpapp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

const testOrigin = "https://freehire.me"

// aJob builds a fixture the way a read path does, through jobview.FromRow.
//
// NEVER as a jobview.Job literal. The literal skips outboundurl.Tag and the facet
// normalisation, so it carries values no production read can produce — which is exactly how
// a utm-tagged official_job_url passed green on the OJCP surface until a review caught it
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

func TestEverySearchResultNamesItsSourceAndBothURLs(t *testing.T) {
	// This channel renders results as prose, where a link is the easiest thing to drop. A
	// posting that arrives without its source can be repeated as though freehire were the
	// employer, and one without the employer's own link sends nobody anywhere.
	got := NewProjector(testOrigin).JobSummary(aJob(t))

	if got.Source != "greenhouse" {
		t.Errorf("source = %q, want the board it came from", got.Source)
	}
	if got.URL != "https://freehire.me/jobs/senior-go-engineer-acme-abc123" {
		t.Errorf("url = %q, want the posting's page on freehire", got.URL)
	}
	// Untagged. jobview stamps utm_source on every URL it serves, and this field is what a
	// consumer deduplicates and domain-verifies against, so the tag defeats both. Our own
	// `url` keeps it.
	if got.OfficialJobURL != "https://boards.greenhouse.io/acme/jobs/1" {
		t.Errorf("official_job_url = %q, want the employer's own posting untagged", got.OfficialJobURL)
	}
}

func TestAnAggregatorsLinkIsNeverCalledTheEmployersOwn(t *testing.T) {
	// An aggregator's stored URL points at the aggregator, not at the employer. Publishing
	// it as official_job_url is a claim we cannot vouch for, and a consumer that
	// domain-verifies on the field would be misled by every one of them. The source name
	// still travels, so attribution is not lost — only the claim is.
	j := aJob(t, func(row *db.Job) {
		row.Source = "adzuna"
		row.URL = "https://www.adzuna.com/details/123"
	})

	got := NewProjector(testOrigin).JobSummary(j)

	if got.OfficialJobURL != "" {
		t.Errorf("official_job_url = %q, want nothing for an aggregator", got.OfficialJobURL)
	}
	if got.Source != "adzuna" {
		t.Errorf("source = %q, want the aggregator still named", got.Source)
	}
}

func TestAnUnconfiguredOriginPublishesNoFreehireURLRatherThanARelativeOne(t *testing.T) {
	// A relative "/jobs/<slug>" is a link the model will render and nobody can follow. The
	// posting is still reachable through official_job_url, which is the better failure.
	got := NewProjector("").JobSummary(aJob(t))

	if got.URL != "" {
		t.Errorf("url = %q, want empty when no origin is configured", got.URL)
	}
	if got.OfficialJobURL == "" {
		t.Error("official_job_url was dropped too; it does not depend on our origin")
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
