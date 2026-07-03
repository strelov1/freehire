## 1. Test infrastructure

- [x] 1.1 Add `vitest` dev-dependency and a `test` script to `web/package.json`; add a minimal `vitest.config.ts` (jsdom not required — logic is pure) and confirm `npm run test` runs an empty/passing suite.

## 2. Model + URL serialization (TDD on pure logic)

- [x] 2.1 Write failing tests in `web/src/lib/filters.test.ts` for the new `FacetState { include, exclude, matchAll }`: `emptyFacet`, `filtersToParams` (include → `param`, exclude → `param_exclude`, `param_mode=and` only when `include.length > 1`), `filtersFromParams` (round-trip + both-sets-present, and the exclude-wins de-dup when a value appears in both), `canonicalQuery` round-trip, and `activeFilterCount = include + exclude`.
- [x] 2.2 Change `FacetState` to `{ include: string[]; exclude: string[]; matchAll: boolean }` and update `emptyFacet`, `filtersToParams`, `filtersFromParams`, `activeFilterCount` until 2.1 is green. Keep the URL shape unchanged.

## 3. Store API (TDD where pure)

- [x] 3.1 Write failing tests for the store transitions: `setSign` (off/include/exclude), `cycle` (off→include→exclude→off), `pick` (off→include, else→off), `toggleSign` (include↔exclude), `add` (→include, no-op on blank/dup), `remove` (→off), and `setMatchAll`/`clearFacet`.
- [x] 3.2 Implement the new methods on `FilterStore`, remove `setExclude` and the old `toggle`, and mirror the same surface on `StagedFilters`; update the `FacetSelection`/`FacetStore` interfaces in `facets.ts`. Make 3.1 green.

## 4. Facet controls (visual)

- [x] 4.1 `PillGroup.svelte`: render three states (off/include/exclude) keyed off the value's sign, call `cycle`, and set a `title` naming the next state. Replace the `exclude` boolean prop with the include/exclude sets.
- [x] 4.2 `SearchSelect.svelte` and `RemoteSearchSelect.svelte`: dropdown pick → `pick`; render selected values as an include chip group and a destructive exclude chip group, each chip with an include↔exclude toggle (`toggleSign`) and a remove (`remove`).
- [x] 4.3 `FacetSection.svelte` + `FacetHeader.svelte`: drop the whole-facet Exclude toggle; keep Clear (resets both sets) and the match-all (Any/All) toggle gated on `include.length > 1`. Wire `ChipFacet.svelte` pills to `cycle`.
- [x] 4.4 `TokenInput.svelte`: confirm it drives `add`/`remove` as include-only (no exclude affordance needed for its facets).

## 5. Summary + company parity

- [x] 5.1 `FilterSummary.svelte`: render included and excluded values distinctly (destructive style for excludes).
- [x] 5.2 Update the company filter store to implement the new `FacetStore` methods as include-only (all company facets are non-excludable); confirm the company filter page still compiles and filters.

## 6. Verify

- [x] 6.1 `npm run test` green; `npm run check` (svelte-check) clean across the `FacetState` chain; `npm run lint` no new errors.
- [x] 6.2 Visual verify (headless-Chrome) on the live skills facet: include `nodejs` + exclude `php`/`.net` yields `?skills=nodejs&skills_exclude=php&skills_exclude=.net`; three-state region pills cycle correctly. Remove the throwaway `filter-prototype.html`.
