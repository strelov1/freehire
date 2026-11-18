## 1. Backend: the modifier in the shared filter builder

- [x] 1.1 Failing tests in `internal/search/search/query_filter_test.go`: with the
      modifier set, remote + physical modes + a location include build the
      disjunction group (exact Meili expression asserted); no location include →
      plain OR group; no `remote` in the modes → modifier ignored; `_mode=and` →
      modifier ignored; excludes stay separate AND groups; `Narrows` stays false
      for a query carrying only the modifier.
- [x] 1.2 Implement in `query_filter.go`: read `work_mode_remote_anywhere`,
      swap the work-mode include group for the disjunction when active.
- [x] 1.3 Register in `scalarModifiers` (`query_params.go`); extend the
      scalarModifiers test there.
- [x] 1.4 `go test ./internal/search/search/...` passes.

## 2. Frontend: model, profile seeding, UI

- [x] 2.1 `facetModel.ts`: `workModeRemoteAnywhere` on `JobFilters`;
      serialize/parse `work_mode_remote_anywhere`; `activeFilterCount` counts it
      only when it changes the answer (remote+physical included AND a location
      facet included); `savedSearchQuery` keeps it. Round-trip tests.
- [x] 2.2 `filtersFromProfile` sets the flag per design; extend
      `profileFilters.test.ts`.
- [x] 2.3 `FilterModal.svelte` Work format pane: the checkbox, visible under the
      design's condition, with the one-line explanation.
- [x] 2.4 `FilterSummary.svelte`: chip under the same condition.
- [x] 2.5 `pnpm test` (web) passes; `pnpm check` lint passes.

## 3. API contract

- [x] 3.1 `web/static/openapi.yaml`: document the parameter on both job-search
      endpoints (public + agent), semantics and no-op conditions.
- [x] 3.2 Regenerate the internal API reference (`pnpm gen:openapi`,
      `gen:api-docs`), if the generated files are checked in — keep the diff to
      the generator's own output.
- [x] 3.3 `scripts/check-openapi-breaking.sh` passes (additive param).

## 4. Wrap-up

- [ ] 4.1 `openspec validate --changes` clean (openspec CLI unavailable in this environment; structure mirrors existing changes).
- [x] 4.2 Full CI-equivalent locally: `go test ./...`, `golangci-lint run` (search
      packages: 0 issues), web `pnpm test` (3245 passed), `pnpm check` (0 errors),
      `pnpm lint`, `check:dead`, `check:links`, OpenAPI breaking check + smoke tests.
