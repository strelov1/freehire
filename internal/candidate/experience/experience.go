// Package experience is the experience bank: the durable, per-user record of what a
// candidate has actually done. An Employment is a place where something happened (a
// job or a project); an Atom is one piece of evidence at the grain of a CV bullet.
//
// The bank exists because every other store of experience is a cache. The structured
// résumé is derived from the uploaded file and replaced whenever that file changes; a
// tailored CV is derived from a vacancy and thrown away with it. Knowledge a candidate
// confirms — to the tailoring agent, in a chat, on their profile page — had nowhere to
// live. Here it accumulates, and only its owner removes it.
package experience

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// This file holds ONLY the wire shape and its sanitizer, so cmd/gen-contracts can
// generate the TypeScript types from this file alone without dragging in the
// server-only Store and Importer (mirrors cv.go vs store.go, structured.go vs
// resumeextract.go).

// Field bounds for untrusted bank content. Atoms are written by the user AND by the
// model, and are fed back into LLM prompts on retrieval, so Sanitize is both the
// persistence guard and the prompt-injection guard — the same invariant as
// internal/candidate/cv and internal/candidate/resumeextract.
const (
	maxClaimRunes   = 400  // one CV-bullet-grade sentence
	maxContextRunes = 1200 // how it was done: raw material for reframing
	maxSummaryRunes = 600  // the employment's one-line context
	maxShortRunes   = 200  // company, role, location, date labels, source ref

	maxMetrics = 10
	maxSkills  = 40
)

// Employment kinds. A place where evidence was produced is either paid work or a
// project the candidate ran; anything else is not a place.
const (
	KindJob     = "job"
	KindProject = "project"
)

// Provenance records how the bank came to hold an atom, and it is the honest wall
// moved out of the system prompt and into the data.
type Provenance string

// The four ways an atom enters the bank. The first three are the candidate speaking —
// through their own CV, their own words, or their own typing. The fourth is the model
// speaking, which is a legal thing to record and an illegal thing to publish.
const (
	ProvenanceCVImport      Provenance = "cv_import"
	ProvenanceStatedInChat  Provenance = "stated_in_chat"
	ProvenanceManual        Provenance = "manual"
	ProvenanceAgentInferred Provenance = "agent_inferred"
)

// Sentinel errors, mapped to 4xx by the handler and returned verbatim to the model by
// the assistant tools — the message is the model's only route to self-correction.
var (
	// ErrEmptyClaim is an atom with no claim: context without an assertion is not evidence.
	ErrEmptyClaim = errors.New("experience: an atom needs a claim")
	// ErrInvalidProvenance is a provenance outside the known four.
	ErrInvalidProvenance = errors.New("experience: provenance must be one of cv_import, stated_in_chat, manual, agent_inferred")
	// ErrEmptyEmployment is an employment naming neither a company nor a role.
	ErrEmptyEmployment = errors.New("experience: an employment needs a company or a role")
	// ErrInvalidKind is a kind outside job/project.
	ErrInvalidKind = errors.New("experience: kind must be job or project")
)

// Valid reports whether p is one of the four known values. An unknown provenance is
// refused rather than defaulted: defaulting would decide, silently and in the wrong
// direction, whether a claim may reach a CV.
func (p Provenance) Valid() bool {
	switch p {
	case ProvenanceCVImport, ProvenanceStatedInChat, ProvenanceManual, ProvenanceAgentInferred:
		return true
	}
	return false
}

// Publishable reports whether an atom of this provenance may be rendered into a CV.
//
// This is the single predicate the whole capability turns on. Everything the candidate
// asserted — in their own CV, in their own words, by their own typing — may be
// published. What the model originated may not, until the candidate confirms it and it
// is re-stamped. An unknown provenance is never publishable, so a value that somehow
// escaped validation fails closed.
func (p Provenance) Publishable() bool {
	switch p {
	case ProvenanceCVImport, ProvenanceStatedInChat, ProvenanceManual:
		return true
	}
	return false
}

// Employment is a place where evidence was produced. Start/End are a structured
// perioddate.PeriodDate (year, optional month) — the same shared type resumeextract.Experience
// and cv.ExperienceItem use.
// JSON is kind-aware (see employment_json.go): jobs emit `company`, projects emit `name`
// for the same stored Company field.
type Employment struct {
	ID       uuid.UUID              `json:"id"`
	Kind     string                 `json:"kind"`
	Company  string                 `json:"-"` // place label; wire: company (job) or name (project)
	Role     string                 `json:"role,omitempty"`
	Location string                 `json:"location,omitempty"`
	Start    *perioddate.PeriodDate `json:"start,omitempty"`
	End      *perioddate.PeriodDate `json:"end,omitempty"`
	Current  bool                   `json:"current,omitempty"`
	Summary  string                 `json:"summary,omitempty"`
	// Link is the outbound URL for a portfolio project. Empty for jobs; for projects it
	// is what Document.Projects.Link needs when seeding a CV from the bank.
	Link string `json:"link,omitempty"`
	// Stack is the role's technologies, as printed on the CV's per-role stack line.
	// It lives here rather than on each atom: copying it down would tag a bullet about
	// hiring with the role's message broker.
	Stack []string `json:"stack,omitempty"`
}

// Atom is one piece of evidence. Claim is the sentence a CV bullet would carry; Context
// is how it was done, kept as raw material for reframing against a vacancy's language.
// Skills are canonical slugs from the same dictionary that produces the jobs.skills
// facet, so matching evidence to a requirement is a set intersection.
type Atom struct {
	ID           uuid.UUID  `json:"id"`
	EmploymentID *uuid.UUID `json:"employment_id,omitempty"`
	Claim        string     `json:"claim"`
	Context      string     `json:"context,omitempty"`
	Metrics      []string   `json:"metrics,omitempty"`
	Skills       []string   `json:"skills,omitempty"`
	// Provenance is READ-ONLY on a write. Store derives it from the Author the entry point
	// names, and ignores whatever is here — see Author for the laundering route that closes.
	// It is serialised because every reader needs to see the standing of a claim.
	Provenance Provenance `json:"provenance"`
	SourceRef  string     `json:"source_ref,omitempty"`
}

// Sanitize bounds every string and caps every array. Skills are canonicalized through
// the shared dictionary rather than merely clipped, so the bank cannot hold a skill the
// vacancy side has never heard of — an out-of-vocabulary tag would be evidence nothing
// could ever match.
func (a *Atom) Sanitize() {
	a.Claim = clip(a.Claim, maxClaimRunes)
	a.Context = clip(a.Context, maxContextRunes)
	a.SourceRef = clip(a.SourceRef, maxShortRunes)
	a.Metrics = orEmpty(limit(nonEmpty(mapStrings(a.Metrics, maxShortRunes)), maxMetrics))
	a.Skills = orEmpty(limit(skilltag.Canonicalize(a.Skills, skilltag.WithResumeAcronyms()), maxSkills))
}

// Validate reports the first reason this atom cannot be persisted. It runs after
// Sanitize, so a claim that was only whitespace has already become empty.
func (a Atom) Validate() error {
	if a.Claim == "" {
		return ErrEmptyClaim
	}
	if !a.Provenance.Valid() {
		return ErrInvalidProvenance
	}
	return nil
}

// Sanitize bounds every string on the employment.
func (e *Employment) Sanitize() {
	e.Company = clip(e.Company, maxShortRunes)
	e.Role = clip(e.Role, maxShortRunes)
	e.Location = clip(e.Location, maxShortRunes)
	e.Start = perioddate.Sanitize(e.Start)
	e.End = perioddate.Sanitize(e.End)
	e.Summary = clip(e.Summary, maxSummaryRunes)
	e.Link = clip(e.Link, maxShortRunes)
	e.Stack = orEmpty(limit(skilltag.Canonicalize(e.Stack, skilltag.WithResumeAcronyms()), maxSkills))
}

// Validate reports the first reason this employment cannot be persisted.
func (e Employment) Validate() error {
	if e.Kind != KindJob && e.Kind != KindProject {
		return ErrInvalidKind
	}
	if e.Company == "" && e.Role == "" {
		return ErrEmptyEmployment
	}
	return nil
}

// claimNoiseRE matches everything that is not a letter, a digit or a space, in
// already-lowercased text.
var claimNoiseRE = regexp.MustCompile(`[^\p{L}\p{N} ]+`)

// claimSpaceRE matches a run of whitespace.
var claimSpaceRE = regexp.MustCompile(`\s+`)

// ClaimKey is the normalized form of a claim, and the key import matches on. It is
// stored as its own column under a unique index, so "the same CV re-uploaded does not
// duplicate atoms" is a database constraint rather than a property of the import code.
//
// Normalization is deliberately shallow: case, punctuation and whitespace are noise
// between two spellings of one achievement, but numbers are not — "20s to 1s" and
// "30s to 1s" are different claims and must not collide. Nothing stems or reorders
// words, because a key that merges two genuinely different achievements loses evidence
// silently, which is far worse than keeping a near-duplicate the user can delete.
func ClaimKey(claim string) string {
	lowered := strings.ToLower(strings.TrimSpace(claim))
	stripped := claimNoiseRE.ReplaceAllString(lowered, " ")
	return strings.TrimSpace(claimSpaceRE.ReplaceAllString(stripped, " "))
}

// --- small helpers (mirror internal/candidate/cv) ---

// clip trims s and truncates to at most max runes on a rune boundary, trimming again so
// a mid-word cut never leaves a trailing space.
func clip(s string, max int) string {
	return strings.TrimSpace(truncateRunes(strings.TrimSpace(s), max))
}

// truncateRunes clamps s to at most max runes. Local rather than reused from internal/platform/llm
// so this file stays free of any server-only dependency — cmd/gen-contracts generates the
// TypeScript wire types from this file alone (see the package-level comment on Employment).
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// limit returns at most n elements of s (nil-safe, preserves order).
func limit[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// mapStrings clips each string to max runes.
func mapStrings(in []string, max int) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = clip(v, max)
	}
	return out
}

// orEmpty turns a nil slice into an empty one. The array columns are NOT NULL, and pgx
// sends a nil slice as SQL NULL — a column DEFAULT does not apply to an explicitly-passed
// NULL, so an atom whose skills all failed to resolve would be rejected by the database
// (SQLSTATE 23502) rather than stored with none. Coercing here is the sanitizer's job:
// it is what makes the value one the schema will accept.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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
