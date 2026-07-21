## 1. List-search registry seam

- [x] 1.1 Extend the `listSearch.svelte.ts` target contract with optional
  `openFilters?: () => void` and `activeFilters?: () => number`; unit-test that a
  registered target exposes them via `listSearchTarget()` and that they clear on unregister

## 2. Header trigger

- [x] 2.1 Render an All-filters trigger (sliders icon + active-count badge) at the right
  edge of `HeaderListSearch.svelte`, shown only when the active target exposes
  `openFilters`; clicking calls `openFilters()`, badge reads `activeFilters()`, and the
  search text/`/`-hotkey are untouched

## 3. Wire the page views into the seam

- [x] 3.1 In `JobsView.svelte`, publish `openFilters` (open `FilterModal`) and
  `activeFilters` (`filters.active`) into the registered target; confirm the registration
  runs in the embedded/non-standalone path so `/companies/[slug]` wires up, un-gating if
  needed
- [x] 3.2 In `CompaniesView.svelte`, publish `openFilters` (open `CompanyFilterModal`) and
  `activeFilters` into the registered target

## 4. Remove the toolbar filter trigger

- [x] 4.1 Remove the inline and floating filter buttons from `ListToolbar.svelte`, drop the
  now-unused `active`/`onOpenFilters` props (and any observer logic no longer needed by the
  Swipe affordance), and update the `JobsView`/`CompaniesView` call sites; keep the sort
  control and Swipe affordance

## 5. Verify

- [x] 5.1 Run `svelte-check` and the web build; visually verify (headless Chrome) the
  trigger + badge on `/`, `/companies`, and `/companies/[slug]`, that the toolbar shows no
  filter button at any scroll position, and that `/my/profile` Market-coverage is unchanged
