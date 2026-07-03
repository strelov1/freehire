## 1. Company staging primitive (pure logic, TDD)

- [ ] 1.1 Add `web/src/lib/stagedCompanyFilters.svelte.ts`: `StagedCompanyFilters` implementing `FacetStore` + `value`/`active`/`seed`/`params`/`commit`/`clear`, mirroring `StagedFilters` over the `CompanyFilters` shape.
- [ ] 1.2 Unit test (vitest): seed from `CompanyFilters` → toggle/add/remove → `params()` matches `companyFiltersToParams`; `commit(store)` applies to a `CompanyFilterStore`; `active` counts selected values; `clear()` empties.

## 2. Reusable modal shell

- [ ] 2.1 Add `FilterModalShell.svelte`: backdrop/header/rail (from `rail` + `sections` props)/footer (Clear all / Apply / preview) / seed-on-open / Escape+backdrop close / error handling, depending on the minimal staging contract (`active`/`seed`/`params`/`commit`/`clear`), `entryCount(entry)`, and a `pane` snippet.
- [ ] 2.2 Refactor `FilterModal.svelte` (job) into a thin wrapper over the shell: create `StagedFilters`, pass job `RAIL` + job `entryCount` + the existing pane if/else as the `pane` snippet; preserve all current public props and behavior.
- [ ] 2.3 Verify `JobsView`, `AnalyticsView`, and `my/profile` still open/apply the job modal unchanged (`svelte-check`).

## 3. My filters as a deferred modal tab

- [ ] 3.1 Add `StagedFilters.apply(query)` and a canonical-current getter (from `params()`), so `SavedSearches` can read/seed the staged state.
- [ ] 3.2 Remove board sharing from `SavedSearches.svelte`: delete `shareActive`/`unshareActive`/`copyBoardLink` + their UI; keep select/save/update/delete + Telegram notify.
- [ ] 3.3 Point `SavedSearches` at the staged store (prop), so select seeds staged and save persists staged.
- [ ] 3.4 Add the "My filters" rail entry (first, `SAVED` section) in the job `FilterModal`, rendering `SavedSearches` in the pane; omit it when `railKeys` restricts the rail (profile).

## 4. Summary shell + job summary

- [ ] 4.1 Add `FilterSummaryShell.svelte`: heading + Reset all, All-filters button (active badge), empty state, chip-group rendering; props `groups`/`active`/`onReset`/`onOpen`.
- [ ] 4.2 Refactor `FilterSummary.svelte` (job) to compute its chip groups and render the shell; remove the embedded `<SavedSearches>`.

## 5. Companies on the jobs pattern

- [ ] 5.1 Add `CompanyFilterModal.svelte`: wraps the shell with `StagedCompanyFilters`, a `COMPANY_FACETS`-derived rail (single section, `facet`-kind), a `FacetSection` pane, and `staged.facet(param).values.length` counts.
- [ ] 5.2 Add `CompanyFilterSummary.svelte`: compute flat per-facet chip groups from `COMPANY_FACETS`, render `FilterSummaryShell`, open `CompanyFilterModal`.
- [ ] 5.3 Rewire `CompaniesView.svelte`: desktop `CompanyFilterSummary` + `FilterEdgeTab` (mobile) opening `CompanyFilterModal`; remove the bespoke mobile drawer. Delete `CompanyFiltersPanel.svelte`.

## 6. Profile filter gating

- [ ] 6.1 In `routes/my/profile/+page.svelte`, render the filter summary sidebar, `FilterEdgeTab`, and `FilterModal` only when `tab === 'coverage'`; ensure no "My filters" tab appears (railKeys-restricted).

## 7. Verification

- [ ] 7.1 `pnpm --dir web svelte-check` and vitest green.
- [ ] 7.2 Visual verify (headless Chrome): mobile "My filters" tab reachable; `/companies` modal parity; profile filters only on Market coverage; board sharing absent from the panel but present on `/my/searches`.
