package coverletter

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

const MaxSelectedAtoms = 50

var (
	ErrInvalidSelection  = errors.New("coverletter: select between 1 and 50 confirmed achievements; review unavailable facts")
	ErrSelectionConflict = errors.New("coverletter: evidence changed in another session; review and save again")
)

// Selection is absent for automatic retrieval; an existing empty selection is explicit.
type Selection struct {
	Revision int64       `json:"revision"`
	AtomIDs  []uuid.UUID `json:"atom_ids"`
}

func ResolveSelection(ids []uuid.UUID, bank []experience.Atom) ([]experience.Atom, error) {
	if len(ids) == 0 || len(ids) > MaxSelectedAtoms {
		return nil, ErrInvalidSelection
	}
	available := make(map[uuid.UUID]experience.Atom, len(bank))
	for _, a := range bank {
		if a.Provenance.Publishable() {
			available[a.ID] = a
		}
	}
	out := make([]experience.Atom, 0, len(ids))
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		a, ok := available[id]
		if id == uuid.Nil || !ok || seen[id] {
			return nil, ErrInvalidSelection
		}
		seen[id] = true
		out = append(out, a)
	}
	return out, nil
}

// EvidenceFingerprint includes every offered atom field, so edits invalidate the draft.
func EvidenceFingerprint(atoms []experience.Atom) string {
	ordered := slices.Clone(atoms)
	slices.SortFunc(ordered, func(a, b experience.Atom) int { return cmp.Compare(a.ID.String(), b.ID.String()) })
	payload, _ := json.Marshal(ordered)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
