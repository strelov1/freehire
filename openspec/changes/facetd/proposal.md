## Why

Meilisearch carries three jobs at once — free text, facet counts, and vectors — on one index of
7–11M documents that changes ~2,500 times a minute. Every change re-extracts the whole document,
writes go through one serial task queue, and a batch has a fixed cost of ~2 minutes, so the
engine is busy most of every hour; facet-heavy pages (the `skills` facet answers in 100–700 ms)
are what bot waves and SSR land on, and a reindex stalls the drain and with it the alerts. The
facet half of this load does not need a text engine. Spikes on 2026-11-08 (real data from the
replica) showed an in-memory bitmap engine answering the same facet counts in 1–35 ms, taking a
document change in microseconds, and matching Meilisearch v1.49 exactly on 1,964 filters our code
builds.

## What Changes

- New long-lived process `cmd/facetd`: an in-memory facet engine (roaring bitmaps per facet
  value, sorted numeric columns, a forward array per facet) over the same `JobDocument`s the
  `jobs` index holds. It answers facet counts, distinct counts and totals for requests **without
  free text**. Meilisearch keeps every request that carries `q`, and keeps the `jobs` index as it
  is today.
- New `Router` inside `internal/search/search`, at the level of the raw engine call (beneath
  `Client`'s business logic such as `widenImpliedRegionCounts` and disjunctive facets), with three
  modes switchable at runtime: `meili` (today), `shadow` (Meilisearch answers, facetd runs beside it
  and differences are logged and counted), `facetd` (facetd answers; an error, an unsupported filter
  or a timeout falls back to Meilisearch for that request).
- `search-drain` writes every document change, close, duplicate mark and delete to facetd as well
  as to Meilisearch, from the same batch, so a fallback never serves a staler answer than before.
- facetd boots from its last on-disk snapshot and is kept current by the drain; a full rebuild
  runs through `cmd/reindex` (the same documents it builds for Meilisearch, teed into facetd).
- Metrics: request counts and latency per engine and mode, fallbacks by reason, shadow
  differences, index size and staleness.
- Out of scope, later changes: filtered result LISTS from facetd (ordered ids then documents),
  vectors (similar jobs, skill-match sort), removing facets or embedders from Meilisearch,
  sharding across servers.

## Capabilities

### New Capabilities

- `facet-engine`: the facetd service — what it indexes, which requests it answers and which it
  refuses, its exact Meilisearch-compatible filter semantics, how it stays fresh, and how a caller
  is routed and falls back.

### Modified Capabilities

None. Every public answer stays the same; the change is which engine produces it.

## Impact

- Code: new `cmd/facetd`, new package `internal/search/facetidx` (index, filter evaluator, snapshot,
  HTTP API), router and facetd client in `internal/search/search` (`facetSearch`, `DistinctCount`
  and the disjunctive path), `cmd/search-drain` dual write, the layering table
  (`internal/platform/arch/layering/blocks.go`), `docs/agents/workers.md`.
- Dependency: `github.com/RoaringBitmap/roaring/v2` (Apache-2.0, pure Go).
- Hosts: facetd runs on core beside hire-api, ~2–3 GB RSS; a systemd unit and a snapshot directory
  in `freehire-ops`. One process for both blue and green colours.
- Configuration: `FACETD_URL`, `SEARCH_FACET_ENGINE` (`meili` | `shadow` | `facetd`, default
  `meili`), `FACETD_TIMEOUT`.
- Risk: a semantic difference from Meilisearch would show wrong counts; shadow mode with a
  difference budget gates the switch, and `meili` is one setting away.
