## 1. Role dictionary (`internal/roletag`)

- [x] 1.1 Define the role catalog: canonical slug → label for composite roles
  (generated from a seniority-label map × a category role-noun map, e.g.
  `senior_backend` → "Senior Backend Engineer") and curated named roles
  (`founding_engineer`, `fractional_cto`, `cloud_solutions_engineer`,
  `solutions_engineer`, `staff_engineer`, `technical_lead`). Export it for reuse.
- [x] 1.2 Implement `Derive(seniority, category, title) []string`: composite
  `{seniority}_{category}` only when both non-empty; named-role whole-word alias
  matches from the title (`wordmatch.Contains`, unicode boundary); dedupe; never
  guess; every emitted slug present in the catalog.
- [x] 1.3 Table-driven tests mirroring `classify_test.go`: composite present,
  composite requires both axes, named role regardless of grid, EN/RU aliases,
  nothing-resolvable → empty, dedupe, every derivable slug is in the catalog.

## 2. Index wiring (`internal/search`)

- [x] 2.1 `search.FromJob`: compute `roles` via `roletag.Derive(job.Seniority,
  job.Category, job.Title)` and add it to the document (index-only, not in the
  public wire shape).
- [x] 2.2 Declare `roles` as a filterable + facetable attribute in the index
  settings (`client.go` `facetSettings`).
- [x] 2.3 Add `role` → `roles` to `StringFacets` (`query_filter.go`) so the
  filter builder handles `role` / `role_exclude` / `role_mode` for free; add a
  filter-builder test for OR-within / exclude / AND-with-other-facet.
- [x] 2.4 Document-shape test: a job with seniority+category+title indexes a
  `roles` array; assert `posted_ts`-style index-only behavior (absent from the
  public read shape).

## 3. Facet distribution (`internal/handler`)

- [x] 3.1 Include `roles` in the `/api/v1/jobs/facets` distribution keyed by the
  public param `role` (`facets.go` attribute list + `facetParamByAttr`).
- [x] 3.2 Handler/integration test: `role` appears in the returned `facets` map
  with per-slug counts under the applied scope.

## 4. Contracts codegen (`cmd/gen-contracts`)

- [x] 4.1 Emit the role catalog (slug → label map) from `roletag` into the web
  contracts (`web/src/lib/contracts.ts`), matching the `emitMap` pattern used for
  `COUNTRY_REGION_MAP`.
- [x] 4.2 Regenerate contracts and verify the generated role catalog is present
  and well-formed.

## 5. Frontend role picker (`web/`)

- [x] 5.1 Add a `role` entry to `FACETS` (`web/src/lib/facets.ts`): control
  `select`, `dynamic:true`, `hasAndOr`, excludable; labels from the generated
  catalog map (flat, busiest-first — no grouping, same as `skills`).
- [x] 5.2 Add the Role control to the ROLE rail section (`filterSections.ts` /
  `FilterModal.svelte`) alongside seniority and specialization; wire it to the
  live `counts.facets.role` path (reuse `FacetSection`).
- [x] 5.3 Verify `role` / `role_exclude` / `role_mode` round-trip through
  `filtersToParams` / `filtersFromParams` (generic path — add a vitest case).
- [x] 5.4 Visual check of the picker (busiest-first counts, typeahead, exclude,
  labels) via svelte-check + a screenshot pass.

## 6. Rollout

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`; run the web unit
  tests. Note the post-deploy reindex step (the facet is empty until reindex).
