package resumeextract

import (
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// This file holds ONLY the wire shape (Structured + Experience + Education) and its
// sanitizer, so cmd/gen-contracts can generate the TypeScript type from this file alone
// without dragging in the server-only Extractor (mirrors matchanalysis.go vs analyzer.go).

// Field bounds for the untrusted, model-produced structure. Modest on purpose: the
// structure is a display/summary artifact, not the ground-truth CV text (which the fit
// chain still reads in full). These caps keep the persisted JSON and the profile render
// bounded regardless of what the model returns.
const (
	maxNameRunes         = 120
	maxHeadlineRunes     = 200
	maxLocationRunes     = 160
	maxEmailRunes        = 200
	maxPhoneRunes        = 60
	maxSummaryRunes      = 1200
	maxEntrySummaryRunes = 600
	maxShortRunes        = 200 // title, company, degree, institution, dates, language, link

	maxExperience = 30
	maxEducation  = 20
	maxLanguages  = 20
	maxLinks      = 20
	maxSkills     = 80
	maxHighlights = 12
	maxProjects   = 20

	maxYears = 70
)

// Structured is the typed, sanitized résumé. Every field is optional — a value the CV
// does not state is left empty rather than invented (the model is prompted accordingly
// and Sanitize drops empty entries).
type Structured struct {
	FullName   string       `json:"full_name,omitempty"`
	Headline   string       `json:"headline,omitempty"`
	Location   string       `json:"location,omitempty"`
	Email      string       `json:"email,omitempty"`
	Phone      string       `json:"phone,omitempty"`
	Summary    string       `json:"summary,omitempty"`
	TotalYears int          `json:"total_years,omitempty"`
	Experience []Experience `json:"experience,omitempty"`
	Education  []Education  `json:"education,omitempty"`
	Languages  []string     `json:"languages,omitempty"`
	Links      []string     `json:"links,omitempty"`
	Skills     []string     `json:"skills,omitempty"`
	Projects   []Project    `json:"projects,omitempty"`
}

// Experience is one work-history entry. Dates are kept as free-form strings as printed
// on the CV (e.g. "2021-03", "Mar 2021", "Present") — no date parsing is attempted.
// Summary is the role/company one-line context; Highlights are the achievement bullets.
type Experience struct {
	Title      string   `json:"title,omitempty"`
	Company    string   `json:"company,omitempty"`
	Location   string   `json:"location,omitempty"`
	Start      string   `json:"start,omitempty"`
	End        string   `json:"end,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
	Stack      []string `json:"stack,omitempty"`
}

// Project is one portfolio/side-project entry.
type Project struct {
	Name       string   `json:"name,omitempty"`
	Link       string   `json:"link,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
}

// Education is one education entry.
type Education struct {
	Degree      string `json:"degree,omitempty"`
	Institution string `json:"institution,omitempty"`
	Year        string `json:"year,omitempty"`
}

// Sanitize bounds every string, coerces total-years into [0, maxYears], caps each
// array's cardinality, and drops entries that carry no content. Only the sanitized
// value is persisted or served, so untrusted CV text cannot inject unbounded or
// malformed content (the same invariant as enrich/matchanalysis).
func (s *Structured) Sanitize() {
	s.FullName = clip(s.FullName, maxNameRunes)
	s.Headline = clip(s.Headline, maxHeadlineRunes)
	s.Location = clip(s.Location, maxLocationRunes)
	s.Email = clip(s.Email, maxEmailRunes)
	s.Phone = clip(s.Phone, maxPhoneRunes)
	s.Summary = clip(s.Summary, maxSummaryRunes)

	if s.TotalYears < 0 {
		s.TotalYears = 0
	}
	if s.TotalYears > maxYears {
		s.TotalYears = maxYears
	}

	s.Experience = limit(mapEntries(s.Experience, sanitizeExperience), maxExperience)
	s.Education = limit(mapEntries(s.Education, sanitizeEducation), maxEducation)
	s.Languages = limit(nonEmpty(mapStrings(s.Languages, maxShortRunes)), maxLanguages)
	s.Links = limit(nonEmpty(mapStrings(s.Links, maxShortRunes)), maxLinks)
	s.Skills = limit(nonEmpty(mapStrings(s.Skills, maxShortRunes)), maxSkills)
	s.Projects = limit(mapEntries(s.Projects, sanitizeProject), maxProjects)
}

func sanitizeExperience(e Experience) (Experience, bool) {
	e.Title = clip(e.Title, maxShortRunes)
	e.Company = clip(e.Company, maxShortRunes)
	e.Location = clip(e.Location, maxShortRunes)
	e.Start = clip(e.Start, maxShortRunes)
	e.End = clip(e.End, maxShortRunes)
	e.Summary = clip(e.Summary, maxEntrySummaryRunes)
	e.Highlights = limit(nonEmpty(mapStrings(e.Highlights, maxEntrySummaryRunes)), maxHighlights)
	e.Stack = limit(nonEmpty(mapStrings(e.Stack, maxShortRunes)), maxSkills)
	keep := e.Title != "" || e.Company != "" || e.Location != "" ||
		e.Start != "" || e.End != "" || e.Summary != "" || len(e.Highlights) > 0
	return e, keep
}

func sanitizeProject(p Project) (Project, bool) {
	p.Name = clip(p.Name, maxShortRunes)
	p.Link = clip(p.Link, maxShortRunes)
	p.Highlights = limit(nonEmpty(mapStrings(p.Highlights, maxEntrySummaryRunes)), maxHighlights)
	return p, p.Name != "" || p.Link != "" || len(p.Highlights) > 0
}

func sanitizeEducation(e Education) (Education, bool) {
	e.Degree = clip(e.Degree, maxShortRunes)
	e.Institution = clip(e.Institution, maxShortRunes)
	e.Year = clip(e.Year, maxShortRunes)
	keep := e.Degree != "" || e.Institution != "" || e.Year != ""
	return e, keep
}

// --- small helpers ---

// clip trims s and truncates to at most max runes, reusing the shared rune-boundary cut
// (llm.TruncateRunes) and trimming again so a mid-word cut never leaves a trailing space.
func clip(s string, max int) string {
	return strings.TrimSpace(llm.TruncateRunes(strings.TrimSpace(s), max))
}

// limit returns at most n elements of s (nil-safe, preserves order).
func limit[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
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
		out[i] = clip(v, max)
	}
	return out
}

// nonEmpty drops blank strings, preserving order; returns nil when nothing remains.
func nonEmpty(in []string) []string {
	var out []string
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
