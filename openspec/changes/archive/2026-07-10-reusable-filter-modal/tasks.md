## 1. Company staging primitive (pure logic, TDD)

- [x] 1.1 Extract the pure company model into `web/src/lib/companyFacetModel.ts` (`$app`-free: types, serialization, `activeCompanyFilterCount`, pure mutators `toggleCompanyFacet`/`addCompanyFacet`/`removeCompanyFacet`/`clearCompanyFacet`), mirroring `facetModel.ts`; re-point `companyFilters.ts` at it and add `CompanyFilterStore.apply(query)`. Add `stagedCompanyFilters.svelte.ts` (`StagedCompanyFilters`: `FacetStore` + `value`/`active`/`seed`/`params`/`commit`/`clear`), a thin `$state` wrapper over the model like `StagedFilters`.
- [x] 1.2 Unit test (vitest) `companyFacetModel.test.ts`: serialization round-trip + dedup, pure mutators (toggle/add/remove/clearFacet, no-mutation), `activeCompanyFilterCount`. (The `.svelte.ts` staged wrapper is a thin delegation covered by `svelte-check`, matching the untested `StagedFilters` convention — runes files can't import the `$app`-bound store under node vitest.)

## 2. Reusable modal shell

- [x] 2.1 Add `FilterModalShell.svelte`: backdrop/header/rail (from `rail` + `sections` props)/footer (Clear all / Apply / preview) / seed-on-open / Escape+backdrop close / error handling, depending on the minimal staging contract (`active`/`seed`/`params`/`commit`/`clear`), `entryCount(entry)`, and a `pane` snippet.
- [x] 2.2 Refactor `FilterModal.svelte` (job) into a thin wrapper over the shell: create `StagedFilters`, pass job `RAIL` + job `entryCount` + the existing pane if/else as the `pane` snippet; preserve all current public props and behavior.
- [x] 2.3 Verify `JobsView`, `AnalyticsView`, and `my/profile` still open/apply the job modal unchanged (`svelte-check`).

## 3. My filters as a deferred modal tab

- [x] 3.1 Add `StagedFilters.apply(query)` and a canonical-current getter (from `params()`), so `SavedSearches` can read/seed the staged state.
- [x] 3.2 Remove board sharing from `SavedSearches.svelte`: delete `shareActive`/`unshareActive`/`copyBoardLink` + their UI; keep select/save/update/delete + Telegram notify.
- [x] 3.3 Point `SavedSearches` at the staged store (prop), so select seeds staged and save persists staged.
- [x] 3.4 Add the "My filters" rail entry (first, `SAVED` section) in the job `FilterModal`, rendering `SavedSearches` in the pane; omit it when `railKeys` restricts the rail (profile).

## 4. Summary shell + job summary

- [x] 4.1 Add `FilterSummaryShell.svelte`: heading + Reset all, All-filters button (active badge), empty state, chip-group rendering; props `groups`/`active`/`onReset`/`onOpen`.
- [x] 4.2 Refactor `FilterSummary.svelte` (job) to compute its chip groups and render the shell; remove the embedded `<SavedSearches>`.

## 5. Companies on the jobs pattern

- [x] 5.1 Add `CompanyFilterModal.svelte`: wraps the shell with `StagedCompanyFilters`, a `COMPANY_FACETS`-derived rail (single section, `facet`-kind), a `FacetSection` pane, and `staged.facet(param).values.length` counts.
- [x] 5.2 Add `CompanyFilterSummary.svelte`: compute flat per-facet chip groups from `COMPANY_FACETS`, render `FilterSummaryShell`, open `CompanyFilterModal`.
- [x] 5.3 Rewire `CompaniesView.svelte`: desktop `CompanyFilterSummary` + `FilterEdgeTab` (mobile) opening `CompanyFilterModal`; remove the bespoke mobile drawer. Delete `CompanyFiltersPanel.svelte`.

## 6. Profile filter gating

- [x] 6.1 In `routes/my/profile/+page.svelte`, render the filter summary sidebar, `FilterEdgeTab`, and `FilterModal` only when `tab === 'coverage'`; ensure no "My filters" tab appears (railKeys-restricted).

## 7. Verification

- [x] 7.1 `pnpm --dir web svelte-check` and vitest green.
- [x] 7.2 Visual verify (headless Chrome): mobile "My filters" tab reachable; `/companies` modal parity; profile filters only on Market coverage; board sharing absent from the panel but present on `/my/searches`.
