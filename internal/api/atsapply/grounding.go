package atsapply

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// GroundingContext is what a drafted answer may reference — the candidate's own asserted
// facts, and nothing else. Atoms.Provenance is always Publishable() here; the filter runs
// once, at read time, so nothing downstream has to re-check it per field.
type GroundingContext struct {
	Atoms []experience.Atom
}

// AtomReader is the one experience-bank read this package needs.
type AtomReader interface {
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
}

// buildGroundingContext reads a candidate's experience bank and keeps only what the
// candidate themselves is the source of (experience.Provenance.Publishable) — the same
// gate internal/cvedit's CV-write path enforces, applied here at read time instead. An
// agent_inferred atom is excluded, not merely unpreferred: a draft must never quote a fact
// the candidate has not confirmed, and this is where that guarantee is made structural
// rather than left to the drafting prompt.
func buildGroundingContext(ctx context.Context, atoms AtomReader, userID int64) (GroundingContext, error) {
	all, err := atoms.ListAtoms(ctx, userID)
	if err != nil {
		return GroundingContext{}, err
	}
	kept := make([]experience.Atom, 0, len(all))
	for _, a := range all {
		if a.Provenance.Publishable() {
			kept = append(kept, a)
		}
	}
	return GroundingContext{Atoms: kept}, nil
}
