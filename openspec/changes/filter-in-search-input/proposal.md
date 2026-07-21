## Why

The **All filters** trigger for the jobs and companies lists currently lives in the
`ListToolbar` — an inline button in the sort row plus a floating edge button that the
scroll reveals once the toolbar leaves the viewport. Because the header search box is
sticky and always visible, hosting the filter trigger there instead makes filters
reachable at every scroll position from one consistent place, and removes the
duplicated floating affordance.

## What Changes

- Add an **All filters** trigger (sliders icon + active-count badge) to the right edge
  of the shared header search box (`HeaderListSearch`), mirroring the existing
  location/work-format scope-prefix on the left. It opens the active page's existing
  filter modal.
- Show this trigger on all viewports across the jobs feed (`/`), the individual
  company page (`/companies/[slug]`), and the companies list (`/companies`).
- **BREAKING (UI)**: Remove both the inline toolbar filter button and the floating
  scroll-revealed filter edge button from `ListToolbar`. The sort control and the
  Swipe (Layers) affordance stay.
- Extend the shared list-search registration seam (`listSearch.svelte.ts`) so a page
  publishes its active-filter count and a modal-open callback for the header to consume.
- Leave the `/my/profile` **Market coverage** filter tab (`FilterEdgeTab`) and the
  desktop `FilterSummary` sidebar untouched; the in-box trigger is an additional entry
  point on desktop, not a replacement for the sidebar.

## Capabilities

### New Capabilities

- `header-filter-trigger`: The **All filters** modal trigger is hosted in the shared
  header search box on jobs-backed and companies list pages, showing an active-filter
  badge and opening the page's own filter modal, on every viewport.

### Modified Capabilities

- `web-frontend`: The companies list's narrow-viewport filter affordance moves from a
  pinned left-edge tab to the header-search-box trigger.

## Impact

- `web/src/lib/listSearch.svelte.ts` — registered-target contract gains `activeFilters`
  and `openFilters`.
- `web/src/lib/components/HeaderListSearch.svelte` — renders the trailing filter trigger.
- `web/src/lib/components/ListToolbar.svelte` — drops the filter button (both variants)
  and its `active`/`onOpenFilters` props.
- `web/src/lib/components/JobsView.svelte`, `web/src/lib/components/CompaniesView.svelte`
  — publish `activeFilters`/`openFilters` into the target; stop passing them to
  `ListToolbar`.
- No backend, API, or database changes.
