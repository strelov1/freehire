package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/facetsnapshot"
)

// groupFacetStats folds the flat snapshot rows into facet param → value → count. Every
// covered facet is always present (empty map if it has no rows), so the shape is stable.
func groupFacetStats(rows []db.InsightsFacetStat) map[string]map[string]int64 {
	facets := make(map[string]map[string]int64, len(facetsnapshot.Facets))
	for _, f := range facetsnapshot.Facets {
		facets[f] = map[string]int64{}
	}
	for _, r := range rows {
		dist, ok := facets[r.Facet]
		if !ok {
			// A facet no longer in facetsnapshot.Facets but still in the table: skip it.
			continue
		}
		dist[r.Value] = r.Count
	}
	return facets
}

// StatsFacets serves the value→count distribution for countries, skills, seniority and
// work_mode, precomputed daily by cmd/rollup-facets. An unpopulated snapshot yields empty
// facet maps (200), not an error.
func (h *statsHandlers) StatsFacets(ctx context.Context) (*apigen.StatsFacetsOK, error) {
	rows, err := h.queries.ListFacetStats(ctx)
	if err != nil {
		return nil, err
	}

	// Marshalled here, not modelled: the keys are facet values and json.Marshal keeps them sorted.
	facets, err := json.Marshal(groupFacetStats(rows))
	if err != nil {
		return nil, err
	}
	return &apigen.StatsFacetsOK{Data: apigen.FacetStats{Facets: facets}}, nil
}

// SkillNeighbours serves the skills most often listed beside one skill, most frequent
// first, from the insights_skill_neighbours snapshot cmd/rollup-facets rebuilds daily.
// It replaces a live skills facet count, too costly under a crawler burst. An unknown
// skill, or a snapshot not yet built, is an empty list (200).
func (h *statsHandlers) SkillNeighbours(ctx context.Context, params apigen.SkillNeighboursParams) (*apigen.SkillNeighbours, error) {
	skill := strings.ToLower(params.Slug)
	rows, err := h.queries.ListSkillNeighbours(ctx, skill)
	if err != nil {
		return nil, err
	}
	items := make([]apigen.SkillNeighbour, len(rows))
	for i, r := range rows {
		items[i] = apigen.SkillNeighbour{Skill: r.Neighbour, Count: r.Count}
	}
	return &apigen.SkillNeighbours{Data: items, Meta: apigen.SkillNeighboursMeta{Skill: skill}}, nil
}
