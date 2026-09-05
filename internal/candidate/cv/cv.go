// Package cv is the CV-builder domain: the structured CV Document, its sanitizer,
// seeding from the extracted résumé, and PDF rendering behind a Renderer interface.
package cv

import (
	"math"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/llm"
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
	maxLinks          = 20

	// DefaultMaxBullets is the per-experience / per-project bullet ceiling when
	// CV_MAX_BULLETS is unset. Writers must refuse growth past MaxBullets rather
	// than let Sanitize drop trailing content silently.
	DefaultMaxBullets = 20
)

// MaxBullets is the hard ceiling on bullets per experience (and per project).
// Sanitize keeps the first MaxBullets and drops the rest. Mutable so a deployment
// can raise or lower it via CV_MAX_BULLETS without a code change; SetMaxBullets
// is the only writer.
var MaxBullets = DefaultMaxBullets

// SetMaxBullets sets the per-role bullet ceiling. Values below 1 fall back to
// DefaultMaxBullets — a typo must not erase the ceiling.
func SetMaxBullets(n int) {
	if n < 1 {
		n = DefaultMaxBullets
	}
	MaxBullets = n
}

// Page margins are in inches. Sanitize clamps each side to this résumé-sane band and
// defaults a zero (unset) side to defaultMargin, so a fresh skeleton and an old document
// with no margin field both render at half an inch.
const (
	defaultMargin = 0.5
	minMargin     = 0.25
	maxMargin     = 1.5
)

// Typography bounds. Unlike the margin band above, these clamp only a value the candidate
// has actually set: see Style for why zero is not a value here.
const (
	minFontSize   = 8.5 // below this the text stops being comfortably readable in print
	maxFontSize   = 12.0
	fontSizeStep  = 0.5 // a set size is rounded to the nearest multiple
	minLineHeight = 0.3
	maxLineHeight = 0.9
)

// Document is the typed, sanitized CV. Every field is optional; sections the user has
// not filled in are left empty rather than invented, and Sanitize drops empty entries.
type Document struct {
	Margins Margins `json:"margins"`
	// No omitempty: it does nothing on a struct (encoding/json only omits false, 0, nil, and
	// empty strings/slices/maps), so the tag would promise clients an absence they never see
	// while making the generated TypeScript optional for no reason.
	Style          Style            `json:"style"`
	Header         Header           `json:"header"`
	Summary        string           `json:"summary,omitempty"`
	Experience     []ExperienceItem `json:"experience,omitempty"`
	Education      []EducationItem  `json:"education,omitempty"`
	Skills         []SkillGroup     `json:"skills,omitempty"`
	Languages      []Language       `json:"languages,omitempty"`
	Projects       []Project        `json:"projects,omitempty"`
	Certifications []Certification  `json:"certifications,omitempty"`
}

// Margins is the CV's per-side page margin in inches, applied identically to the HTML
// preview and the Typst-rendered PDF. Sanitize clamps each side and defaults an unset
// (zero) side to defaultMargin.
type Margins struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Style is the CV's typography: the typeface, the base type size in points, and the line
// height as a leading multiple of the em. It is part of the document, so it persists with
// the CV, is copied when the CV is tailored, and is not clobbered by field-level patches.
//
// A zero value means "whatever the active template uses" and Sanitize KEEPS IT ZERO. This is
// the opposite of Margins, which resolves an unset side to a concrete 0.5in, and the
// difference is deliberate on both counts:
//
//   - Every CV written before this type existed has no style block, so it renders exactly as
//     it did. There is nothing to migrate.
//   - A template stays a whole design choice. Switching to a template with looser leading
//     actually loosens the leading, because that value was never frozen into the document.
//
// Resolving defaults here — the natural thing to do by analogy with clampMargin — would
// silently bake the current template's typography into every document on its next save.
//
// FontFamily is a registry id (see fonts.go), never a renderer's own face name: the renderer
// resolves it on its own copy of the document at render time.
type Style struct {
	FontFamily string  `json:"font_family,omitempty"`
	FontSize   float64 `json:"font_size,omitempty"`   // points
	LineHeight float64 `json:"line_height,omitempty"` // em of leading
}

// sanitized returns the style with each SET value clamped to its band, and every unset
// (zero) value left unset.
func (s Style) sanitized() Style {
	// A face we cannot render is dropped, not refused: Sanitize repairs a document rather
	// than failing it, the same way an out-of-range margin is clamped. The fonts endpoint
	// exists so a client never has to guess an id.
	if !knownFontID(s.FontFamily) {
		s.FontFamily = ""
	}
	if s.FontSize != 0 {
		s.FontSize = clampFloat(math.Round(s.FontSize/fontSizeStep)*fontSizeStep, minFontSize, maxFontSize)
	}
	if s.LineHeight != 0 {
		s.LineHeight = clampFloat(s.LineHeight, minLineHeight, maxLineHeight)
	}
	return s
}

func clampFloat(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}

// DefaultMargins is the half-inch-per-side margin a fresh CV starts with.
func DefaultMargins() Margins {
	return Margins{Top: defaultMargin, Right: defaultMargin, Bottom: defaultMargin, Left: defaultMargin}
}

// sanitized returns the margins with every side clamped to [minMargin, maxMargin] and a
// zero (unset) side defaulted to defaultMargin.
func (m Margins) sanitized() Margins {
	return Margins{
		Top:    clampMargin(m.Top),
		Right:  clampMargin(m.Right),
		Bottom: clampMargin(m.Bottom),
		Left:   clampMargin(m.Left),
	}
}

func clampMargin(v float64) float64 {
	switch {
	case v == 0:
		return defaultMargin
	case v < minMargin:
		return minMargin
	case v > maxMargin:
		return maxMargin
	default:
		return v
	}
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

// withoutContacts returns a copy of the document carrying only the parts of the header a
// model may see. Location stays: it is not an identifier, and a tailoring agent reasons
// about it when the vacancy is tied to a place. Everything else goes — a name, an address,
// a number, and a personal profile URL all name the candidate equally.
//
// It REBUILDS the header from what is allowed through rather than clearing the fields it
// knows to be identifiers, so a field added to Header is withheld until someone decides
// otherwise here. A list of what to remove discloses whatever it has not been taught about,
// which is the reading internal/candidate/resumeextract already rejected for its own projection
// (structured.go: "A blacklist ... would disclose that new field by default, which is the
// wrong way round"). This projection reaches the same models, so it follows the same rule.
//
// The receiver is a value, so the stored document is untouched — only the copy handed
// to a reader is stripped.
func (d Document) withoutContacts() Document {
	d.Header = Header{Location: d.Header.Location}
	return d
}

// Experience is one work-history entry. Start/End are a structured perioddate.PeriodDate
// (year, optional month) — the same shared type experience.Employment and
// resumeextract.Experience use. Rendering to PDF formats this back to display text
// (see renderer.go); no template deals with the structured value directly.
type ExperienceItem struct {
	Role     string                 `json:"role,omitempty"`
	Company  string                 `json:"company,omitempty"`
	Location string                 `json:"location,omitempty"`
	Start    *perioddate.PeriodDate `json:"start,omitempty"`
	End      *perioddate.PeriodDate `json:"end,omitempty"`
	Current  bool                   `json:"current,omitempty"`
	// Summary is the one-line company/role context printed under the role header, before
	// the bullets. Stack is the per-role technology line printed after the bullets.
	Summary string   `json:"summary,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
	Stack   []string `json:"stack,omitempty"`
}

// Education is one education entry. Start/End are the same structured perioddate.PeriodDate.
type EducationItem struct {
	Institution string                 `json:"institution,omitempty"`
	Degree      string                 `json:"degree,omitempty"`
	Field       string                 `json:"field,omitempty"`
	Start       *perioddate.PeriodDate `json:"start,omitempty"`
	End         *perioddate.PeriodDate `json:"end,omitempty"`
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

// Certification is one certification/credential. Year is the same structured
// perioddate.PeriodDate.
type Certification struct {
	Name   string                 `json:"name,omitempty"`
	Issuer string                 `json:"issuer,omitempty"`
	Year   *perioddate.PeriodDate `json:"year,omitempty"`
}

// EmptyDocument returns an empty skeleton (default margins, no content) that is already
// sanitize-stable: a valid starting point when a user creates a CV with no résumé to seed from.
func EmptyDocument() Document {
	return Document{Margins: DefaultMargins()}
}

// Sanitize bounds every string, caps every array's cardinality, and drops entries that
// carry no content. Only the sanitized value is persisted or served, so untrusted CV
// text cannot inject unbounded or malformed content.
func (d *Document) Sanitize() {
	d.Margins = d.Margins.sanitized()
	d.Style = d.Style.sanitized()

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
	e.Start = perioddate.Sanitize(e.Start)
	e.End = perioddate.Sanitize(e.End)
	e.Summary = clip(e.Summary, maxBulletRunes)
	e.Bullets = limit(nonEmpty(mapStrings(e.Bullets, maxBulletRunes)), MaxBullets)
	e.Stack = limit(nonEmpty(mapStrings(e.Stack, maxShortRunes)), maxSkillItems)
	keep := e.Role != "" || e.Company != "" || e.Location != "" ||
		e.Start != nil || e.End != nil || e.Summary != "" || len(e.Bullets) > 0
	return e, keep
}

func sanitizeEducation(e EducationItem) (EducationItem, bool) {
	e.Institution = clip(e.Institution, maxShortRunes)
	e.Degree = clip(e.Degree, maxShortRunes)
	e.Field = clip(e.Field, maxShortRunes)
	e.Start = perioddate.Sanitize(e.Start)
	e.End = perioddate.Sanitize(e.End)
	keep := e.Institution != "" || e.Degree != "" || e.Field != "" || e.Start != nil || e.End != nil
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
	p.Bullets = limit(nonEmpty(mapStrings(p.Bullets, maxBulletRunes)), MaxBullets)
	return p, p.Name != "" || p.Link != "" || len(p.Bullets) > 0
}

func sanitizeCertification(c Certification) (Certification, bool) {
	c.Name = clip(c.Name, maxShortRunes)
	c.Issuer = clip(c.Issuer, maxShortRunes)
	c.Year = perioddate.Sanitize(c.Year)
	return c, c.Name != "" || c.Issuer != "" || c.Year != nil
}

// --- small helpers (mirror internal/candidate/resumeextract) ---

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
