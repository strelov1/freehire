## Context

Job filters are stored in the URL query string as the single source of truth. `FilterStore` (jobs-specific, `web/src/lib/filters.ts`) wraps the generic `UrlSyncedState` primitive (`web/src/lib/urlSynced.svelte.ts`), which owns the state↔URL transport: explicit edits go through `setNow`/`setSoon` → `#write` → synchronous `replaceState`; browser back/forward re-seeds via `syncFromUrl` (wired by `syncOnNavigation`). The primitive was deliberately built around synchronous URL writes to fix a dropped-character race, so it must not be reshaped casually.

There are many entry points to `/jobs` (`TopBar`, `HeaderMenu`, `Footer`, home CTAs, error page, swipe exit). Each lands on a bare `/jobs` with an empty URL, which currently clears the user's filters. Making individual links filter-aware would scatter inconsistent behavior; the page itself is the natural single choke-point for restoration.

## Goals / Non-Goals

**Goals:**
- Filters on the standalone `/jobs` survive navigation to a bare `/jobs`, restored regardless of which entry point was used.
- Filters are cleared only by an explicit user change or reset — never lost to an ordinary navigation.
- Keep the URL as the source of truth; keep the `UrlSyncedState` primitive generic and its synchronous-write contract intact.

**Non-Goals:**
- Persistence for the company-embedded jobs list, analytics, or swipe (out of scope).
- Expiry / TTL for stored filters.
- Cross-tab synchronization (`storage` event).
- Server-side or account-level filter persistence (saved searches already cover intentional saves).

## Decisions

### Persist on `#write` only, via an optional `onWrite` callback on the primitive
`#write` is invoked by every explicit user mutation (`setNow`/`setSoon`) and by nothing else — `syncFromUrl` sets `value`/`applied` directly. That makes `#write` the exact "user changed the filters" signal. `UrlSyncedState` gains an optional `onWrite?(value)` callback fired inside `#write`; the primitive stays generic (it knows nothing about storage). `FilterStore` gains an opt-in `persist` flag that wires `onWrite` to save `filtersToParams(value).toString()` (empty string → remove the key).

- Why this over persisting on `applied` changes: `applied` also settles empty when navigation re-seeds to a bare URL, so it can't distinguish "user cleared" from "navigated to empty" — the whole crux. `#write` fires for the former only.
- Why store the serialized query string over a bespoke JSON shape: reuse the already unit-tested `filtersToParams`/`filtersFromParams`, so storage and URL share one codec with zero schema drift.

### The `/jobs` page is the single restore choke-point
Restoration happens in `JobsView` (standalone only), not per nav link. It is wired via **`afterNavigate`** (not a URL-tracking `$effect`): on a navigation that lands on a bare `/jobs` (empty `location.search`) with a non-empty stored set, `filters.apply(stored)` restores it; otherwise `filters.syncFromUrl()` runs (unchanged). For the company-embedded list the branch is gated off and the original `syncOnNavigation(filters)` is used, so it behaves exactly as today.

`apply(stored)` runs through `setNow` → `#write` → `replaceState`, rewriting the URL and re-persisting the same string (idempotent); shallow `replaceState` does not re-fire `afterNavigate`, so there is no loop.

Two constraints, both surfaced by end-to-end verification, shaped this:
- **`afterNavigate`, not `$effect`.** A restore `$effect` runs during hydration, *before* SvelteKit's client router is initialized, so `apply`'s `replaceState` throws "Cannot call replaceState before router is initialized". `afterNavigate` runs only after the router is ready and after the mount effects, so `replaceState` is safe and the restore's `applied` change lands after the reload effect's first pass (this also removes the effect-ordering fragility a `$effect` would carry).
- **Skip the initial `enter`.** Even in `afterNavigate`, the very first (cold) load — `nav.type === 'enter'` — is still too early for `replaceState`, and the SSR already rendered that exact URL. So restore is skipped on `enter` (the URL as loaded is served) and applies only on client-side navigations — which is every restore-worthy case: the "Jobs" nav link, cross-route entry, and back/forward. `location.search` (not `page.url.search`) is the address-bar truth, since `page.url` can lag after shallow routing.

### Scope gating via a `persist` flag
`FilterStore` persistence is opt-in; `JobsView` enables it only when `standalone` (no `scope`). The company-embedded list keeps `persist=false` so it never reads or writes the shared `hire.jobFilters` key.

### New `filterStorage.ts` module
A small, pure, SSR-guarded module (`loadJobFilters()`, `saveJobFilters(qs)`). It **feature-detects `localStorage` (`typeof`)** rather than importing `browser` from `$app/environment`, so it is importable by the plain-Node vitest env (which has no SvelteKit runtime) — that is what makes it the unit-testable core. Access is wrapped in try/catch and failures are swallowed, like `theme.svelte.ts`/`github.svelte.ts`. This keeps the storage concern out of the runes-bound files.

## Risks / Trade-offs

- **Clicking "Jobs" while on a filtered `/jobs` no longer clears the list** → intended per the approved design; "Clear all" is the explicit reset. Confirmed with the user.
- **Brief unfiltered flash on client-side entry to a bare `/jobs`** (the list shows the just-loaded unfiltered page for a beat, then the restore reloads it filtered) → acceptable and confined to client-side navigations; a cold load skips restore, so it never flashes there.
- **A cold hard-load / bookmark of a bare `/jobs` does not restore** → deliberate (the `enter` skip that avoids the router-not-ready error). It's the nice-to-have edge, not the user's scenario, and storage is preserved so the next client-side return restores.
- **Runes/`$app`-bound code (`urlSynced`, `JobsView`) is not unit-tested** → per project norms, verified via `svelte-check` + a Playwright end-to-end pass (persist, restore-on-nav, clear, URL-precedence, company isolation, and the cold-load no-error edge); the pure `filterStorage.ts` carries the automated coverage.
- **Frequent `localStorage` writes while typing** (one per keystroke via `setSoon`) → negligible; matches the existing per-keystroke URL write. Not debounced (YAGNI).
