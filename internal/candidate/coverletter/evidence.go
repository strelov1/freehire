package coverletter

import (
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

// Publishable narrows a bank to the atoms that may reach a letter: the ones the CANDIDATE
// asserted. It delegates to experience.Provenance.Publishable rather than restating the
// admissible labels, so there is one list.
//
// The filter runs over the chain's INPUT, before any model call: a model that never sees an
// inferred atom cannot cite one, and checking prose would mean fuzzy sentence-to-atom matching.
// Provenance decides publication in the service, never in a prompt (experience/AGENTS.md).
func Publishable(atoms []experience.Atom) []experience.Atom {
	kept := make([]experience.Atom, 0, len(atoms))
	for _, a := range atoms {
		if a.Provenance.Publishable() {
			kept = append(kept, a)
		}
	}
	return kept
}

// IDs lists the atoms' identifiers in order. The offered set and the Sanitize citation filter
// must both come from the same already-filtered slice; the raw bank would re-admit what
// Publishable withheld.
func IDs(atoms []experience.Atom) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(atoms))
	for _, a := range atoms {
		ids = append(ids, a.ID)
	}
	return ids
}
