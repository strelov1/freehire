## Context

CV-similarity ranking already exists end-to-end. `GET /api/v1/me/recommendations`
(handler `Recommendations`) ranks open jobs by the caller's persisted CV vector via
`search.RecommendByVector`, which issues one Meilisearch query with **Filter + Vector
together** (`SemanticRatio: 1`, empty query text): the facet filter constrains the
candidate set, the CV vector orders it. Facet params are translated by the same
shared `search.FilterFromValues` the keyword search endpoint uses, so the two
endpoints accept identical facet query strings and return the same `jobview` shape.

Today this ranking is only reachable from a dedicated `/my/recommendations` page
(`RecommendationsView.svelte`), which duplicates the `/jobs` filter sidebar. The main
feed (homepage `/`; `/jobs` 301s to it) is rendered by `JobsView.svelte`, whose
paginator always calls `api.searchJobs()` (`/jobs/search`, keyword-only,
`semantic_ratio=0`). The feed has no sort UI; `SortField` in `facetModel.ts` is the
single value `'posted_at'`, and `filtersFromParams` never reads `sort` back.

Full agreed design: `docs/superpowers/specs/2026-07-11-cv-sort-on-jobs-feed-design.md`.

## Goals / Non-Goals

**Goals:**
- Surface CV ranking as a sort mode ("Newest" / "Recommended") on the standalone feed.
- Reuse the existing recommendations endpoint unchanged; zero backend work.
- Keep facet filters working in CV mode; round-trip the sort in URL + localStorage.
- Graceful prompts for signed-out and no-CV users; never break the public feed.
- Remove the now-redundant `/my/recommendations` page and its nav entry.

**Non-Goals:**
- No backend, DB migration, or search-index change.
- No combining free-text `q` with CV ranking (the endpoint ignores query text).
- No CV sort on company-scoped embedded feeds.
- No per-row match-percentage badge — this is sort order only.

## Decisions

**1. `sort=cv` is a frontend routing signal, not a backend sort field.**
When the applied sort is `cv`, the paginator calls `api.recommendations()` instead of
`api.searchJobs()`. The `sort` param is stripped from the params before they reach any
API (the search endpoint's sort allowlist is `posted_at`/`created_at`/`salary_*`;
`cv` there would be ignored at best, misleading at worst). It stays only in the URL
and store for round-trip.
*Alternative considered:* add a `cv` sort mode to `/jobs/search`. Rejected — that
endpoint is unauthenticated and takes a query-text vector, not the caller's CV vector;
it has no access to the caller's identity. `RecommendByVector` is the clean path.

**2. SSR keeps serving the public "Newest" feed; CV mode is client-only.**
Homepage `+page.server.ts` continues to SSR page one via `searchJobs` (crawlable,
cacheable, no auth). When the feed starts in CV mode the SSR seed is newest-sorted and
therefore wrong, so the client forces a reload on the first effect run — reusing the
existing `initialStale` guard (which already reloads when the seed was fetched for a
different URL) extended to also fire when `filters.applied.sort === 'cv'`.
*Alternative considered:* SSR the recommendations feed when `sort=cv`. Rejected — the
endpoint is authenticated (cookie forwarding on SSR), personal (uncacheable), and the
current `RecommendationsView` is already client-only; matching that keeps SSR simple.

**3. Sort round-trips via the existing filter model.**
`filtersToParams` already emits `?sort=<v>` for a non-default sort, so no change there.
`filtersFromParams` learns to read `sort` back (accepting only known values, else the
default), which also makes the standalone list's `localStorage` restore CV mode. The
store's `setSort()` already exists — the UI just calls it.

**4. Empty-state disambiguation is lifted from `RecommendationsView`.**
An empty CV feed is ambiguous (no-CV vs no-match both return `[]`). The existing page
disambiguates on whether a facet filter is applied; that logic moves into the feed
before the page is deleted. Signed-out CV mode is gated client-side (no fetch) with a
sign-in prompt, since the endpoint would 401.

**5. Removing the nav link needs no `header-navigation` delta.**
The `header-navigation` "Consolidated menu" requirement never listed a Recommendations
item (the code drifted ahead of the spec). Deleting the link brings code back in line
with the spec, so it is implementation cleanup, not a spec change.

## Risks / Trade-offs

- **Newest-seed flash in CV mode** → a signed-in user landing on `?sort=cv` briefly
  sees the SSR newest feed before the client reload swaps in CV ranking. Acceptable and
  matches today's client-only recommendations behavior; the reload is immediate.
- **Persisted `sort=cv` for a now-ineligible user** → a returning signed-out / no-CV
  user whose stored filters include `sort=cv` sees the prompt instead of jobs. Handled
  by the always-visible prompt; not an error state.
- **Free-text `q` silently ignored in CV mode** → a user who typed a query and picked CV
  sort gets CV ranking without their text applied. Deferred by decision (Non-Goal); the
  facet filters they set still apply.
- **Deleting `RecommendationsView` orphans its empty-state logic** → mitigated by
  lifting that logic into the feed first, verified by the feed's no-CV / no-match tests.

## Migration Plan

Frontend-only; no deploy ordering, migration, or reindex. Ship the web change; the
`/my/recommendations` route 404s after deploy (its function moves to `/?sort=cv`).
Rollback is a plain revert — the backend endpoint is untouched throughout.

## Open Questions

None — free-text `q` + CV composition is explicitly deferred, not open.
