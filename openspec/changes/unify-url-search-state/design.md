## Context

Three frontend screens implement the same "input → debounce → URL → reload"
mechanism three different ways:

- **`FilterStore`** (`web/src/lib/filters.svelte.ts`), used by `JobsView` and
  `AnalyticsView`: mutates `value` synchronously but debounces the **URL write**
  via `goto()` (`#commitSoon`). Because the URL write is debounced and `goto`
  re-runs the route `load`, a navigation in flight lands its `load` result and
  `syncFromUrl()` reverts `value` to the lagging URL — dropping characters the
  user typed during the round-trip.
- **`CompaniesView`**: writes the URL **synchronously** via `replaceState` in the
  input handler and debounces only a local client `fetch`. No input race.
- **`AnalyticsView`**: reads filters from `FilterStore` (whose `goto` commit
  re-runs `load`) **and** debounces a second counts `fetch` in its own `$effect`
  — a double debounce and double load.

Constraint: there is no web test runner (`go test` does not cover `web/`).
Verification is `svelte-check` plus a manual per-screen checklist. The project is
MVP-stage with a fluid architecture, so reshaping `FilterStore` rather than
patching it is appropriate.

## Goals / Non-Goals

**Goals:**

- One reusable primitive for URL-synced search/filter state, used by all three
  screens, that eliminates the dropped-character bug **structurally** (not via a
  guard).
- Remove the duplicate debounce / double load in analytics.
- Replace "Load more" with bottom-reach infinite scroll, keeping an accessible
  fallback control.

**Non-Goals:**

- No backend, API, or DB changes.
- No change to `Paginator` (`paginated.svelte.ts`) — infinite scroll is only a
  new trigger for its existing `loadMore()`.
- No change to the set of facets, the search params, or SSR-first rendering.
- No pre-fetch-ahead tuning of the scroll trigger (explicitly "when you hit the
  bottom"); `rootMargin` stays `0px`.

## Decisions

### D1: Client-driven (`replaceState`) over load-driven (`goto`)

The root cause of the bug is **debouncing the URL write**. With `goto`, every URL
write is a navigation that re-runs `load`, so it must be debounced — which opens
the race window. Switching to `replaceState` lets the URL be written
**synchronously** on every keystroke (cheap, no navigation, no `load`), so the
URL is always in lockstep with the input and back/forward reconciliation can
never observe a lagging URL. Only the *data reload* is debounced.

This is the pattern `CompaniesView` already uses successfully; the change
generalizes it. JobsView is already half client-driven (it owns a client
`Paginator` for "load more" and a client `facetCounts` fetch), so dropping the
"reseed list from route `load`'s `initial` on every nav" effect in favor of a
client reload is a natural consolidation, not a new paradigm.

*Alternative considered — keep `goto` + a guard* (skip `syncFromUrl` while a
debounced commit is pending): works, but carries the race as a patch inside the
shared primitive and keeps the double-load shape. Rejected: the brief was to do
it right, not to encapsulate the workaround.

*Trade-off:* back/forward now reloads data **client-side** (via an effect) rather
than restoring a server-rendered `load` result. This matches `CompaniesView`
today and is acceptable — SEO/first-paint still come from SSR on direct load,
share, and refresh, which continue to read the URL through the route `load`.

### D2: `value` (live) + `applied` (debounced) as two `$state` fields

The primitive exposes two reactive fields:

- `value: $state<T>` — the live model bound to the input; mutators write it and
  **synchronously** `replaceState(serialize(value))`.
- `applied: $state<T>` — a debounced snapshot of `value` (300 ms), the sole
  signal consumers watch to reload.

Consumers reload via `$effect(() => { state.applied; reload() })`. A screen with
two data sources (jobs: list + counts) simply runs two effects. This keeps the
debounce in **one** place (the `value → applied` transition) instead of copied
into each component's effect, which is exactly what removes analytics' double
debounce.

`applied` cannot be a `$derived` of `value` (the debounce is asynchronous), so a
private `setTimeout` inside the mutator drives it. That timer is the only timer in
the system and is fully encapsulated.

### D3: Immediate vs debounced mutators (`setNow` / `setSoon`)

The primitive offers two mutators, preserving `FilterStore`'s current sensible
behavior:

- `setNow(next)` — discrete changes (facet pills, checkboxes, exclude toggle,
  clear): write URL **and** set `applied` immediately. A click is not typed
  fast, so debouncing it only adds latency.
- `setSoon(next)` — continuous input (free-text query, salary slider): write URL
  synchronously, set `applied` debounced.

`FilterStore` becomes a thin wrapper: `toggle/add/remove/setExclude/clear/
setMatchAll/setVisa/setSort → setNow`; `setQuery/setSalaryMin → setSoon`. Its
parse/serialize are the existing `filtersFromParams`/`filtersToParams`.
`CompaniesView` uses the primitive directly as `UrlSyncedState<string>`.

### D4: Infinite scroll as a separate trigger component

A new `InfiniteScroll.svelte` owns an `IntersectionObserver` on a sentinel `<div>`
at the list bottom (`rootMargin: '0px'`), calling an `onLoad` prop when the
sentinel enters the viewport; it disconnects the observer in the `$effect`
cleanup and is gated by an `enabled` prop (off while a load is in flight or none
remain). `Paginator` is untouched; the component just calls `loadMore()`. The
existing `LoadMore` button stays mounted as the fallback (spinner, retry,
keyboard/SR reach), so accessibility and error recovery are preserved.

*Alternative considered — drop the button entirely:* rejected for accessibility
(keyboard/SR users cannot scroll-trigger) and error recovery (a failed auto-load
needs a retry affordance).

## Risks / Trade-offs

- **No automated tests for the frontend** → mitigate with `svelte-check` and a
  fixed manual checklist per screen (fast typing, back/forward, URL share,
  scroll-to-load, load-error retry, keyboard load); migrate one screen at a time
  (companies → jobs → analytics) so a regression is isolated.
- **back/forward reloads client-side instead of restoring SSR `load`** (D1
  trade-off) → acceptable; SSR still serves direct/share/refresh. Verified by the
  back/forward checklist item.
- **`replaceState` does not create history entries** → intended (per-tweak
  history was never wanted); the URL still updates so share/reload/back across
  *real* navigations works. Matches current `replaceState`-keep behavior in
  `FilterStore`'s `#commit`.
- **Visible pause at the bottom** from `rootMargin: 0px` (no pre-fetch) → an
  explicit UX choice; a one-line `rootMargin` bump can add lead time later if it
  annoys.

## Migration Plan

Per-screen, smallest blast radius first, each verified before the next:

1. Land the primitive (`urlSynced.svelte.ts`) and `InfiniteScroll.svelte` with no
   consumers wired yet.
2. **Companies** — switch search to the primitive; adopt `InfiniteScroll`.
3. **Jobs** — rewrite `FilterStore` over the primitive; drop `#navTimer`/guard
   and the reseed-from-`initial` effect; adopt `InfiniteScroll`.
4. **Analytics** — consume the rewritten `FilterStore`; remove the duplicate
   debounce, reload counts off `applied`.

Rollback is per-screen (revert that screen's commit); the primitive is additive
until a screen adopts it.

## Open Questions

- None blocking. The fallback button's visibility (always shown vs only on
  error/keyboard-focus) can be settled during implementation against the existing
  `LoadMore` styling without a spec change.
