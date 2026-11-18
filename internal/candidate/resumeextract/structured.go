package resumeextract

import (
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// This file holds ONLY the wire shape and its sanitizer, so cmd/gen-contracts can generate
// the TypeScript type from it without the server-only Extractor.

// Field bounds for the untrusted, model-produced structure. Modest on purpose: it is a
// display artifact, not the ground-truth CV text.
const (
	maxNameRunes         = 120
	maxHeadlineRunes     = 200
	maxLocationRunes     = 160
	maxEmailRunes        = 200
	maxPhoneRunes        = 60
	maxSummaryRunes      = 1200
	maxEntrySummaryRunes = 600
	maxShortRunes        = 200 // title, company, degree, institution, dates, language, link

	maxExperience     = 30
	maxEducation      = 20
	maxLanguages      = 20
	maxLinks          = 20
	maxSkills         = 80
	maxCertifications = 40
	maxHighlights     = 12
	maxProjects       = 20

	maxYears = 70
)

// Structured is the typed, sanitized résumé. Every field is optional: a value the CV
// does not state stays empty, and Sanitize drops empty entries.
type Structured struct {
	FullName       string       `json:"full_name,omitempty"`
	Headline       string       `json:"headline,omitempty"`
	Location       string       `json:"location,omitempty"`
	Email          string       `json:"email,omitempty"`
	Phone          string       `json:"phone,omitempty"`
	Summary        string       `json:"summary,omitempty"`
	TotalYears     int          `json:"total_years,omitempty"`
	Experience     []Experience `json:"experience,omitempty"`
	Education      []Education  `json:"education,omitempty"`
	Languages      []string     `json:"languages,omitempty"`
	Links          []string     `json:"links,omitempty"`
	Skills         []string     `json:"skills,omitempty"`
	Certifications []string     `json:"certifications,omitempty"`
	Projects       []Project    `json:"projects,omitempty"`
}

// Professional is Structured without the contact fields: what the person has done, not
// who they are. It goes to consumers that must not know the name (the LLM fit chain, an
// agent reading the profile over the API).
//
// The field set is a whitelist on purpose: a field later added to Structured stays
// withheld until added here, where a blacklist would disclose it by default.
type Professional struct {
	Headline       string       `json:"headline,omitempty"`
	Location       string       `json:"location,omitempty"`
	Summary        string       `json:"summary,omitempty"`
	TotalYears     int          `json:"total_years,omitempty"`
	Experience     []Experience `json:"experience,omitempty"`
	Education      []Education  `json:"education,omitempty"`
	Languages      []string     `json:"languages,omitempty"`
	Skills         []string     `json:"skills,omitempty"`
	Certifications []string     `json:"certifications,omitempty"`
	Projects       []Project    `json:"projects,omitempty"`
}

// Professional projects the résumé onto its contact-free view.
func (s Structured) Professional() Professional {
	return Professional{
		Headline:       s.Headline,
		Location:       s.Location,
		Summary:        s.Summary,
		TotalYears:     s.TotalYears,
		Experience:     s.Experience,
		Education:      s.Education,
		Languages:      s.Languages,
		Skills:         s.Skills,
		Certifications: s.Certifications,
		Projects:       s.Projects,
	}
}

// Experience is one work-history entry. Current marks an ongoing role instead of a
// fabricated End. Summary is the one-line role/company context; Highlights are the
// achievement bullets.
type Experience struct {
	Title      string                 `json:"title,omitempty"`
	Company    string                 `json:"company,omitempty"`
	Location   string                 `json:"location,omitempty"`
	Start      *perioddate.PeriodDate `json:"start,omitempty"`
	End        *perioddate.PeriodDate `json:"end,omitempty"`
	Current    bool                   `json:"current,omitempty"`
	Summary    string                 `json:"summary,omitempty"`
	Highlights []string               `json:"highlights,omitempty"`
	Stack      []string               `json:"stack,omitempty"`
}

// Project is one portfolio/side-project entry.
type Project struct {
	Name       string   `json:"name,omitempty"`
	Link       string   `json:"link,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
}

// Education is one education entry; Year is usually year-only.
//
// Degree must stay short (a CV's own degree line, e.g. "BSc Computer Science"), never a
// sentence: internal/dict/edulevel.ForDegree relies on it to match bare "BS"/"MS", which
// are ambiguous in prose.
type Education struct {
	Degree      string                 `json:"degree,omitempty"`
	Institution string                 `json:"institution,omitempty"`
	Year        *perioddate.PeriodDate `json:"year,omitempty"`
}

// Sanitize bounds every string, coerces total-years into [0, maxYears], caps each array's
// length, and drops entries with no content. Only the sanitized value may be persisted or
// served, so untrusted CV text cannot inject unbounded content.
func (s *Structured) Sanitize() {
	s.FullName = llm.TrimTruncateRunes(s.FullName, maxNameRunes)
	s.Headline = llm.TrimTruncateRunes(s.Headline, maxHeadlineRunes)
	s.Location = llm.TrimTruncateRunes(s.Location, maxLocationRunes)
	s.Email = llm.TrimTruncateRunes(s.Email, maxEmailRunes)
	s.Phone = llm.TrimTruncateRunes(s.Phone, maxPhoneRunes)
	s.Summary = llm.TrimTruncateRunes(s.Summary, maxSummaryRunes)

	if s.TotalYears < 0 {
		s.TotalYears = 0
	}
	if s.TotalYears > maxYears {
		s.TotalYears = maxYears
	}

	s.Experience = llm.Limit(mapEntries(s.Experience, sanitizeExperience), maxExperience)
	s.Education = llm.Limit(mapEntries(s.Education, sanitizeEducation), maxEducation)
	s.Languages = llm.Limit(llm.NonEmpty(mapStrings(s.Languages, maxShortRunes)), maxLanguages)
	s.Links = llm.Limit(llm.NonEmpty(mapStrings(s.Links, maxShortRunes)), maxLinks)
	s.Skills = llm.Limit(llm.NonEmpty(mapStrings(s.Skills, maxShortRunes)), maxSkills)
	s.Certifications = llm.Limit(llm.NonEmpty(mapStrings(s.Certifications, maxShortRunes)), maxCertifications)
	s.Projects = llm.Limit(mapEntries(s.Projects, sanitizeProject), maxProjects)
}

func sanitizeExperience(e Experience) (Experience, bool) {
	e.Title = llm.TrimTruncateRunes(e.Title, maxShortRunes)
	e.Company = llm.TrimTruncateRunes(e.Company, maxShortRunes)
	e.Location = llm.TrimTruncateRunes(e.Location, maxShortRunes)
	e.Start = perioddate.Sanitize(e.Start)
	e.End = perioddate.Sanitize(e.End)
	e.Summary = llm.TrimTruncateRunes(e.Summary, maxEntrySummaryRunes)
	e.Highlights = llm.Limit(llm.NonEmpty(mapStrings(e.Highlights, maxEntrySummaryRunes)), maxHighlights)
	e.Stack = llm.Limit(llm.NonEmpty(mapStrings(e.Stack, maxShortRunes)), maxSkills)
	keep := e.Title != "" || e.Company != "" || e.Location != "" ||
		e.Start != nil || e.End != nil || e.Summary != "" || len(e.Highlights) > 0
	return e, keep
}

func sanitizeProject(p Project) (Project, bool) {
	p.Name = llm.TrimTruncateRunes(p.Name, maxShortRunes)
	p.Link = llm.TrimTruncateRunes(p.Link, maxShortRunes)
	p.Highlights = llm.Limit(llm.NonEmpty(mapStrings(p.Highlights, maxEntrySummaryRunes)), maxHighlights)
	return p, p.Name != "" || p.Link != "" || len(p.Highlights) > 0
}

func sanitizeEducation(e Education) (Education, bool) {
	e.Degree = llm.TrimTruncateRunes(e.Degree, maxShortRunes)
	e.Institution = llm.TrimTruncateRunes(e.Institution, maxShortRunes)
	e.Year = perioddate.Sanitize(e.Year)
	keep := e.Degree != "" || e.Institution != "" || e.Year != nil
	return e, keep
}

// mapEntries applies fn to each entry, keeping only those fn marks as non-empty.
func mapEntries[T any](in []T, fn func(T) (T, bool)) []T {
	var out []T
	for _, v := range in {
		if cleaned, keep := fn(v); keep {
			out = append(out, cleaned)
		}
	}
	return out
}

// mapStrings clips each string to max runes.
func mapStrings(in []string, max int) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = llm.TrimTruncateRunes(v, max)
	}
	return out
}
