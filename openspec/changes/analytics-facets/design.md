## Context

The catalogue (~390k jobs in prod) is fully faceted — every job carries
regions, countries, work mode, skills, plus an enrichment blob (category,
seniority, salary, company type, …) — and Meilisearch already indexes all of it
as filterable attributes for `/jobs/search`. Meilisearch also computes
single-dimension facet distributions (value → count) in the same query under the
same filter, but no endpoint exposes them. The frontend has a mature, URL-synced
filter store (`FilterStore`) and filter panel (`FiltersPanel`) driving `/jobs`.

This is Phase 1 of three (facets now; trends, cross-tabs later — see Non-Goals).

## Goals / Non-Goals

**Goals:**
- A public `GET /api/v1/jobs/facets` returning per-value counts for every facet
  (+ numeric min/max) under the same filters as `/jobs/search`.
- A public `/analytics` page with interactive, URL-synced drill-down, reusing the
  existing filter store and panel; breakdowns as plain-CSS bars.
- Near-zero backend cost: one Meili query per page state, no DB aggregation.

**Non-Goals:**
- Trends over time (Phase 2 — Postgres `date_trunc`).
- Cross-tabulation / two-dimension breakdowns (Phase 3).
- "Self-exclude" facet counting (a facet not narrowing itself) — deferred seam.
- Any schema change, new dependency, or charting library.

## Decisions

**1. Dedicated `FacetCounts` method, not an overloaded `Search`.**
`Search` returns ranked job hits; facet counting has different inputs (no sort,
no hybrid embedder, `limit: 0`) and outputs (distributions, not documents). New
`search.FacetCounts(ctx, FacetParams) (FacetResult, error)` with its own types
keeps both honest. *Alternative considered:* add `Facets` to `SearchParams`/
`SearchResult` — rejected, it leaves most fields meaningless for analytics calls
and mixes two responsibilities.

**2. Meili facet distribution, not Postgres aggregation.**
Meili computes the counts for free under the existing filter semantics (arrays,
enrichment dot-paths, AND/OR, exclude). Postgres would mean re-implementing the
whole filter vocabulary in SQL and keeping two implementations in sync.
*Trade-off:* Meili can't time-bucket or cross-tab — those become later,
Postgres-backed phases, which is the deliberate phasing.

**2b. Response keyed by public param, not index attribute.**
The handler re-keys Meili's distribution/stats from the index attribute
(`enrichment.seniority`) to the public query-param name (`seniority`) before
responding, so the endpoint speaks the same vocabulary clients filter with and
the index's internal dot-path structure never leaks into the public API. The
single param↔attr vocabulary lives in the handler (inverting `searchStringFacets`
plus the numeric/bool extras), so the frontend reads `facets[facet.param]`
directly. Continuous numeric facets are pruned from the distribution (kept only
as stats), since a bucket per distinct salary is noise.

**3. Simple self-include counting (one request).**
Every facet is counted under *all* active filters. The "correct" self-exclude
(N requests, each facet counted under all filters except its own) is deferred —
the simple version is a faithful "slice under current filters" and one cheap
query. Recorded as a seam.

**4. Reuse `buildSearchFilter` + the filter store; separate interfaces.**
Query-param → Meili-filter parsing is a shared *parsing* concern, so the new
handler reuses `buildSearchFilter`. But the handler depends on a small
`facetCounter` interface (just `FacetCounts`), separate from `searcher`, so the
two capabilities stay decoupled at the seam. The frontend reuses `FilterStore`/
`FiltersPanel` so drill-down and `/jobs` share one filter model.

**5. `maxValuesPerFacet: 300`, sort client-side.**
Meili's default of 100 truncates `skills`/`countries`. Raise it via
`indexSettings().Faceting`. Value-by-count sorting is done on the frontend, not
via `sortFacetValuesByCount`, to avoid the SDK over-serialization issue already
documented for `TypoTolerance` in `client.go`.

## Risks / Trade-offs

- **Stale index settings** → the live index keeps the old 100-value cap until
  reindexed. *Mitigation:* `make reindex` is a documented deploy step (tasks +
  proposal Impact).
- **Self-include can confuse drill-down** (a selected facet shows only its
  chosen value) → *Mitigation:* acceptable for Phase 1; self-exclude is a noted
  seam if UX demands it.
- **High facet count per request** (counting ~20 facets at once) → Meili handles
  this cheaply with `limit: 0`; if it ever shows latency, the page can request a
  curated subset of facets. No action now.
