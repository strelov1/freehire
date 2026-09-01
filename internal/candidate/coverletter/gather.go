package coverletter

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// perRequirement bounds how much evidence one requirement contributes. Retrieval already
// ranks and drops zero-scoring atoms, so this only stops one broadly-worded requirement from
// filling the whole offered set on its own.
const perRequirement = 4

// Retriever is the narrow slice of the experience bank this package needs: evidence for a
// requirement, and the bank itself when there is no requirement to retrieve against.
type Retriever interface {
	Retrieve(ctx context.Context, userID int64, q experience.Query, limit int) ([]experience.Match, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
}

// Gather collects the candidate's evidence for a vacancy's requirements, best first and with
// each atom appearing once.
//
// Retrieval runs per requirement because that is the grain the bank scores at — one atom
// against one requirement. Two requirements answered by the same achievement is the common
// case rather than an edge one ("Kafka" and "event-driven architecture" are one bullet), so
// an atom keeps its BEST score across the requirements it answered and appears once.
//
// The bank is the fallback in two cases: a vacancy with no requirements to retrieve against,
// and requirements that retrieve nothing. Both mean the candidate's evidence does not line up
// requirement-by-requirement, which is a reason to write from the bank rather than a reason to
// refuse — the provenance gate downstream still decides what may be used.
//
// Gather does NOT apply that gate. Draft does, so that a caller cannot skip it.
func Gather(ctx context.Context, r Retriever, userID int64, reqs []matchanalysis.Requirement) ([]experience.Atom, error) {
	best := make(map[uuid.UUID]float64)
	byID := make(map[uuid.UUID]experience.Atom)

	for _, req := range reqs {
		if req.Text == "" {
			continue
		}
		matches, err := r.Retrieve(ctx, userID, experience.Query{Text: req.Text}, perRequirement)
		if err != nil {
			return nil, fmt.Errorf("coverletter: retrieve evidence: %w", err)
		}
		for _, m := range matches {
			if score, seen := best[m.Atom.ID]; !seen || m.Score > score {
				best[m.Atom.ID] = m.Score
			}
			byID[m.Atom.ID] = m.Atom
		}
	}

	if len(byID) == 0 {
		atoms, err := r.ListAtoms(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("coverletter: read bank: %w", err)
		}
		if atoms == nil {
			atoms = []experience.Atom{}
		}
		return atoms, nil
	}

	out := make([]experience.Atom, 0, len(byID))
	for _, a := range byID {
		out = append(out, a)
	}
	// Sorted by score, then by id so the order is stable for a given bank — an unstable order
	// would make two drafts of the same letter differ for no reason a reader could see.
	sort.Slice(out, func(i, j int) bool {
		si, sj := best[out[i].ID], best[out[j].ID]
		if si != sj {
			return si > sj
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out, nil
}
