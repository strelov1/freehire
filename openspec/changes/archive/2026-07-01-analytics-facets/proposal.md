## Why

Visitors and operators want to understand the shape of the job market the
aggregator holds — "how many vacancies match these criteria" — broken down by
region, category, seniority, work mode, skills, salary, and so on. The catalogue
already carries all of this on every job and Meilisearch already computes
single-dimension facet distributions instantly under the same filter semantics
as search, but nothing exposes those counts. A public analytics page turns the
existing data into an at-a-glance market view (and a SEO surface) for near-zero
backend cost.

## What Changes

- Add a public `GET /api/v1/jobs/facets` endpoint that accepts the **same**
  query params as `/jobs/search` (filters + `q`) and returns the per-value count
  for every facet plus numeric min/max stats, instead of a page of jobs.
- Add a dedicated `FacetCounts` method to the search layer (`internal/search`)
  with its own params/result types — it requests Meilisearch facet distributions
  and decodes them into typed maps. `Search` (which returns ranked job hits) is
  left untouched: counting facets is a distinct responsibility from returning
  results.
- Raise the index's `maxValuesPerFacet` so high-cardinality facets (skills,
  countries) are not truncated at the Meili default of 100. Requires a reindex
  to reach the live index.
- Add a public `/analytics` page (SvelteKit SSR) with interactive drill-down:
  the existing URL-synced filter store + filter panel on one side, plain-CSS
  horizontal bar breakdowns per facet on the other; clicking a value narrows the
  filter and recomputes every breakdown.

This is Phase 1 of a planned three-phase analytics dashboard. Trends over time
(Phase 2, Postgres `date_trunc`) and cross-tabulation (Phase 3) are explicitly
out of scope here and recorded as seams.

## Capabilities

### New Capabilities
- `job-analytics`: a public, filterable facet-distribution endpoint that reports
  vacancy counts per facet value (and numeric min/max) under a given set of
  search filters, built on the Meilisearch facet distribution feature.

### Modified Capabilities
- `web-frontend`: add a public `/analytics` page that renders facet-distribution
  breakdowns with interactive, URL-synced drill-down, reusing the existing job
  filter store and filter panel.

## Impact

- **Backend:** `internal/search` new `FacetCounts` method + `FacetParams`/
  `FacetResult`/`FacetStat` types and the `interface{}` → typed-map decode; index
  `Faceting` setting in `indexSettings()`. New `internal/handler/facets.go`
  (`JobFacets` handler reusing `buildSearchFilter` for query-param parsing), route
  registration in `Register`. `Search` is unchanged.
- **Frontend:** new `web/src/routes/analytics/` route (`+page.server.ts`,
  `+page.svelte`), `web/src/lib/api.ts` (`facetCounts`), `web/src/lib/types.ts`
  (response shape), `+layout.svelte` nav link.
- **Ops:** a `make reindex` is required after deploy for the new
  `maxValuesPerFacet` setting to take effect on the live index.
- **No schema changes**, no new dependencies, no breaking changes.
