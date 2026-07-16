// Package cv is the CV-builder domain: the structured CV Document, its sanitizer,
// seeding from the extracted résumé, and PDF rendering behind a Renderer interface.
package cv

import (
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// This file holds ONLY the wire shape (Document + its section types) and the sanitizer,
// so cmd/gen-contracts can generate the TypeScript type from this file alone without
// dragging in the server-only Renderer/Store (mirrors resumeextract.go's structured.go
// vs resumeextract.go split).

// Field bounds for untrusted CV content. The Document is user-editable and — in the
// follow-up tailoring phase — fed to an LLM, so Sanitize is both the persistence guard
// and the prompt-injection guard (same invariant as enrich/resumeextract).
const (
	maxNameRunes     = 120
	maxLocationRunes = 160
	maxEmailRunes    = 200
	maxPhoneRunes    = 60
	maxSummaryRunes  = 2000
	maxBulletRunes   = 600
	maxShortRunes    = 200 // role, company, degree, field, dates, language name/level, links, names

	maxExperience     = 30
	maxEducation      = 20
	maxSkillGroups    = 20
	maxSkillItems     = 60
	maxLanguages      = 20
	maxProjects       = 30
	maxCertifications = 30
	maxBullets        = 20
	maxLinks          = 20
)

// Document is the typed, sanitized CV. Every field is optional; sections the user has
// not filled in are left empty rather than invented, and Sanitize drops empty entries.
type Document struct {
	Header         Header           `json:"header"`
	Summary        string           `json:"summary,omitempty"`
	Experience     []ExperienceItem `json:"experience,omitempty"`
	Education      []EducationItem  `json:"education,omitempty"`
	Skills         []SkillGroup     `json:"skills,omitempty"`
	Languages      []Language       `json:"languages,omitempty"`
	Projects       []Project        `json:"projects,omitempty"`
	Certifications []Certification  `json:"certifications,omitempty"`
}

// Header is the top-of-CV contact block. The tagline under the name is Document.Summary
// (there is no separate headline field — one "summary" term across the CV).
type Header struct {
	FullName string   `json:"full_name,omitempty"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Location string   `json:"location,omitempty"`
	Links    []string `json:"links,omitempty"`
}

// Experience is one work-history entry. Dates are free-form strings as printed on the
// CV (e.g. "2021-03", "Mar 2021", "Present") — no date parsing is attempted.
type ExperienceItem struct {
	Role     string `json:"role,omitempty"`
	Company  string `json:"company,omitempty"`
	Location string `json:"location,omitempty"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	Current  bool   `json:"current,omitempty"`
	// Summary is the one-line company/role context printed under the role header, before
	// the bullets. Stack is the per-role technology line printed after the bullets.
	Summary string   `json:"summary,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
	Stack   []string `json:"stack,omitempty"`
}

// Education is one education entry.
type EducationItem struct {
	Institution string `json:"institution,omitempty"`
	Degree      string `json:"degree,omitempty"`
	Field       string `json:"field,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
}

// SkillGroup is a named cluster of skills (e.g. "Languages" → Go, Python). A group with
// no items and no name is dropped.
type SkillGroup struct {
	Group string   `json:"group,omitempty"`
	Items []string `json:"items,omitempty"`
}

// Language is one spoken/written language and its proficiency level.
type Language struct {
	Name  string `json:"name,omitempty"`
	Level string `json:"level,omitempty"`
}

// Project is one portfolio/side-project entry.
type Project struct {
	Name    string   `json:"name,omitempty"`
	Link    string   `json:"link,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
}

// Certification is one certification/credential.
type Certification struct {
	Name   string `json:"name,omitempty"`
	Issuer string `json:"issuer,omitempty"`
	Year   string `json:"year,omitempty"`
}

// EmptyDocument returns a zero-value skeleton that is already sanitize-stable: a valid
// starting point when a user creates a CV with no résumé to seed from.
func EmptyDocument() Document {
	return Document{}
}

// Sanitize bounds every string, caps every array's cardinality, and drops entries that
// carry no content. Only the sanitized value is persisted or served, so untrusted CV
// text cannot inject unbounded or malformed content.
func (d *Document) Sanitize() {
	d.Header.FullName = clip(d.Header.FullName, maxNameRunes)
	d.Header.Email = clip(d.Header.Email, maxEmailRunes)
	d.Header.Phone = clip(d.Header.Phone, maxPhoneRunes)
	d.Header.Location = clip(d.Header.Location, maxLocationRunes)
	d.Header.Links = limit(nonEmpty(mapStrings(d.Header.Links, maxShortRunes)), maxLinks)

	d.Summary = clip(d.Summary, maxSummaryRunes)

	d.Experience = limit(mapEntries(d.Experience, sanitizeExperience), maxExperience)
	d.Education = limit(mapEntries(d.Education, sanitizeEducation), maxEducation)
	d.Skills = limit(mapEntries(d.Skills, sanitizeSkillGroup), maxSkillGroups)
	d.Languages = limit(mapEntries(d.Languages, sanitizeLanguage), maxLanguages)
	d.Projects = limit(mapEntries(d.Projects, sanitizeProject), maxProjects)
	d.Certifications = limit(mapEntries(d.Certifications, sanitizeCertification), maxCertifications)
}

func sanitizeExperience(e ExperienceItem) (ExperienceItem, bool) {
	e.Role = clip(e.Role, maxShortRunes)
	e.Company = clip(e.Company, maxShortRunes)
	e.Location = clip(e.Location, maxShortRunes)
	e.Start = clip(e.Start, maxShortRunes)
	e.End = clip(e.End, maxShortRunes)
	e.Summary = clip(e.Summary, maxBulletRunes)
	e.Bullets = limit(nonEmpty(mapStrings(e.Bullets, maxBulletRunes)), maxBullets)
	e.Stack = limit(nonEmpty(mapStrings(e.Stack, maxShortRunes)), maxSkillItems)
	keep := e.Role != "" || e.Company != "" || e.Location != "" ||
		e.Start != "" || e.End != "" || e.Summary != "" || len(e.Bullets) > 0
	return e, keep
}

func sanitizeEducation(e EducationItem) (EducationItem, bool) {
	e.Institution = clip(e.Institution, maxShortRunes)
	e.Degree = clip(e.Degree, maxShortRunes)
	e.Field = clip(e.Field, maxShortRunes)
	e.Start = clip(e.Start, maxShortRunes)
	e.End = clip(e.End, maxShortRunes)
	keep := e.Institution != "" || e.Degree != "" || e.Field != "" || e.Start != "" || e.End != ""
	return e, keep
}

func sanitizeSkillGroup(g SkillGroup) (SkillGroup, bool) {
	g.Group = clip(g.Group, maxShortRunes)
	g.Items = limit(nonEmpty(mapStrings(g.Items, maxShortRunes)), maxSkillItems)
	keep := g.Group != "" || len(g.Items) > 0
	return g, keep
}

func sanitizeLanguage(l Language) (Language, bool) {
	l.Name = clip(l.Name, maxShortRunes)
	l.Level = clip(l.Level, maxShortRunes)
	return l, l.Name != "" || l.Level != ""
}

func sanitizeProject(p Project) (Project, bool) {
	p.Name = clip(p.Name, maxShortRunes)
	p.Link = clip(p.Link, maxShortRunes)
	p.Bullets = limit(nonEmpty(mapStrings(p.Bullets, maxBulletRunes)), maxBullets)
	return p, p.Name != "" || p.Link != "" || len(p.Bullets) > 0
}

func sanitizeCertification(c Certification) (Certification, bool) {
	c.Name = clip(c.Name, maxShortRunes)
	c.Issuer = clip(c.Issuer, maxShortRunes)
	c.Year = clip(c.Year, maxShortRunes)
	return c, c.Name != "" || c.Issuer != "" || c.Year != ""
}

// --- small helpers (mirror internal/resumeextract) ---

// clip trims s and truncates to at most max runes on a rune boundary, trimming again so
// a mid-word cut never leaves a trailing space.
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
