## 1. Search layer (`internal/search`)

- [x] 1.1 Add `FacetStat` type and pure decode helpers that turn Meili's raw `FacetDistribution`/`FacetStats` JSON into the typed maps; table-test the decode (incl. empty and high-cardinality cases). (`FacetParams`/`FacetResult` land in 1.2 with the method that uses them.)
- [x] 1.2 Add `FacetCounts(ctx, FacetParams) (FacetResult, error)` issuing a `limit:0` Meili search with `Facets` + filter + query, using the decode helper; leave `Search` untouched
- [x] 1.3 Add `Faceting{MaxValuesPerFacet: 300}` to `indexSettings()` so skills/countries are not capped at 100

## 2. HTTP endpoint (`internal/handler`)

- [x] 2.1 Add `facetAttributes()` returning the facetable attribute list (single source shared with `searchStringFacets`/`FilterableAttributes`)
- [x] 2.2 Add `facetCounter` interface (just `FacetCounts`) and the `JobFacets` handler reusing `buildSearchFilter`; unit-test with a fake: params parse → filter passed through → distribution surfaced; nil backend → 503
- [x] 2.3 Register `GET /api/v1/jobs/facets` next to `jobs/search` in `Register`

## 3. Frontend (`web/`)

- [x] 3.1 Add the `FacetCounts` response type to `web/src/lib/types.ts` and a `facetCounts(params)` method to `web/src/lib/api.ts`
- [x] 3.2 Add `web/src/routes/analytics/+page.server.ts` — SSR-load facets under the URL filter (empty by default)
- [x] 3.3 Add `web/src/routes/analytics/+page.svelte` + `AnalyticsView.svelte` + `FacetBreakdown.svelte` — reuse `FilterStore` + `FiltersPanel`; render per-facet plain-CSS bar breakdowns sorted by count, total headline + salary stat, click-to-drill-down
- [x] 3.4 Add an `/analytics` nav link (in `TopBar.svelte`, which holds the nav — not `+layout.svelte`; just one `links` entry, no conflict with main's working-tree edits)

## 4. Verify & ops

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` (all pass, gofmt clean); in `web/` `npm run check` (svelte-check 0 errors/0 warnings); `go test -tags=integration -run TestIntegration_FacetCounts ./internal/search/` passes against a live Meilisearch (distribution + filter-narrowing + numeric stats verified end-to-end)
- [x] 4.2 Post-deploy `make reindex` step (for the new `maxValuesPerFacet`) recorded in proposal Impact and design.md Risks
