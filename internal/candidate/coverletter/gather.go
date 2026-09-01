package coverletter

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

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
	best := make(map[uuid.UUID]experience.Match)

	for _, req := range reqs {
		if req.Text == "" {
			continue
		}
		matches, err := r.Retrieve(ctx, userID, experience.Query{Text: req.Text}, perRequirement)
		if err != nil {
			return nil, fmt.Errorf("coverletter: retrieve evidence: %w", err)
		}
		for _, m := range matches {
			if prev, seen := best[m.Atom.ID]; !seen || m.Score > prev.Score {
				best[m.Atom.ID] = m
			}
		}
	}

	if len(best) == 0 {
		atoms, err := r.ListAtoms(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("coverletter: read bank: %w", err)
		}
		if atoms == nil {
			atoms = []experience.Atom{}
		}
		return atoms, nil
	}

	ranked := slices.Collect(maps.Values(best))
	// Sorted by score, then by id so the order is stable for a given bank — an unstable order
	// would make two drafts of the same letter differ for no reason a reader could see.
	slices.SortFunc(ranked, func(x, y experience.Match) int {
		if c := cmp.Compare(y.Score, x.Score); c != 0 {
			return c
		}
		return cmp.Compare(x.Atom.ID.String(), y.Atom.ID.String())
	})

	out := make([]experience.Atom, 0, len(ranked))
	for _, m := range ranked {
		out = append(out, m.Atom)
	}
	return out, nil
}
