package search

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

// FacetStatsReader reads the facet-distribution snapshot. *db.Queries satisfies it;
// tests inject a fake.
type FacetStatsReader interface {
	ListFacetStats(ctx context.Context) ([]db.InsightsFacetStat, error)
}

// The generated queries are the production reader. Asserting it here means a
// regenerated signature breaks this file rather than each of the three indexers.
var _ FacetStatsReader = (*db.Queries)(nil)

// skillsFacet is the facet name cmd/rollup-facets writes the per-skill open-job
// counts under.
const skillsFacet = "skills"

// LoadSkillWeights derives skill-rarity weights from the facet-distribution snapshot
// (insights_facet_stats), which cmd/rollup-facets already maintains — the counts this
// needs are the same ones the public /open page renders, so nothing new is computed
// and no worker of our own is required.
//
// The catalogue size is the sum of the SKILL counts, not an open-job total: a job
// naming three skills contributes to three counts, so this sum counts (job, skill)
// pairs, which is the right denominator for an inverse document frequency over
// skills. Rows of other facets are ignored entirely — folding a country count into
// the total would dwarf the skill counts and flatten every weight.
//
// A snapshot with no skill rows yields the zero Weights, which builds no vectors.
// That is the intended degradation before the first rollup — indexing must not fail
// because a rarity snapshot is missing — and never an error.
func LoadSkillWeights(ctx context.Context, r FacetStatsReader) (skillvec.Weights, error) {
	rows, err := r.ListFacetStats(ctx)
	if err != nil {
		return skillvec.Weights{}, fmt.Errorf("search: list facet stats: %w", err)
	}
	counts := make(map[string]int64)
	var total int64
	for _, row := range rows {
		if row.Facet != skillsFacet {
			continue
		}
		counts[row.Value] = row.Count
		total += row.Count
	}
	return skillvec.WeightsFromCounts(counts, total), nil
}
