## 1. Sort model & URL round-trip (`web/src/lib/facetModel.ts`)

- [x] 1.1 Write failing unit tests (facetModel test file): `filtersFromParams` reads `sort=cv` back into `sort: 'cv'`; an unknown/absent `sort` falls to `DEFAULT_SORT` (`'posted_at'`); `filtersToParams` emits `sort=cv` for CV mode and omits `sort` for the default.
- [x] 1.2 Extend `SortField` to `'posted_at' | 'cv'`; make `filtersFromParams` parse `sort` accepting only known values (else default). Confirm `filtersToParams` already serializes it. Run tests green.
- [x] 1.3 Verify `canonicalQuery` and the persisted-filter path still round-trip `sort=cv` (add/adjust a test if needed).

## 2. Feed data routing (`web/src/lib/components/JobsView.svelte`, standalone only)

- [x] 2.1 In `scopedParams()`, strip the `sort` param from the built params when `filters.applied.sort === 'cv'` (routing signal must not reach the API).
- [x] 2.2 Branch `makePaginator()` on `filters.applied.sort`: `'cv'` → `api.recommendations(scopedParams(), limit, offset)`, else `api.searchJobs(...)`.
- [x] 2.3 Extend the first-run reload guard so the newest-sorted SSR seed is discarded when the feed starts in CV mode (reload when `initialStale` OR `filters.applied.sort === 'cv'`).
- [x] 2.4 `svelte-check` clean; visually confirm switching to CV mode routes to the recommendations endpoint and back.

## 3. Sort control (`web/src/lib/components/JobsView.svelte`)

- [x] 3.1 Add a compact sort selector on the count row, right-aligned, rendered only when `standalone`, with options "Newest" (`posted_at`) and "Recommended" (`cv`); wire it to `filters.setSort(...)` reading `filters.value.sort`.
- [x] 3.2 `svelte-check` clean; visual pass — control renders on the standalone feed, absent on the company-embedded feed, matches existing feed control styling.

## 4. Eligibility prompts (`web/src/lib/components/JobsView.svelte`)

- [x] 4.1 When `sort === 'cv'` and the user is signed out, show a sign-in prompt and skip the fetch (don't call the authenticated endpoint).
- [x] 4.2 When `sort === 'cv'`, signed in, and the feed is empty: show the "add/update your CV" prompt (link to `/my/profile`) when no facet filter is applied, and the ordinary "no matches" state when a filter is applied — lifting the disambiguation from `RecommendationsView.svelte`.
- [x] 4.3 `svelte-check` clean; visual pass on all three states (signed-out, signed-in-no-CV, filtered-empty).

## 5. Remove `/my/recommendations`

- [x] 5.1 Delete `web/src/routes/my/recommendations/+page.server.ts`, `web/src/routes/my/recommendations/+page.svelte`, and `web/src/lib/components/RecommendationsView.svelte`.
- [x] 5.2 Remove the `{ href: '/my/recommendations', label: 'Recommendations' }` entry from `web/src/lib/components/HeaderMenu.svelte`.
- [x] 5.3 Grep the repo for any remaining reference to `my/recommendations` or `RecommendationsView`; confirm only `api.recommendations()` (kept) and the OpenSpec change remain. `svelte-check` clean.

## 6. Verification

- [x] 6.1 Run the web unit tests (vitest) — facetModel round-trip tests pass.
- [x] 6.2 Run `svelte-check` and eslint locally (web CI does not gate them) — clean.
- [x] 6.3 Visual end-to-end: signed-in-with-CV user toggles Newest ↔ Recommended, applies a facet filter in CV mode and sees it narrow the ranked feed, reloads `?sort=cv` and it restores; signed-out and no-CV prompts render.
