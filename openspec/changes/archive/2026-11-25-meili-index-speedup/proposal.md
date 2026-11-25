## Why

The `jobs` facet index in Meilisearch is slow to update. A local benchmark against a
real ~317k-document sample of prod job data showed the "merging word proximity"
indexing phase alone costing up to ~10s per 200-document batch — the same shape of
work `cmd/search-drain` (internal/searchdrain) does continuously and `cmd/reindex`
does on a full rebuild. Meilisearch's own `progressTrace` confirms the cause: the
`wordPairProximityDocids` internal structure is larger than the documents themselves
and is rebuilt on every write. Meilisearch has shipped a setting specifically to skip
this cost — `proximityPrecision: byAttribute` (v1.6+) — and the freehire jobs index
(v1.49.0) supports it.

(Meilisearch also ships `prefixSearch: disabled`, v1.12+, covering the other half of
the same benchmark's cost. It is deliberately NOT applied here: `HeaderSearch.svelte`
and the `/jobs` list's `filters.ts` both debounce a query-as-you-type search through
this same index, relying on Meilisearch's default last-word prefix matching to return
results mid-word — this was caught in code review, see design.md.)

## What Changes

- Set `ProximityPrecision: "byAttribute"` on the jobs facet index settings
  (`facetSettings()` in `internal/search/client.go`).
- `proximityPrecision: byAttribute` drops the per-word-pair distance computation
  entirely (Meilisearch only checks whether query terms share an attribute), which
  eliminated the `merging word proximity` phase completely in the benchmark.
- **BREAKING (internal, not user-facing)**: existing indexed data was built under the
  old settings. A full `cmd/reindex` run is required after deploy for the new
  settings to take effect on the live index (the same requirement any
  `SearchableAttributes`/settings change already carries per the existing code
  comment).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — this is an internal indexing-performance change to `internal/search`, not a
change to job-search's request/response contract. Search relevancy may shift
marginally for queries where word-to-word distance within the same field mattered;
Meilisearch's own guidance is that this has little practical impact.)

## Impact

- `internal/search/client.go` — `facetSettings()`.
- `cmd/search-drain` (internal/searchdrain) — every incremental batch it pushes gets
  cheaper; this is the queue observed backlogged (~90k pending entries) in
  production.
- `cmd/reindex` — full facet rebuilds run faster (the combined-settings benchmark
  measured ~3x: 292s → 97s at ~317k documents; the batch-level `progressTrace`
  breakdown shows `proximityPrecision` alone accounts for most of that — see
  design.md — since `prefixSearch` is not being changed by this proposal).
- Deploy runbook: after this ships, a full `reindex` must run once to apply the new
  settings to the existing live index (see `internal/searchdrain/AGENTS.md` for how
  reindex and search-drain already coordinate via the timer stop/start hook).
- No schema, API, or migration changes.
