package mcpapp

import (
	"strings"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/search/search"
)

// summaryMaxChars bounds a search result's preview.
//
// The index already serves a truncated description, but the bound belongs to this tool: a
// caller handing us a rehydrated body (/agent/jobs/search does) must not turn a page into
// full job descriptions and silently exhaust the model's context.
const summaryMaxChars = 400

// Projector renders our stored shapes into what the tools answer in. It holds the two things
// a projection cannot derive: where this deployment is served from, and which providers
// publish a URL that really is the employer's own page.
type Projector struct {
	origin string
	// publishesEmployerURL is resolved ONCE, here: asking the sources package per job
	// rebuilds every adapter.
	publishesEmployerURL map[string]bool
}

func NewProjector(origin string) Projector {
	return Projector{
		origin:               strings.TrimSuffix(origin, "/"),
		publishesEmployerURL: sources.EmployerURLProviders(),
	}
}

// JobSummary projects one posting as a search result.
func (p Projector) JobSummary(j jobview.Job) JobSummary {
	return JobSummary{
		Slug:            j.PublicSlug,
		Title:           j.Title,
		Company:         j.Company,
		CompanySlug:     j.CompanySlug,
		Location:        j.Location,
		Countries:       j.Countries,
		WorkMode:        j.WorkMode,
		Seniority:       j.Enrichment.Seniority,
		Category:        j.Enrichment.Category,
		EmploymentType:  j.Enrichment.EmploymentType,
		Skills:          j.Skills,
		SalaryMin:       j.Enrichment.SalaryMin,
		SalaryMax:       j.Enrichment.SalaryMax,
		SalaryCurrency:  j.Enrichment.SalaryCurrency,
		SalaryPeriod:    j.Enrichment.SalaryPeriod,
		VisaSponsorship: j.Enrichment.VisaSponsorship,
		EnglishLevel:    j.Enrichment.EnglishLevel,
		PostedAt:        deref(j.PostedAt),
		FirstSeenAt:     deref(j.CreatedAt),
		Summary:         preview(j.Description),
		Source:          j.Source,
		URL:             p.pageURL("/jobs/", j.PublicSlug),
		OfficialJobURL:  sources.EmployerURL(p.publishesEmployerURL, j.Source, j.URL),
	}
}

// JobDetail projects one posting in full. form is its captured application form, nil where
// none was captured — which is most of the catalogue.
func (p Projector) JobDetail(j jobview.Job, form *applyform.Form) JobResult {
	out := JobResult{
		JobSummary:   p.JobSummary(j),
		Description:  htmltext.ToText(j.Description),
		Requirements: requirements(j.Enrichment.Requirements),
	}
	if form != nil {
		out.ApplyVia = form.Provider
		out.ApplyRequires = applyRequires(*form)
	}
	out.LastSeenAt = deref(j.LastSeenAt)
	out.ClosedAt = deref(j.ClosedAt)
	if j.Reality != nil {
		out.Reality = j.Reality.Class
		fake := j.Reality.IsFakeFreshness
		out.FakeFreshness = &fake
	}
	// Ghost is nil at level none, which is most of the catalogue, so absence on the wire
	// already says "nothing observed".
	if j.Ghost != nil {
		out.GhostLevel = j.Ghost.Level
		out.GhostCriteria = j.Ghost.Criteria
	}
	return out
}

// applyRequires lists what the employer's form will refuse the application without.
//
// It reads form.ForDisplay() rather than the raw fields (the curation OJCP and the job page
// use): hidden controls, the mandated diversity survey and consent boilerplate are dropped,
// since they say nothing about THIS employer.
func applyRequires(form applyform.Form) []string {
	display := form.ForDisplay()

	out := make([]string, 0, len(display.Basics)+len(display.Questions))
	out = append(out, display.Basics...)
	for _, question := range display.Questions {
		if question.Required {
			out = append(out, question.Text)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CompanySummary projects one employer as a search result.
func (p Projector) CompanySummary(c search.CompanyDocument) CompanySummary {
	return CompanySummary{
		Slug:     c.Slug,
		Name:     c.Name,
		Tagline:  c.Tagline,
		OpenJobs: openJobs(c.JobCount),
		URL:      p.pageURL("/companies/", c.Slug),
	}
}

// CompanyDetail projects one employer in full, read from the row rather than the index,
// which holds only what a search needs.
func (p Projector) CompanyDetail(c db.Company) CompanyResult {
	return CompanyResult{
		CompanySummary: CompanySummary{
			Slug:     c.Slug,
			Name:     c.Name,
			Tagline:  c.Tagline.String,
			OpenJobs: openJobs(c.JobCount),
			URL:      p.pageURL("/companies/", c.Slug),
		},
		Industries: c.Industries,
		// Stored lowercase, like every geography facet; published as ISO 3166-1 alpha-2.
		HQCountry: strings.ToUpper(strings.TrimSpace(c.HqCountry.String)),
	}
}

// openJobs floors the rollup-written count at zero; nothing here can prove it non-negative.
func openJobs(count int32) int { return max(int(count), 0) }

// pageURL builds a link into this deployment, or nothing at all.
//
// An unconfigured origin would produce a relative path nobody can follow; publishing no
// link is better, since official_job_url and the employer's own site remain.
func (p Projector) pageURL(path, slug string) string {
	if p.origin == "" || slug == "" {
		return ""
	}
	return p.origin + path + slug
}

// preview is the search result's description: plain text, bounded.
//
// It cuts at the last space before the bound, not mid-word: a truncated token reads as an
// unknown term rather than a sentence that stopped.
func preview(description string) string {
	text := strings.TrimSpace(htmltext.ToText(description))
	if utf8.RuneCountInString(text) <= summaryMaxChars {
		return text
	}

	cut := string([]rune(text)[:summaryMaxChars])
	if space := strings.LastIndex(cut, " "); space > 0 {
		cut = cut[:space]
	}
	return strings.TrimSpace(cut) + "…"
}

func requirements(stated []enrich.Requirement) []Requirement {
	if len(stated) == 0 {
		return nil
	}
	out := make([]Requirement, 0, len(stated))
	for _, r := range stated {
		out = append(out, Requirement{Text: r.Text, Priority: r.Priority})
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
