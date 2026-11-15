// Package coverletter drafts the cover letter a vacancy's application form asks for, from the
// achievement atoms the candidate has actually asserted.
//
// The draft comes from a fixed three-stage chain (select evidence, draft, audit), not an
// autonomous agent, for the reason internal/candidate/matchanalysis gives. See AGENTS.md.
package coverletter

import (
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// Band is how long the letter should be. Two are offered because forms state no limit, so the
// numbers are a product decision and a finer control would imply precision the data lacks.
type Band string

const (
	BandShort    Band = "short"
	BandStandard Band = "standard"
)

// Bounds are the server-owned ceilings on what the chain may return. The model is never
// trusted to observe them; Sanitize enforces them afterwards.
type Bounds struct {
	// ShortCeiling and StandardCeiling bound the body in runes, per band.
	ShortCeiling    int
	StandardCeiling int
	// Floor is the shortest body that still counts as a letter. The audit only cuts and can cut
	// to nothing; below this the caller keeps the un-audited draft.
	Floor int
	// MaxCited caps how many atoms one letter may cite; citing everything makes the list
	// uncheckable.
	MaxCited int
}

// DefaultBounds are the shipped ceilings, in runes because that is what a textarea bounds.
func DefaultBounds() Bounds {
	return Bounds{
		ShortCeiling:    900,
		StandardCeiling: 1800,
		Floor:           200,
		MaxCited:        5,
	}
}

// OrDefault fills every unset or non-positive field from DefaultBounds, field by field. A
// whole-struct zero check would let a caller setting only MaxCited leave ceilings at 0, which
// clips every body to "". A MaxCited of 0 would also disable the cap, since Sanitize stops at
// `len(kept) == MaxCited`, never true after the first append.
func (b Bounds) OrDefault() Bounds {
	d := DefaultBounds()
	if b.ShortCeiling < 1 {
		b.ShortCeiling = d.ShortCeiling
	}
	if b.StandardCeiling < 1 {
		b.StandardCeiling = d.StandardCeiling
	}
	if b.Floor < 1 {
		b.Floor = d.Floor
	}
	if b.MaxCited < 1 {
		b.MaxCited = d.MaxCited
	}
	return b
}

// ceiling is the bound for one band. An unrecognised band (a caller bug) takes the standard
// ceiling, since clipping a little long beats clipping to nothing.
func (b Bounds) ceiling(band Band) int {
	if band == BandShort {
		return b.ShortCeiling
	}
	return b.StandardCeiling
}

// Letter is one drafted cover letter: the prose, and the banked atoms it stands on.
type Letter struct {
	Body string `json:"body"`
	// Cited are the atoms the letter's claims rest on, in the order the letter uses them; the UI
	// shows them beside the letter.
	Cited []uuid.UUID `json:"cited_atom_ids"`
	// Language is what the letter was written in, from the VACANCY, not the candidate's profile.
	Language string `json:"language"`
}

// Sanitize bounds the letter to what the server allows: the body to the band's ceiling, and
// the citations to the set of atoms actually offered to the chain, deduplicated and capped.
//
// Citations are filtered against `offered` because an id the model invents would render as
// evidence the letter does not have. A nil result becomes empty because the column is NOT
// NULL and pgx sends a nil slice as NULL (as experience.Sanitize).
func (l *Letter) Sanitize(band Band, b Bounds, offered []uuid.UUID) {
	l.Body = llm.TruncateRunes(l.Body, b.ceiling(band))

	// One set does both jobs: an id is kept only while in `unused`, and removing it on use makes
	// a repeat a no-op.
	unused := make(map[uuid.UUID]struct{}, len(offered))
	for _, id := range offered {
		unused[id] = struct{}{}
	}

	kept := make([]uuid.UUID, 0, min(len(l.Cited), b.MaxCited))
	for _, id := range l.Cited {
		if _, ok := unused[id]; !ok {
			continue
		}
		delete(unused, id)
		kept = append(kept, id)
		if len(kept) == b.MaxCited {
			break
		}
	}
	l.Cited = kept
}

// BelowFloor reports whether the letter is too short to be one; checked on the audit's result
// before it replaces the draft.
func (l Letter) BelowFloor(b Bounds) bool {
	return len([]rune(l.Body)) < b.Floor
}
