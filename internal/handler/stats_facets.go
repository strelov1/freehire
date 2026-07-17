package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/facetsnapshot"
)

// groupFacetStats folds the flat snapshot rows into the `facets` envelope the /open
// page consumes: facet param → value → count. Every covered facet is always present
// (empty map when the snapshot has no rows for it yet), so the shape is stable for
// clients regardless of whether the daily rollup has run.
func groupFacetStats(rows []db.InsightsFacetStat) map[string]map[string]int64 {
	facets := make(map[string]map[string]int64, len(facetsnapshot.Facets))
	for _, f := range facetsnapshot.Facets {
		facets[f] = map[string]int64{}
	}
	for _, r := range rows {
		dist, ok := facets[r.Facet]
		if !ok {
			// A facet no longer covered (e.g. removed from facetsnapshot.Facets) but
			// still lingering in the snapshot table — ignore it rather than surface a
			// facet the page does not render.
			continue
		}
		dist[r.Value] = r.Count
	}
	return facets
}

// StatsFacets serves the public, unauthenticated facet-distribution snapshot the
// /open transparency page renders: the value→count distribution for countries,
// skills, seniority, and work_mode, precomputed daily by cmd/rollup-facets. Reading
// from the snapshot keeps /open off the live Meilisearch facet count. Aggregate-only
// — the query selects nothing but per-value counts. An unpopulated snapshot yields
// the four empty facet maps (200), not an error.
func (a *API) StatsFacets(c *fiber.Ctx) error {
	rows, err := a.queries.ListFacetStats(c.Context())
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": fiber.Map{"facets": groupFacetStats(rows)}})
}
