// Package coverletter drafts the cover letter a vacancy's application form asks for, from the
// achievement atoms the candidate has actually asserted.
//
// The draft is produced by a fixed three-stage chain — select evidence, draft, audit — and not by
// an autonomous tool-calling agent, for the reason internal/candidate/matchanalysis gives for its
// own chain: deterministic, typed, cacheable. See AGENTS.md.
package coverletter

import (
	"github.com/google/uuid"
)

// Band is how long the letter should be. Two are offered rather than a free-form length,
// because the application forms do not state a limit — no captured apply_forms field carries
// maxlength — so the numbers are a product decision and a slider over them would be a
// precision the data does not support.
type Band string

const (
	BandShort    Band = "short"
	BandStandard Band = "standard"
)

// Bounds are the server-owned ceilings on what the chain may return. The model is never
// trusted to observe them; Sanitize enforces them after the fact, the same discipline
// matchanalysis keeps over its own output.
type Bounds struct {
	// ShortCeiling and StandardCeiling bound the body in runes, per band.
	ShortCeiling    int
	StandardCeiling int
	// Floor is the shortest body that still counts as a letter. The audit stage cuts, and a
	// stage whose only instruction is to cut can cut to nothing; below this the caller keeps
	// the un-audited draft instead.
	Floor int
	// MaxCited caps how many atoms one letter may cite. A letter that cites everything cites
	// nothing — the point of the citation list is that a reader can check it.
	MaxCited int
}

// DefaultBounds are the shipped ceilings. See design.md: the application forms carry no
// maxlength to measure against, so these are the range a recruiter-facing letter occupies in
// practice, expressed in runes because that is what a textarea bounds.
func DefaultBounds() Bounds {
	return Bounds{
		ShortCeiling:    900,
		StandardCeiling: 1800,
		Floor:           200,
		MaxCited:        5,
	}
}

// ceiling is the bound for one band. An unrecognised band takes the standard ceiling: a
// letter clipped a little long is a worse outcome than one clipped to nothing, and an
// unknown band is a caller bug rather than a request for brevity.
func (b Bounds) ceiling(band Band) int {
	if band == BandShort {
		return b.ShortCeiling
	}
	return b.StandardCeiling
}

// Letter is one drafted cover letter: the prose, and the banked atoms it stands on.
type Letter struct {
	Body string `json:"body"`
	// Cited are the atoms the letter's claims rest on, in the order the letter uses them.
	// This is what the UI shows beside the letter and what makes the honesty claim checkable.
	Cited []uuid.UUID `json:"cited_atom_ids"`
	// Language is what the letter was written in, taken from the VACANCY and not from the
	// candidate's profile — the employer is the reader.
	Language string `json:"language"`
}

// Sanitize bounds the letter to what the server allows: the body to the band's ceiling, and
// the citations to the set of atoms actually offered to the chain, deduplicated and capped.
//
// Filtering citations against `offered` is not defensive tidying. The model is handed a set of
// atoms and asked which it used; an id it invents would render beside the letter as evidence
// the letter does not have, and that display is the whole claim this feature makes. A nil
// result is coerced to empty because the column is NOT NULL and pgx sends a nil slice as SQL
// NULL, which a DEFAULT does not cover — the same coercion experience.Sanitize makes.
func (l *Letter) Sanitize(band Band, b Bounds, offered []uuid.UUID) {
	l.Body = clip(l.Body, b.ceiling(band))

	allowed := make(map[uuid.UUID]struct{}, len(offered))
	for _, id := range offered {
		allowed[id] = struct{}{}
	}

	kept := make([]uuid.UUID, 0, len(l.Cited))
	seen := make(map[uuid.UUID]struct{}, len(l.Cited))
	for _, id := range l.Cited {
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		kept = append(kept, id)
		if len(kept) == b.MaxCited {
			break
		}
	}
	l.Cited = kept
}

// BelowFloor reports whether the letter is too short to be one. The audit stage's result is
// checked against this before it replaces the draft it audited.
func (l Letter) BelowFloor(b Bounds) bool {
	return len([]rune(l.Body)) < b.Floor
}

// clip truncates to n runes, never bytes — a body cut mid-rune would reach the column as
// invalid UTF-8.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
