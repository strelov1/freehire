## 1. Pure scope-summary helper

- [x] 1.1 Write failing vitest `web/src/lib/headerScope.test.ts` covering `summarizeScope(store)`: no selection → `{ icon: 'globe', label: 'Location' }`; `work_mode=[remote]` → `{ icon: 'remote', label: 'Remote' }`; `regions=[eu]` → `{ icon: 'globe', label: 'Europe' }`; `regions=[eu,uk]` → `{ icon: 'globe', label: 'Europe +1' }`; `countries=[DE]` (no region) → `{ icon: 'globe', label: 'Germany' }`; `work_mode=[remote]`+`regions=[eu,uk]` → `{ icon: 'remote', label: 'Remote · Europe +1' }`; an excluded geo value counts toward the `+N` roll-up. Use a minimal fake `Pick<FacetStore,'facet'>`.
- [x] 1.2 Implement `web/src/lib/headerScope.ts`: `summarizeScope(store)` reads `work_mode`/`regions`/`countries`/`cities` facets (include ∪ exclude), picks the icon from the first selected work format (`remote`/`hybrid`/`onsite`) else `'globe'`, and builds the label — work-format label (via `WORK_MODE_LABELS`/title-case) and/or the first geography label (region via `REGION_LABELS`, country via `countryLabel`, else the raw city) with a `+N` roll-up over the remaining geo values, joined by `·`. Geo ordering: regions, then countries, then cities.
- [x] 1.3 Run `npx vitest run src/lib/headerScope.test.ts` — all pass.

## 2. Bridge capability

- [x] 2.1 Extend `ListSearchTarget` in `web/src/lib/listSearch.svelte.ts` with an optional `filterScope?: { store: FacetStore; counts(): FacetCounts | null }` (import `FacetStore` from `$lib/facets`, `FacetCounts` from `$lib/types`); document it as jobs-only. Keep the base `value`/`setQuery` contract unchanged.

## 3. JobsView adapter registration

- [x] 3.1 In `web/src/lib/components/JobsView.svelte`, change the `setListSearchTarget(filters)` call (~line 218) to register an adapter object: `{ get value() { return filters.value; }, setQuery: (q) => filters.setQuery(q), filterScope: { store: filters, counts: () => counts } }`, where `counts` is the existing `$state.raw<FacetCounts|null>`. Leave the `setListSearchTarget(null)` cleanup as-is. Confirm `CompaniesView.svelte` is untouched (bare `CompanyFilterStore`, no `filterScope`).

## 4. HeaderLocationFilter component

- [x] 4.1 Create `web/src/lib/components/HeaderLocationFilter.svelte` with props `{ store: FacetStore; counts: FacetCounts | null }`. Trigger button: icon from `summarizeScope(store)` (Lucide `Globe`/`House` etc.) + label + `ChevronDown`; label hidden on `max-sm` (icon+caret only). Uses `$state` `open`, outside-click + Escape + `afterNavigate` close, mirroring `HeaderMenu.svelte`.
- [x] 4.2 In the popover: header row with a `Location & format` title and a `Clear all` button that calls `store.clearFacet('work_mode'|'regions'|'countries'|'cities')`. Then a `Work format` labelled pill row over `WORK_MODE_OPTIONS`, each pill using `pillClass`/`pillTitle` (from `./facets/pill`) and `onclick={() => store.cycle('work_mode', opt.value)}` with include/exclude state read from `store.facet('work_mode')`. Then `<LocationPane {store} {counts} />`. Popover container `max-h-[70vh] overflow-y-auto`, anchored under the trigger.
- [x] 4.3 Visually verify the component in isolation on the running dev server (trigger states, pill cycling, pane rendering, open/close). Run `npx svelte-check` — no new errors.

## 5. Wire into the header search box

- [x] 5.1 In `web/src/lib/components/HeaderListSearch.svelte`, read the active `listSearchTarget()`; when `target.filterScope` is set, render `<HeaderLocationFilter store={target.filterScope.store} counts={target.filterScope.counts()} />` plus a vertical divider immediately before the search `<Search>` icon inside the box. When absent, render the box exactly as today.

## 6. Verify end-to-end

- [x] 6.1 On the dev server, on the jobs feed `/`: open the popover, select Remote + a region + a country + a city; confirm the list and counts reload, the URL gains `work_mode`/`regions`/`countries`/`cities`, and the trigger label reflects the selection. Confirm the popover is absent on `/companies` and on a non-list page (global launcher).
- [x] 6.2 Run `npx vitest run`, `npx svelte-check`, and `npm run lint` in `web/`; all clean.
