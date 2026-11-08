package facetsnapshot

import (
	"context"
	"fmt"
	"sort"

	"github.com/strelov1/freehire/internal/search/search"
)

// NeighbourLimit is how many neighbours are kept per skill. The glossary page shows
// eight, but only after dropping skills it has no page for, so it needs headroom.
const NeighbourLimit = 24

// NeighbourRow is one (skill, neighbour) pair of the co-occurrence snapshot: Count
// open postings list both.
type NeighbourRow struct {
	Skill     string
	Neighbour string
	Count     int64
}

// facetCounter is the one engine call the pass makes; *search.Client satisfies it.
type facetCounter interface {
	FacetCounts(ctx context.Context, p search.FacetParams) (search.FacetResult, error)
}

// Neighbours computes, for every skill, the skills listed most often beside it. It asks
// the engine the same question the glossary page used to ask on every render — the
// skills distribution under `skills = <skill>`, one exact value (the query-string
// parser would split a value at commas), so the snapshot matches what the page showed
// for every real skill. Open, canonical postings only, as everywhere. One query at a
// time: this runs nightly and must not compete with the site for the engine. Any error
// aborts the pass so the previous snapshot keeps serving.
func Neighbours(ctx context.Context, fc facetCounter, skills []string, limit int) ([]NeighbourRow, error) {
	attr := search.StringFacets["skills"]
	var rows []NeighbourRow
	for _, skill := range skills {
		res, err := fc.FacetCounts(ctx, search.FacetParams{
			Filter: search.Filter([]string{search.Eq(attr, skill)}),
			Facets: []string{attr},
		})
		if err != nil {
			return nil, fmt.Errorf("neighbours of %q: %w", skill, err)
		}
		rows = append(rows, topNeighbours(skill, res.Facets[attr], limit)...)
	}
	return rows, nil
}

// topNeighbours ranks a distribution by count (ties by name, so reruns agree) and keeps
// the first limit values other than the skill itself.
func topNeighbours(skill string, dist map[string]int64, limit int) []NeighbourRow {
	rows := make([]NeighbourRow, 0, len(dist))
	for v, n := range dist {
		if v != skill && n > 0 {
			rows = append(rows, NeighbourRow{Skill: skill, Neighbour: v, Count: n})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Neighbour < rows[j].Neighbour
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}
