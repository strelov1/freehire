## Why

To constrain the jobs feed by geography or work format today, a user must open the
full `FilterModal`. Location and work format are the two highest-intent filters, so
surfacing them directly from the header search box removes a modal round-trip for
the most common narrowing action.

## What Changes

- Add a **scope-prefix trigger** inside the header search box (left of the search
  icon, separated by a divider) that opens a **Location & work-format popover**.
- The popover contains two blocks: a **Work format** pill row (Remote / Hybrid /
  On-site, the `work_mode` facet) above the **existing `LocationPane`** (region →
  country accordion + searchable cities, the `regions`/`countries`/`cities` facets).
- The trigger shows a **computed summary label** of the current selection
  (e.g. `Location`, `Remote`, `Europe +2`, `Remote · Europe +1`), collapsing to an
  icon on narrow screens.
- The popover appears **only on jobs-backed lists** (jobs feed `/` and the company
  jobs list `/companies/:slug`) — not on the company list `/companies` and not on
  the global launcher.
- **No new filter logic, backend, or fetch.** Selections drive the existing
  `FilterStore` facet API (cycle/include/exclude), which already mirrors to the URL,
  localStorage, and the list/counts reload. The popover reuses `JobsView`'s
  already-fetched facet counts.

## Capabilities

### New Capabilities
- `header-location-filter`: a Location & work-format quick-filter embedded in the
  header search box on jobs-backed list pages — a scope-prefix trigger with a
  computed summary label opening a popover (work-format pills + the location pane)
  that drives the page's existing job-filter store.

### Modified Capabilities
<!-- None: the header search input, its `/` hotkey, FilterModal, and the underlying
     facet/URL model are unchanged. This capability is purely additive. -->

## Impact

- **Frontend only** (`web/`):
  - `web/src/lib/listSearch.svelte.ts` — extend `ListSearchTarget` with an optional
    `filterScope` capability.
  - `web/src/lib/components/JobsView.svelte` — register an adapter target carrying
    `filterScope` (store + reactive counts getter).
  - `web/src/lib/components/HeaderLocationFilter.svelte` — new component (trigger +
    popover + work-format pills).
  - `web/src/lib/headerScope.ts` (+ test) — new pure `summarizeScope` helper.
  - `web/src/lib/components/HeaderListSearch.svelte` — render the trigger when
    `filterScope` is present.
- No backend, database, API, or search-index changes. No new HTTP requests.
- Reuses existing exports: `LocationPane`, `WORK_MODE_OPTIONS`, `pillClass`/
  `pillTitle`, the `FacetStore`/`FacetCounts` types.
