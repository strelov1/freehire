## Why

Job filters live only in the URL query string. When a user navigates back to a bare `/jobs` — most commonly by clicking the "Jobs" nav link (`TopBar`, `HeaderMenu`, `Footer`, home-page CTAs all point at `/jobs`) — the URL is empty and their carefully built filter set is silently dropped. Users expect their filters to persist until *they* change or reset them, not to vanish on an ordinary navigation.

## What Changes

- Job filters are mirrored into `localStorage` (key `hire.jobFilters`) as the serialized filter query string, so the last explicit filter set survives navigation to a bare `/jobs`.
- The standalone `/jobs` page becomes the single restore choke-point: landing on `/jobs` with an empty URL restores filters from storage — on first mount and on same-route navigation to an empty URL.
- Persistence is written **only on explicit user changes** (facet toggles, text/slider input, applying a saved search, and "Clear all"), never on back/forward/navigation re-seeding. This is what makes "Clear all" wipe the stored filters while a navigation to an empty URL restores them.
- Scope is the standalone `/jobs` list only. The company-embedded jobs list (`/companies/:slug`) does not persist, to avoid clobbering the shared key.
- No expiry and no cross-tab syncing (out of scope — YAGNI).

## Capabilities

### New Capabilities
- `filter-persistence`: Remembering the standalone `/jobs` filter set in browser storage and restoring it when the user returns to a bare `/jobs`, cleared only by an explicit filter change or reset.

### Modified Capabilities
<!-- None: this adds a new restore behavior on top of the existing URL-as-source-of-truth filter model; it changes no existing spec's requirements. -->

## Impact

- `web/src/lib/filterStorage.ts` (new): pure, SSR-guarded load/save of the stored filter query string.
- `web/src/lib/urlSynced.svelte.ts`: `UrlSyncedState` gains an optional `onWrite` callback fired inside `#write` (explicit-change signal only; never on `syncFromUrl`).
- `web/src/lib/filters.ts`: `FilterStore` gains an opt-in `persist` flag wiring `onWrite` to storage.
- `web/src/lib/components/JobsView.svelte`: constructs the store with `persist` only when standalone; restores on empty-URL mount; replaces `syncOnNavigation` with a jobs-aware restore-or-sync effect.
- No backend, API, or database changes. No changes to the company/analytics/swipe filter stacks.
