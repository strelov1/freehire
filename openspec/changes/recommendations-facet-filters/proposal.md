## Why

The `/my/recommendations` feed ranks the whole open catalogue by CV similarity but offers no way to narrow it — a user who only wants remote, or senior, or backend roles has to scroll a full ranked list. The `/jobs` list already has a rich sidebar facet filter; recommendations should offer the same narrowing, applied *before* the ranking.

## What Changes

- Add facet filtering to the `GET /api/v1/me/recommendations` endpoint: the same facet query params the search endpoint accepts (regions, work_mode, seniority, category, skills, salary, freshness, …) constrain the candidate set, and the CV vector ranks what survives the filter.
- Thread an optional Meilisearch filter through `search.RecommendByVector`; the `jobs_semantic` index already carries the shared filterable attributes, so no reindex is needed.
- Give `/my/recommendations` a sidebar filter that reuses the existing `FilterModal`/`FilterSummary`/`FilterEdgeTab` machinery (identical to `/jobs`), driving the recommendations endpoint instead of search.
- Keep the feed's existing empty states, distinguishing "no CV uploaded" (prompt to add a CV) from "no jobs match the current filters".

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cv-recommendations`: the recommendations endpoint additionally accepts facet filter params and constrains the ranked candidate set to them; the recommendations page gains a sidebar facet filter and a filters-empty state.

## Impact

- Backend: `internal/search/client.go` (`RecommendByVector` gains a filter arg), `internal/handler/recommendations.go` (parse + pass the facet filter), the `searcher` interface in `internal/handler/search.go`.
- Frontend: `web/src/lib/api.ts` (`recommendations` accepts facet params), `web/src/lib/components/RecommendationsView.svelte` (sidebar + filter wiring), `web/src/routes/my/recommendations/+page.svelte` (widen layout for the sidebar).
- No database migration and no Meilisearch reindex (the semantic index is already filterable).
