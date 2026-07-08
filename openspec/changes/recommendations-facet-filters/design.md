## Context

`/my/recommendations` renders a client-only, auth-gated feed. `RecommendationsView.svelte` pages `api.recommendations(limit, offset)` → `GET /api/v1/me/recommendations` → `handler.Recommendations` → `search.RecommendByVector(vec, limit, offset)`, a pure vector search (`SemanticRatio: 1`) against the `jobs_semantic` index that ranks the whole open catalogue by CV similarity.

`/jobs` already has the full sidebar filter: a URL-driven `FilterStore`, a `FilterModal` (all facets), a `FilterSummary` card, a mobile `FilterEdgeTab`, and live facet counts from `GET /api/v1/jobs/facets`. Its handler turns request facet params into a Meilisearch filter via the shared, pure `search.FilterFromValues`.

Filtering a vector feed must happen server-side: the vector search ranks the entire catalogue and the page only pages a slice, so client-side filtering of a page would be wrong. The `jobs_semantic` index inherits `FilterableAttributes` from `facetSettings()` (`semanticSettings` builds on it), so a filter can be applied to the semantic search with no reindex.

## Goals / Non-Goals

**Goals:**
- Constrain the recommendations feed by the same facet vocabulary `/jobs` supports, applied before the CV ranking.
- Reuse the existing filter UI components unchanged (`FilterModal`, `FilterSummary`, `FilterEdgeTab`, `FilterStore`).
- Reuse the shared, pure filter builder (`search.FilterFromValues`) so recommendations and search cannot drift.

**Non-Goals:**
- No free-text query on recommendations (the CV is the query); the header search stays bound to `/jobs`.
- No swipe-mode entry from recommendations.
- No CV-scoped facet counts: the modal's counts come from the whole-catalogue `/jobs/facets` distribution (a pure-vector feed ranks the whole catalogue, so the facet distribution under the same filters is the natural, honest proxy).
- No database migration, no Meilisearch reindex.

## Decisions

**Backend — thread a filter through the vector search.**
- `search.RecommendByVector(ctx, vector, limit, offset)` → `RecommendByVector(ctx, vector, filter any, limit, offset)`, setting `Filter: filter` on the `meilisearch.SearchRequest`. A `nil` filter preserves today's unfiltered behavior.
- `handler.Recommendations` builds the filter with the existing `buildSearchFilter(c)` (the same helper `SearchJobs` uses) and passes it through.
- Update the `searcher` interface's `RecommendByVector` signature in `internal/handler/search.go` and the test fakes.

**Frontend — grow `RecommendationsView` its own sidebar, not reuse `JobsView`.**
`JobsView` is coupled to `searchJobs`, an SSR `initial` prop, header-search registration, swipe entry, and the shared `hire.jobFilters` localStorage key — none of which fit the client-only recommendations feed. Reusing it would mean a heavily-branched "mode" prop. Instead `RecommendationsView` renders its own thin sidebar reusing the *leaf* filter components:
- A `FilterStore` seeded from the URL with **persistence off** (so it never reads/writes the `/jobs` `hire.jobFilters` key — filters stay per-page). It writes the active filters to the URL synchronously; `syncOnNavigation` re-seeds on back/forward.
- The paginator source becomes `api.recommendations(filtersToParams(filters.applied), limit, offset)`; a debounced-`applied` `$effect` reloads the feed on filter change (same pattern as `JobsView`, minus the SSR-`initial` skip).
- Facet counts feed the modal from `api.facetCounts(params)` / `api.facetCounts(params, { disjunctive })` — no `scope` params.
- `FilterModal` is mounted with `savedSearches={false}` (saved searches belong to the search list).

**API — `recommendations` accepts facet params.**
`api.recommendations(facets: URLSearchParams, limit, offset)` merges the facet params into the query string alongside `limit`/`offset` (mirrors `searchJobs`). Callers with no filter pass an empty `URLSearchParams`.

**Empty-state disambiguation.**
The feed already returns an empty slice both for "no CV" and "no matches". The page can't tell them apart from the payload alone. Decision: treat an empty result as **no-CV** only when no facet filter is active; when filters are active, an empty result is the **no-matches** state. This is exact — a filtered empty feed is by construction a filter outcome, and the no-CV case is independent of filters (an unfiltered open first fetch reveals it).

**Layout.** `/my/recommendations/+page.svelte` widens from `max-w-3xl` to `max-w-6xl` to seat the sidebar, matching `/jobs`; the heading/intro move above the two-column row.

## Risks / Trade-offs

- **Counts are whole-catalogue, not CV-scoped.** A count of "120 remote jobs" describes the catalogue, not how many of *your* recommendations are remote. Accepted: CV-scoped counts aren't meaningful for a pure-vector feed (it ranks everything), and the count still correctly answers "does this facet have jobs". Documented in the view.
- **Deep pagination bound.** The endpoint keeps its `offset+limit > maxSearchWindow` guard; filters don't change that.
- **Two sidebars now exist** (`JobsView` and `RecommendationsView`). If a third feed needs the same shell, extract a shared `FilteredList` wrapper then — not pre-emptively (YAGNI); the leaf components are already shared, so the duplication is only the ~40-line aside/edge-tab wiring.
