package atsapply

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// GroundingContext is what a drafted answer may reference: the candidate's own asserted
// facts only. Atoms are filtered once at read time (Publishable), not per field.
type GroundingContext struct {
	Atoms []experience.Atom
}

// AtomReader is the one experience-bank read this package needs.
type AtomReader interface {
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
}

// buildGroundingContext reads the experience bank and keeps only atoms the candidate is
// the source of (experience.Provenance.Publishable), the same gate as internal/cvedit. An
// agent_inferred atom is excluded outright so a draft never quotes an unconfirmed fact.
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
