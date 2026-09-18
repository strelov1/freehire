package mcpapp

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/job/jobview"
)

const testOrigin = "https://freehire.me"

func aJob() jobview.Job {
	return jobview.Job{
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
		Enrichment: enrich.Enrichment{
			Seniority:      "senior",
			Category:       "backend",
			EmploymentType: "full_time",
		},
	}
}

func TestEverySearchResultNamesItsSourceAndBothURLs(t *testing.T) {
	// This channel renders results as prose, where a link is the easiest thing to drop. A
	// posting that arrives without its source can be repeated as though freehire were the
	// employer, and one without the employer's own link sends nobody anywhere.
	got := NewProjector(testOrigin).JobSummary(aJob())

	if got.Source != "greenhouse" {
		t.Errorf("source = %q, want the board it came from", got.Source)
	}
	if got.URL != "https://freehire.me/jobs/senior-go-engineer-acme-abc123" {
		t.Errorf("url = %q, want the posting's page on freehire", got.URL)
	}
	if got.OfficialJobURL != "https://boards.greenhouse.io/acme/jobs/1" {
		t.Errorf("official_job_url = %q, want the employer's own posting", got.OfficialJobURL)
	}
}

func TestAnUnconfiguredOriginPublishesNoFreehireURLRatherThanARelativeOne(t *testing.T) {
	// A relative "/jobs/<slug>" is a link the model will render and nobody can follow. The
	// posting is still reachable through official_job_url, which is the better failure.
	got := NewProjector("").JobSummary(aJob())

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
	j := aJob()
	j.Enrichment.Seniority = "staff"

	if got := NewProjector(testOrigin).JobSummary(j); got.Seniority != "staff" {
		t.Errorf("seniority = %q, want it passed through unchanged", got.Seniority)
	}
}

func TestTheSummaryIsPlainTextNotMarkup(t *testing.T) {
	// The stored description is markup. Handing tags to a model makes it pay tokens to read
	// past them, ten times over in a search result.
	got := NewProjector(testOrigin).JobSummary(aJob())

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
	if got := NewProjector(testOrigin).JobSummary(aJob()); got.SalaryMin != nil || got.SalaryMax != nil {
		t.Error("an unstated salary was published as a figure")
	}

	j := aJob()
	min := 90000
	j.Enrichment.SalaryMin = &min
	j.Enrichment.SalaryCurrency = "EUR"
	j.Enrichment.SalaryPeriod = "year"

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
	j := aJob()
	j.Description = "<p>" + strings.Repeat("word ", 400) + "</p>"

	p := NewProjector(testOrigin)
	summary := p.JobSummary(j)
	detail := p.JobDetail(j, "")

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
	j := aJob()
	j.Enrichment.Requirements = []enrich.Requirement{
		{Text: "5 years of Go", Priority: "required"},
		{Text: "Kubernetes", Priority: "preferred"},
	}

	got := NewProjector(testOrigin).JobDetail(j, "greenhouse")

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
