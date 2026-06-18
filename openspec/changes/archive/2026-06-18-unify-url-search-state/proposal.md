## Why

The frontend has three divergent copies of one mechanism — "input → debounce →
URL → reload data" — and one of them drops characters during fast typing: the
jobs/analytics `FilterStore` debounces the *URL write* (a `goto` navigation), so
while a navigation is in flight the late `load` result overwrites just-typed
characters. The companies search avoids this by writing the URL synchronously,
and analytics piles a second debounce on top of `FilterStore`'s. One correct,
shared primitive removes the bug structurally and collapses the duplication.

Separately, the jobs/companies lists page with an explicit "Load more" button;
infinite scroll that triggers when the user reaches the bottom is the intended
UX, with the button retained as an accessible fallback.

## What Changes

- Add a single reusable client-side primitive `UrlSyncedState<T>` that mirrors
  search/filter state into the URL **synchronously** (`replaceState`) and exposes
  a debounced `applied` snapshot as the sole "reload" signal. Synchronous URL
  writes mean back/forward reconciliation can never clobber in-flight typing — the
  dropped-character bug is gone by construction (no guard needed).
- Rewrite `FilterStore` (jobs/analytics filters) as a thin wrapper over the
  primitive; discrete changes (facet pills, checkboxes) apply immediately,
  continuous input (text, salary slider) debounces.
- Point the companies search and the analytics counts at the same primitive,
  removing analytics' duplicate debounce and the double data-load.
- **BREAKING (UX)**: Replace the "Load more" button as the *primary* paging
  control with infinite scroll — an `IntersectionObserver` sentinel at the list
  bottom (no pre-fetch ahead of the viewport). The existing button stays as a
  fallback: a spinner while loading, a "Try again" on error, and a
  keyboard/screen-reader-reachable control (these users do not scroll-trigger).
- No backend or API changes; `Paginator` is unchanged (infinite scroll is just a
  new trigger for its existing `loadMore()`).

## Capabilities

### New Capabilities
<!-- none — this reshapes existing frontend behavior, no new capability surface -->

### Modified Capabilities
- `web-frontend`: the "Jobs list with pagination" requirement changes its paging
  control from an explicit "Load more" button to bottom-reach infinite scroll
  (button retained as fallback); a new requirement guarantees the search/filter
  input stays responsive (fast typing never drops characters) and that filter
  state round-trips through the URL for reload, sharing, and back/forward.

## Impact

- **Code (frontend only):**
  - New: `web/src/lib/urlSynced.svelte.ts` (the primitive),
    `web/src/lib/components/InfiniteScroll.svelte` (the sentinel).
  - Rewritten: `web/src/lib/filters.svelte.ts` (`FilterStore` over the primitive).
  - Touched: `JobsView.svelte`, `CompaniesView.svelte`, `AnalyticsView.svelte`
    (consume the primitive; jobs/companies adopt `InfiniteScroll`),
    `LoadMore.svelte` (fallback role).
- **No backend / API / DB impact.** `Paginator` (`paginated.svelte.ts`) unchanged.
- **Verification:** no web test runner exists; `svelte-check` plus a per-screen
  manual checklist (fast typing, back/forward, URL sharing, scroll-to-load, load
  error → retry, keyboard load). Migration proceeds one screen at a time:
  companies → jobs → analytics.
