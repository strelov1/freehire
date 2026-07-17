## 1. Data model

- [x] 1.1 Add migration `migrations/00NN_insights_facet_stats.sql` creating
  `insights_facet_stats(facet text, value text, count bigint, PRIMARY KEY(facet,
  value))` with a header comment following the `0022_insights_rollups.sql`
  convention (pure projection, atomic delete-and-reinsert, initdb + manual-prod
  gotcha).
- [x] 1.2 Add sqlc queries in `internal/db/queries/`: `DeleteAllFacetStats`,
  `InsertFacetStat`, and `ListFacetStats` (ordered by facet, count DESC).
- [x] 1.3 Regenerate sqlc (`make sqlc`) and confirm `internal/db` builds.

## 2. Rollup worker

- [x] 2.1 Write a failing unit test for the FacetCounts→rows mapping: given a
  `search.FacetResult` for the four attributes, it produces the correct
  `(facet, value, count)` rows keyed by public param names, and only for
  `countries`, `skills`, `seniority`, `work_mode`.
- [x] 2.2 Implement the mapping helper to pass 2.1.
- [x] 2.3 Add `cmd/rollup-facets/main.go` (mirrors `cmd/rollup-stats`):
  `worker.Bootstrap`, build `search.NewClient(cfg.MeiliURL, cfg.MeiliKey)`, call
  `FacetCounts` for the four attributes BEFORE opening the transaction, then
  `DeleteAllFacetStats` + batched `InsertFacetStat` in one tx, commit, and exit
  non-zero on any failure.
- [x] 2.4 Add an integration test (testcontainers, as in `internal/db`) covering
  the atomic swap: a populated snapshot is fully replaced, a mid-rebuild reader
  sees the prior snapshot, and a rerun with unchanged input is idempotent.

## 3. Public endpoint

- [x] 3.1 Write a failing handler test: empty table → 200 with empty facet maps;
  populated table → 200 with `{data:{facets:{countries,skills,seniority,work_mode}}}`
  in the shape `/open` consumes.
- [x] 3.2 Implement the `StatsFacets` handler (reads `ListFacetStats`, groups
  rows into the response shape) to pass 3.1.
- [x] 3.3 Register `GET /api/v1/stats/facets` next to the other public `/stats/*`
  routes.

## 4. Frontend rewire

- [x] 4.1 Add `statsFacets()` to the web API client (`web/src/lib/api.ts`),
  returning the same facets shape.
- [x] 4.2 Repoint `web/src/routes/open/+page.server.ts` from `api.facetCounts`
  to `api.statsFacets`; keep the `Promise.allSettled` degradation and update the
  source link to `/api/v1/stats/facets` (in `+page.svelte`, per the spec delta).
- [x] 4.3 Run `npm run check` (svelte-check) and confirm the page type-checks.

## 5. Verify & operationalize

- [x] 5.1 Verified `GET /api/v1/stats/facets` end-to-end against the live dev
  Postgres (real Go server): populated snapshot → the four distributions in the
  `/open` shape; empty snapshot → 200 with four empty maps; `/open` no longer
  references `/api/v1/jobs/facets`. Migration 0026 applied cleanly to the live DB
  (also exercising the manual-prod path). The worker's DB swap + FacetCounts→rows
  mapping are covered by the integration + unit tests; running the worker against a
  populated Meilisearch index is a deploy-time smoke test (local Meili has no
  ingested catalogue).
- [x] 5.2 Documented `cmd/rollup-facets` in AGENTS.md (Layout + Commands) with its
  `MEILI_*` env and daily cadence. The cron entry lives in freehire-ops and the
  manual prod `CREATE TABLE` step is captured in the migration header + design
  Migration Plan (both outside this repo's runtime).
