package mcpapp

import (
	"strings"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/search/search"
)

// summaryMaxChars bounds a search result's preview.
//
// The search index already serves a truncated description, so most of the time this cuts
// nothing. It is applied anyway because the bound is a PROPERTY of this tool rather than of
// whoever filled the field: a caller that hands us a rehydrated body — which
// /agent/jobs/search does by design — must not be able to turn a ten-result page into ten
// full job descriptions. The failure would be silent, expensive, and visible only as a
// model that ran out of room to answer.
const summaryMaxChars = 400

// Projector renders our stored shapes into what the tools answer in. It holds the two things
// a projection cannot derive: where this deployment is served from, and which providers
// publish a URL that really is the employer's own page.
type Projector struct {
	origin string
	// publishesEmployerURL is resolved ONCE, here, for the same reason the OJCP projector
	// resolves its copy once: the answer is a per-provider fact, and asking the sources
	// package per job rebuilds all 223 adapters — 35µs and 394 allocations, ten times over
	// on a ten-result page.
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
		Summary:         preview(j.Description),
		Source:          j.Source,
		URL:             p.pageURL("/jobs/", j.PublicSlug),
		OfficialJobURL:  p.officialJobURL(j),
	}
}

// officialJobURL is the employer's own page for the posting, or nothing where we cannot
// vouch that the stored URL is one.
//
// Two rules, and both were found by a test rather than by reading. The provider gate comes
// first: an aggregator's stored URL points at the aggregator, so publishing it as the
// employer's own is a claim we cannot make — and this channel renders it as a link a person
// will click expecting the employer. The source name still travels either way, so
// attribution survives; only the claim is withheld.
//
// Then the tracking parameter is stripped. jobview stamps utm_source on every URL it
// serves, and this field is what a consumer deduplicates and domain-verifies against, so
// the tag defeats both purposes. Our own `url` keeps it.
func (p Projector) officialJobURL(j jobview.Job) string {
	if !p.publishesEmployerURL[j.Source] {
		return ""
	}
	return outboundurl.Untag(j.URL)
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
	return out
}

// applyRequires lists what the employer's form will refuse the application without.
//
// It reads form.ForDisplay() rather than the raw fields, which is the same curation the
// OJCP surface's required_fields uses and the same one the job page shows a person: the
// platform's hidden controls, its mandated diversity survey and its consent boilerplate are
// dropped. All three are on every application and say nothing about THIS employer, and a
// chat turn is the worst place to spend words on them.
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

// CompanyDetail projects one employer in full, read from the row rather than the index —
// the same split the job tools make, and for the same reason: the index holds what a search
// needs and the row holds what a chosen record is worth reading.
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
		// Stored lowercase, like every geography facet here, and published as ISO 3166-1
		// alpha-2 — the same trap a posting's country fell into on the OJCP surface.
		HQCountry: strings.ToUpper(strings.TrimSpace(c.HqCountry.String)),
	}
}

// openJobs floors the materialised count at zero. It is written by a rollup, so nothing
// here can prove it non-negative, and a negative figure is not an answer any reader should
// have to interpret.
func openJobs(count int32) int { return max(int(count), 0) }

// pageURL builds a link into this deployment, or nothing at all.
//
// An origin the deployment never configured would produce a relative path — a link the
// model will happily render and nobody can follow. Publishing no link is the better
// failure: a posting is still reachable through official_job_url, and an employer through
// their own site.
func (p Projector) pageURL(path, slug string) string {
	if p.origin == "" || slug == "" {
		return ""
	}
	return p.origin + path + slug
}

// preview is the search result's description: plain text, bounded.
//
// It cuts at the last space before the bound rather than mid-word, because a truncated
// token reads to a model as a term it does not know rather than as a sentence that stopped.
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
