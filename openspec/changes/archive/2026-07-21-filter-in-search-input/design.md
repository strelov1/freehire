## Context

The shared header (`TopBar.svelte`, mounted once in `+layout.svelte`) hosts a
context-dependent search box. On jobs-backed lists and the companies list it renders
`HeaderListSearch.svelte`, which proxies its text input into the active page's store via
the `listSearch.svelte.ts` registry (`setListSearchTarget`/`listSearchTarget`) and
already carries a left scope-prefix trigger (`HeaderLocationFilter`, globe/region).

The **All filters** trigger lives elsewhere: `ListToolbar.svelte` renders it twice — an
inline button in the sort row and a floating edge button revealed by an
`IntersectionObserver` once the toolbar scrolls out of view. `JobsView`/`CompaniesView`
own the filter state and modal, passing `active` (count) and `onOpenFilters` down to
`ListToolbar`. The `/my/profile` Market-coverage tab uses a separate `FilterEdgeTab` and
is out of scope.

## Goals / Non-Goals

**Goals:**
- Host the All-filters trigger in the header search box on `/`, `/companies/[slug]`, and
  `/companies`, on all viewports, opening each page's existing modal with an active-count
  badge.
- Remove both `ListToolbar` filter variants (inline + floating) without disturbing sort
  or Swipe.
- Keep filter ownership (state, count, modal) in the page views; the header only triggers.

**Non-Goals:**
- No change to the filter modal internals, facet logic, or URL sync.
- No change to `/my/profile` `FilterEdgeTab` or the desktop `FilterSummary` sidebar.
- No backend/API/database change.

## Decisions

**Extend the list-search registry rather than lift filter state into a shared store.**
`listSearch.svelte.ts` is the established seam connecting the shared header to the active
page. We add two optional fields to the registered target: `openFilters?: () => void` and
`activeFilters?: () => number` (a getter, so Svelte 5 reactivity flows without copying
state). The page views keep owning the modal and count; the header reads the callbacks.
- *Alternative — a dedicated filter store:* rejected; it would duplicate state that
  already lives in each view and blur module boundaries.
- *Alternative — pass props through TopBar:* rejected; TopBar is mounted in the layout and
  has no reference to the per-route view. The registry already exists for exactly this.

**Render the trigger in `HeaderListSearch` gated on `openFilters` presence.** The button
appears only when the active target exposes `openFilters`, so launcher/listless pages and
any target without a modal render nothing — matching the existing pattern for the location
prefix. Placement: right edge, after the clear button.

**Delete the `ListToolbar` filter variants and their props.** Remove the inline filter
button and the floating filter block; drop the now-unused `active`/`onOpenFilters` props
and update `JobsView`/`CompaniesView` call sites. Retain the `IntersectionObserver`/
`pinned` logic only if the floating Swipe affordance still needs it — otherwise remove it.

## Risks / Trade-offs

- [Embedded `JobsView` on `/companies/[slug]` may not register its target] → Verify
  `setListSearchTarget` runs in the non-standalone (`scope` set) path; if it was gated on
  `standalone`, un-gate the filter fields so the header trigger wires up on the company
  page.
- [Desktop now has two entry points to the modal (sidebar button + header trigger)] →
  Accepted per design; both open the same modal, no state divergence.
- [Reactivity of the badge] → Use a getter (`activeFilters: () => number`) in the
  registration so the count tracks the view's reactive filter state; a plain number
  snapshot would go stale.
- [Component-render coverage is thin in `web/`] → Unit-test the registry contract; verify
  the rendered trigger and removed toolbar buttons via headless-Chrome visual check.
