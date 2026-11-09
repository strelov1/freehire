## 1. Package and placement

- [ ] 1.1 Add `github.com/RoaringBitmap/roaring/v2` to go.mod
- [ ] 1.2 Create `internal/search/facetidx` (block `search`) and `cmd/facetd`; add both to `internal/platform/arch/layering/blocks.go`; add an AGENTS.md for the package and an entry in `docs/agents/workers.md`

## 2. Index and filter evaluator

- [ ] 2.1 Document flattening: `JobDocument` JSON → field/value postings by dot path, arrays per element, bools, numbers into sorted columns, empty array/null as present-but-empty, `_vectors` and text fields skipped (unit tests per rule)
- [ ] 2.2 Index generation: dense doc ids, id → doc map, per-value bitmaps, numeric columns, closed/duplicate/deleted bitmaps applied to every answer
- [ ] 2.3 Filter evaluator for `nil`/string/`[]string`/`[][]string` and the fragment shapes (`=`, `!=`, ranges, `IN`, `NOT IN`, `EXISTS`, `NOT EXISTS`, `IS EMPTY`, `IS NOT EMPTY`, `NOT …`); unknown shape → `ErrUnsupportedFilter` (unit tests incl. both `!=` rules)
- [ ] 2.4 Facet counts with the per-request strategy choice (AND-cardinality vs forward-array walk), totals, distinct-value count; forward arrays only for facets where the walk can win
- [ ] 2.4a Distribution shaping: top 1200 values by count per facet (`maxValuesPerFacet`), min/max stats for numeric facets
- [ ] 2.5 Parity integration test: same generated documents into Meilisearch v1.49 (testcontainers) and facetd; every filter `FilterFromValues` builds over the param vocabulary plus random combinations; totals, distributions (with the 1200 cap) and numeric stats must be identical

## 3. Writes and snapshots

- [ ] 3.1 Write batch API: upsert documents (full replace), mark closed, mark duplicate, delete; idempotent
- [ ] 3.2 Copy-on-write generations: clone touched bitmaps, swap an atomic pointer; test that a reader during a 15k batch answers from the previous generation without blocking
- [ ] 3.3 Snapshot to disk and load on start; not-ready until loaded; corrupt/missing snapshot → rebuild
- [ ] 3.4 Full rebuild from Postgres reusing the `cmd/reindex` document builder, streamed into a fresh generation and swapped in

## 4. facetd service

- [ ] 4.1 `cmd/facetd`: HTTP API on localhost (`/facets`, `/distinct`, `/write`, `/rebuild`, `/ready`), graceful shutdown with a final snapshot
- [ ] 4.2 Prometheus metrics: request latency and count by endpoint, unsupported/refused by reason, documents, generation age, last applied drain batch
- [ ] 4.3 facetd HTTP client in `internal/search/search` with per-request timeout

## 5. Routing in the search client

- [ ] 5.0 Engine-neutral raw facet answer (total, distribution, stats) replacing `*meilisearch.SearchResponse` at the `facetSearch` seam; `buildFacetResult` and `widenImpliedRegionCounts` read it; behaviour unchanged (existing tests pass)
- [ ] 5.1 Router at the raw-call level in `facetSearch` and `DistinctCount`: receives the filter before `withOpenJobs`; requests with query text always go to Meilisearch
- [ ] 5.2 Modes `meili` / `shadow` / `facetd` from `SEARCH_FACET_ENGINE`, overridable at runtime through a Redis key; unit tests for each mode, fallback on error/timeout/unsupported/not-ready, and the mode flip
- [ ] 5.3 Shadow: detached evaluation with its own timeout and a concurrency cap; differences counted and logged with the filter; never affects the response
- [ ] 5.4 Metrics: requests by engine and mode, fallbacks by reason, shadow differences
- [ ] 5.5 Integration test through `FacetCounts` and the disjunctive path in `facetd` mode against the same fixture as 2.5 (region widening runs unchanged)

## 6. Drain dual write

- [ ] 6.1 `cmd/search-drain` sends each applied batch (upserts, closes, duplicate marks, deletes) to facetd after Meilisearch accepts it; facetd failure never fails or delays the Meilisearch path
- [ ] 6.2 Retry for facetd writes that failed, so facetd converges; test with facetd down then back

## 7. Rollout (freehire-ops and prod)

- [ ] 7.1 systemd unit for facetd on core (MemoryHigh, snapshot directory, restart policy), env `FACETD_URL`, `FACETD_TIMEOUT`, `SEARCH_FACET_ENGINE=meili`; recorded in freehire-ops
- [ ] 7.2 Deploy with mode `meili`, run a rebuild, confirm staleness gauge tracks the drain
- [ ] 7.3 Switch to `shadow`; collect at least a week of differences including a bot wave; fix every class of difference
- [ ] 7.4 Switch to `facetd`; watch fallbacks, latency and Meilisearch CPU; document the Redis rollback key in `internal/search/AGENTS.md`
