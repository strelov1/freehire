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

// perRequirement bounds how much evidence one requirement contributes, so one broadly-worded
// requirement cannot fill the whole offered set.
const perRequirement = 4

// answerable reports whether a requirement is one the letter should look for evidence
// against. Only `missing-have` is: something the candidate meets but the CV leaves implicit.
//
// `missing-gap` is filtered out, not deprioritised: retrieving against it only surfaces
// adjacent evidence, an invitation to stretch it into a claim. `covered` and `synonym-only`
// are excluded because the CV already shows them. The filter lives here so a caller need not
// remember it.
func answerable(r matchanalysis.Requirement) bool {
	return r.Status == matchanalysis.StatusMissingHave
}

// Retriever is the narrow slice of the experience bank this package needs: evidence for a
// requirement, and the bank itself when there is no requirement to retrieve against.
type Retriever interface {
	Retrieve(ctx context.Context, userID int64, q experience.Query, limit int) ([]experience.Match, error)
	ListAtoms(ctx context.Context, userID int64) ([]experience.Atom, error)
}

// Gather collects the candidate's evidence for a vacancy's requirements, best first and with
// each atom appearing once.
//
// Retrieval runs per requirement, the bank's scoring grain. One achievement often answers
// several requirements, so an atom keeps its BEST score across them.
//
// The whole bank is the fallback when the vacancy has no requirements or none retrieve
// anything. Gather does NOT apply the provenance gate; Draft does, so a caller cannot skip it.
func Gather(ctx context.Context, r Retriever, userID int64, reqs []matchanalysis.Requirement) ([]experience.Atom, error) {
	best := make(map[uuid.UUID]experience.Match)

	for _, req := range reqs {
		if req.Text == "" || !answerable(req) {
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
	// Sorted by score, then id, so two drafts of the same bank see a stable order.
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
