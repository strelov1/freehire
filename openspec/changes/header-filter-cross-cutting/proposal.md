## Why

The header Location & work-format quick-filter currently appears only on jobs-backed
lists. Location intent is just as strong when browsing companies or landing on any
other page, so the menu should be reachable across the app — while the text search
stays exactly as it is.

## What Changes

- Make the header filter menu **cross-cutting**, with three context-aware modes:
  - **Jobs lists** (`/`, `/companies/:slug`) — unchanged: work format + full location.
  - **Companies list** (`/companies`) — new: **Region** + **Remote hiring**
    (`remote_regions`) pills, live-filtering the company list. No work format /
    cities (companies don't carry those facets).
  - **Other pages** (job detail, `/about`, `/collections`, …) — new: the same trigger
    on the global launcher; picking a value **navigates to `/jobs`** with that scope,
    landing on the filtered feed where further tweaks are live.
- The trigger summary label extends to company geo (`remote_regions`) and to the
  empty launcher state.
- **No change to text search, no new backend, no new facet-count request.** Company
  and jobs modes drive their existing stores; launcher mode navigates.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `header-location-filter`: the quick-filter is no longer jobs-feed-only — it also
  filters the companies list (region + remote-hiring) and appears on listless pages
  as a launcher that scopes the jobs feed.

## Impact

- **Frontend only** (`web/`):
  - `HeaderLocationFilter.svelte` — generalize to a `variant` (`jobs` | `companies` |
    `launcher`) with per-mode sections; jobs mode unchanged.
  - `listSearch.svelte.ts` — `filterScope` gains a `variant` discriminator.
  - `CompaniesView.svelte` — register a `variant: 'companies'` filter scope.
  - `HeaderSearch.svelte` — render the trigger in `launcher` mode (navigates to
    `/jobs`).
  - `headerScope.ts` (+ test) — summary handles `remote_regions` and empty launcher.
- No backend/API/DB/search changes.
