package coverletter

import (
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

// Publishable narrows a bank to the atoms that may reach a letter: the ones the CANDIDATE
// asserted. It delegates the rule to experience.Provenance.Publishable rather than restating
// the three admissible labels, because a second copy of that list is a second answer, and the
// drift between them would look exactly like a working gate.
//
// The filter runs over the chain's INPUT, before any model call, and not over its output.
// Checking the finished prose would mean matching sentences back to atoms — the fuzzy problem
// the gate exists to avoid — whereas a model that never sees an inferred atom cannot cite one.
// This is the same placement experience/AGENTS.md argues for: provenance decides publication,
// and the check lives in the service, never in a prompt.
func Publishable(atoms []experience.Atom) []experience.Atom {
	kept := make([]experience.Atom, 0, len(atoms))
	for _, a := range atoms {
		if a.Provenance.Publishable() {
			kept = append(kept, a)
		}
	}
	return kept
}

// IDs lists the atoms' identifiers in order. The chain offers exactly this set to the model
// and Sanitize filters the model's citations against exactly this set, so both must be taken
// from the same already-filtered slice — deriving either from the raw bank would re-admit
// what Publishable just withheld.
func IDs(atoms []experience.Atom) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(atoms))
	for _, a := range atoms {
		ids = append(ids, a.ID)
	}
	return ids
}
