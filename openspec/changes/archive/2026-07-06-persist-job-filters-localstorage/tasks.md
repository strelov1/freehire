## 1. Storage module (pure, unit-tested core)

- [x] 1.1 Add a failing vitest for `web/src/lib/filterStorage.ts`: `saveJobFilters` writes a non-empty query string to `hire.jobFilters`; `loadJobFilters` round-trips it; `saveJobFilters('')` removes the key; SSR/no-storage and throwing storage are swallowed (return `''` / no throw).
- [x] 1.2 Implement `filterStorage.ts` (`loadJobFilters()`, `saveJobFilters(qs)`) self-contained (feature-detects `localStorage`, so the pure-Node vitest resolves it) with try/catch swallowing failures; make the test pass.

## 2. Primitive: explicit-change hook

- [x] 2.1 Add an optional `onWrite?: (value: T, serialized: string) => void` to `UrlSyncedState` and invoke it inside `#write` (only there — not in `syncFromUrl`), passing the already-serialized query string. Keep the primitive storage-agnostic.

## 3. FilterStore opt-in persistence

- [x] 3.1 Give `FilterStore` an opt-in `persist` flag; when set, wire `onWrite` to `saveJobFilters(serialized)` (the URL's own query string) so explicit changes (incl. `clear`/`apply`) mirror to storage and clear removes the key.

## 4. JobsView restore choke-point (standalone only)

- [x] 4.1 Construct the store with `persist` enabled only when standalone (no `scope`); the initial `standalone` is captured via `untrack`.
- [x] 4.2 Restore on empty-URL landing (client-only) — folded into the 4.3 `afterNavigate` handler (fires on every real navigation including cross-route entry), so no separate `onMount` is needed.
- [x] 4.3 For the standalone list, wire restore via `afterNavigate` (not a `$effect` — a hydration-time effect throws "replaceState before router initialized"): skip the initial `enter`; on empty `location.search` + stored → `filters.apply(stored)`, else `filters.syncFromUrl()`. Company-embedded list keeps the original `syncOnNavigation(filters)` — unchanged.

## 5. Verification

- [x] 5.1 Run the vitest suite (`filterStorage` green — 38/38) and `svelte-check` (0 errors, 0 warnings). oxlint clean; the 4 eslint errors are pre-existing `origin/main` constructs, not introduced here.
- [x] 5.2 Playwright end-to-end (dev server against prod API) — 10/10: explicit change writes `hire.jobFilters`; clicking "Jobs" nav restores it (URL rewritten); "Clear all" removes the key and stays unfiltered on return; a shared `/jobs?q=…` wins and leaves storage untouched; company page typing writes nothing; cold hard-load of a bare `/jobs` restores nothing and throws no router error.
