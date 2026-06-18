## 1. Primitives (no consumers yet)

- [x] 1.1 Add `web/src/lib/urlSynced.svelte.ts`: `UrlSyncedState<T>` with `value`
  ($state, live), `applied` ($state, debounced snapshot), constructor
  `(params, { parse, serialize, debounceMs = 300 })`, `setNow(next)` (write URL +
  set `applied` immediately), `setSoon(next)` (write URL synchronously + debounce
  `applied`), `syncFromUrl()` (pull `value`+`applied` from URL, no debounce), and
  a `dispose()` clearing the timer. URL writes use `replaceState` with
  `keepFocus`/`noScroll`.
- [x] 1.2 Add `web/src/lib/components/InfiniteScroll.svelte`: props `{ onLoad,
  enabled }`; an `IntersectionObserver` (`rootMargin: '0px'`) on a sentinel
  `<div aria-hidden="true">` that calls `onLoad` on intersect; observer created
  and torn down in an `$effect` cleanup; no-op while `!enabled`.
- [x] 1.3 `svelte-check` is clean; both modules build with no consumers wired.

## 2. Companies screen (smallest blast radius)

- [x] 2.1 Rewrite `CompaniesView.svelte` search to use `UrlSyncedState<string>`
  (parse `p.get('q') ?? ''`, serialize `q ? {q} : {}`); reload the paginator via
  `$effect(() => { state.applied; reload() })`; remove the hand-rolled
  `timer`/`search()`/back-forward `$effect` now covered by the primitive.
- [x] 2.2 Replace the `LoadMore`-only paging with `InfiniteScroll` + `LoadMore`
  fallback (spinner while `loadingMore`, retry on `loadMoreError`,
  keyboard/SR-reachable).
- [x] 2.3 Verify: `svelte-check` clean; manual checklist on `/companies` — fast
  typing keeps every character, back/forward restores query + results, URL share
  works, scroll-to-bottom loads next page, simulated load error shows retry,
  keyboard activation loads next page.

## 3. Jobs screen

- [x] 3.1 Rewrite `FilterStore` (`filters.svelte.ts`) as a thin wrapper over
  `UrlSyncedState<JobFilters>` (parse `filtersFromParams`, serialize
  `filtersToParams`): `toggle/add/remove/setExclude/clear/setMatchAll/setVisa/
  setSort → setNow`; `setQuery/setSalaryMin → setSoon`. Remove `#navTimer`,
  `#commit`, `#commitSoon`, and the pending-guard `syncFromUrl`; keep the public
  method surface used by `JobsView`/`FiltersPanel` unchanged.
- [x] 3.2 Update `JobsView.svelte`: drive list + counts off `filters.applied`
  via `$effect`s; drop the reseed-from-`initial` effect (keep SSR `initial` only
  as the first seed); keep the monotonic `countsGen` stale-guard.
- [x] 3.3 Replace `LoadMore`-only paging with `InfiniteScroll` + `LoadMore`
  fallback in `JobsView.svelte`.
- [x] 3.4 Verify: `svelte-check` clean; manual checklist on `/jobs` — fast typing
  in the search box keeps every character, facet pills apply immediately,
  back/forward restores filters + input + results, URL share works,
  scroll-to-bottom loads next page, load error shows retry, keyboard load works.

## 4. Analytics screen

- [x] 4.1 Update `AnalyticsView.svelte` to consume the rewritten `FilterStore`:
  reload counts off `filters.applied` (single debounce in the primitive); remove
  the component-local debounce `setTimeout`; keep the `generation` stale-guard.
- [x] 4.2 Verify: `svelte-check` clean; manual checklist on the analytics screen —
  fast typing/filters do not double-load or drop characters, back/forward
  restores filters + breakdowns, URL share works.

## 5. Finish

- [x] 5.1 Full `svelte-check` + lint baseline check across `web/`; confirm no new
  errors versus the known-red baseline.
- [x] 5.2 Confirm `Paginator` (`paginated.svelte.ts`) and backend/API are
  untouched (diff review).
