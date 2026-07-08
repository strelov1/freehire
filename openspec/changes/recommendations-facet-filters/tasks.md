## 1. Backend — filter the vector search

- [x] 1.1 Add a `filter any` argument to `search.RecommendByVector`, setting it as `Filter` on the semantic `SearchRequest` (nil preserves unfiltered behavior); update the integration test to cover a filtered vector search returning only matching jobs.
- [x] 1.2 Update the `searcher` interface's `RecommendByVector` signature in `internal/handler/search.go` and every fake/impl that satisfies it.
- [x] 1.3 Parse the request's facet params in `handler.Recommendations` via `buildSearchFilter(c)` and pass the filter to `RecommendByVector`; add a handler test asserting the facet filter reaches the search backend and an empty-on-no-match result.

## 2. Frontend — recommendations facet params

- [x] 2.1 Change `api.recommendations` to accept `facets: URLSearchParams` and merge them into the request query string alongside `limit`/`offset`.

## 3. Frontend — recommendations sidebar filter

- [x] 3.1 Rework `RecommendationsView.svelte` to add the shared sidebar: a URL-seeded `FilterStore` (persistence off), `FilterSummary` card + `FilterEdgeTab` + `FilterModal` (savedSearches off), facet counts from `api.facetCounts`, and a debounced-`applied` `$effect` that reloads the feed via `api.recommendations(filtersToParams(filters.applied), …)`.
- [x] 3.2 Disambiguate the empty states: unfiltered-empty → "add a CV" prompt (unchanged); filtered-empty → a non-error "no matching jobs" state.
- [x] 3.3 Widen `web/src/routes/my/recommendations/+page.svelte` to `max-w-6xl` and lay out the heading/intro above the sidebar+list row.

## 4. Verify

- [x] 4.1 `go build ./... && go vet ./... && go test ./...`; run the search integration test for the filtered-recommend path. (All exit 0; `TestIntegration_EmbedTextAndRecommend` filter sub-test green.)
- [x] 4.2 Frontend `svelte-check` clean on the touched files (no pure-logic to unit-test). Interactive visual pass of `/my/recommendations` DEFERRED — it needs the full authed stack (CV vector + semantic index); the sidebar reuses the already-visually-verified `/jobs` FilterModal/FilterSummary/FilterEdgeTab in the same layout, so verify on staging after deploy.
