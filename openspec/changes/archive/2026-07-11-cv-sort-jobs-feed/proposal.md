## Why

CV-similarity ranking is hidden on a separate `/my/recommendations` page, so users
who browse the main feed never discover it. Folding it into the feed as a sort mode
("Newest" vs "Recommended") surfaces it where people already look and removes a
redundant page.

## What Changes

- Add a sort selector to the standalone jobs feed (homepage `/`): **Newest**
  (default, `posted_at`) or **Recommended** (`cv`).
- In CV mode the feed's paginator calls the existing `GET /api/v1/me/recommendations`
  instead of `GET /api/v1/jobs/search`; facet filters keep working (the filter
  narrows the set, the CV vector ranks it). Same facet params, same job wire shape.
- `sort=cv` round-trips in the URL and the standalone list's `localStorage`, but is a
  frontend routing signal only — it is stripped from the outgoing API request (never
  sent to the search endpoint, whose sort allowlist is `posted_at`/`created_at`/
  `salary_*`).
- The sort toggle is always visible; ineligible users get a prompt on use — "sign in"
  (signed-out) or "upload your CV" (signed-in, no usable CV vector).
- **BREAKING** (UI): remove the `/my/recommendations` page, `RecommendationsView.svelte`,
  and its nav entry. The backend endpoint stays — the feed now consumes it.
- Only the standalone feed gets CV sort; company-embedded feeds do not.
- Combining free-text search (`q`) with CV ranking is deferred (the endpoint ignores
  query text); in CV mode free text simply doesn't influence ranking.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cv-recommendations`: the CV feed moves from a dedicated `/my/recommendations` page
  to a sort mode on the main jobs feed. The page-presentation requirement is removed
  and replaced by a feed-integration requirement (sort selector, endpoint routing,
  eligibility prompts). The recommendations endpoint requirement is unchanged.

## Impact

- Web frontend only: `web/src/lib/facetModel.ts` (add `'cv'` to `SortField`, parse
  `sort` back from the URL), `web/src/lib/components/JobsView.svelte` (sort control,
  paginator routing, eligibility prompts, SSR-seed reload guard),
  `web/src/lib/components/HeaderMenu.svelte` (drop the nav link). Deletes
  `web/src/routes/my/recommendations/*` and `RecommendationsView.svelte`.
- No backend, database, or search-index changes. `GET /api/v1/me/recommendations`
  and `api.recommendations()` are reused as-is.
