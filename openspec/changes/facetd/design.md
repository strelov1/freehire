## Context

`internal/search/search` is the only package that talks to Meilisearch; no Meilisearch type leaves
it. Callers pass filters as `[][]string` of Meilisearch filter fragments that our own code builds
(`FilterFromValues`, `In`/`NotIn`, `withExcludedIDs`), and an enumeration over every search param
found ~13 fragment shapes with no nesting. The raw calls are few: `facetSearch` (facet counts,
used by `FacetCounts`, region widening and the disjunctive path), `DistinctCount`, `Search`, and
the sitemap and company paths. Business logic sits above the raw calls — `withOpenJobs` adds
`NOT is_closed = true AND NOT is_duplicate = true`, `widenImpliedRegionCounts` re-counts implying
regions, `ParseQuery` turns `q` into federated alternatives.

Spike evidence (2026-11-08, memory `hire-own-search-engine-spike-2026-11-08`): on 11.2M real open
rows, facet bitmaps take 207 MB and ~3 GB with forward arrays for 29 facets; the `skills` facet is
5–35 ms, the company facet (698k values) 2.5 ms under a filter, all 26 page facets 93 ms over the
whole catalogue; writes run at thousands per second; a filter evaluator over our fragment shapes
matched Meilisearch v1.49 on 1,964/1,964 generated filters after two rules were copied.

## Goals / Non-Goals

**Goals:**

- Take facet-count, total and distinct-count traffic without `q` off Meilisearch.
- Identical answers, proven in shadow on production traffic before facetd answers anyone.
- A one-setting way back to Meilisearch, and a per-request fallback when facetd misbehaves.
- No new staleness: facetd is fed from the same drain batch as Meilisearch.

**Non-Goals:**

- Free text of any kind, and counts for requests carrying `q`.
- Result lists, ordering, pagination (a later change: ids from facetd, documents by id).
- Vectors, similar jobs, skill-match sort.
- Removing attributes or embedders from Meilisearch; sharding.

## Decisions

**A separate process, not a library inside hire-api.** hire-api runs blue and green colours side
by side; an in-process index would hold ~3 GB twice and rebuild on every release. `cmd/facetd` is a
long-lived service (like `cmd/mail-ingest`), one instance for both colours, reached over HTTP on
localhost. Alternative considered: embedding in hire-api — rejected for the RAM and the rebuild per
deploy.

**Route at the raw-call level.** The router replaces the Meilisearch call inside `facetSearch` and
`DistinctCount`, not the public `Client` methods, so region widening, disjunctive facets and
`maxValuesPerFacet` handling keep one implementation. The router receives the caller's filter
BEFORE `withOpenJobs`; facetd applies closed/duplicate/deleted as its own bitmaps. That keeps the
one compound fragment (`... AND ...` inside a single string) out of the parser.

**An engine-neutral raw answer.** Today `facetSearch` returns `*meilisearch.SearchResponse` and
`buildFacetResult` / `widenImpliedRegionCounts` read it. The seam becomes a small neutral type —
total, distribution (`attr → value → count`), stats (`attr → {min,max}`) — produced by either engine;
the Meilisearch adapter fills it from the response, facetd's client from its JSON. The value cap
(1200, highest counts first) and numeric stats are part of what facetd must reproduce, and the
parity test compares them.

**Index the flattened `JobDocument`, not Postgres rows.** facetd ingests exactly the JSON the drain
sends Meilisearch and flattens it as Meilisearch does. Parity then follows from one document shape;
a second projection from Postgres would be a second place for a facet to drift. Full rebuild is a
second implementation of `cmd/reindex`'s `rebuilder` interface (Prepare / Push / Promote / Cleanup)
over facetd's `/rebuild/*` endpoints: with `FACETD_URL` set, a reindex tees the same document
batches into Meilisearch and facetd, and `-facetd-only` rebuilds facetd alone (skipping description
vectors, which facetd does not need). A rebuild therefore takes as long as a reindex's Postgres
preparation (hours, not the minute the raw spike export took); it is the rare path — a restart reads
the snapshot, and the drain keeps it current.

**Copy the engine's rules, pinned by a parity test.** `!=` matches documents without the attribute
unless no document carries the attribute (then nothing); `NOT IN` matches documents without it; an
empty array or `null` is present-but-empty. An integration test indexes the same generated documents
into Meilisearch v1.49 (the version prod runs) and facetd and compares counts for every filter
`FilterFromValues` builds over the param vocabulary. An unknown fragment shape is an error, never
match-all.

**Two counting strategies chosen per request.** Per value, AND-cardinality of the value's bitmap
with the filter; or walk the filter's documents through a forward array (document → value ids) and
increment counters. The cheaper is picked from cardinalities (`Σ min(|v|,|F|)` against
`|F| × values-per-doc`). Measured: without the walk, a 140k-doc filter took 710 ms on the company
facet; with it, 2.5 ms.

**Copy-on-write snapshots.** A write batch builds the next index generation by cloning only the
bitmaps it touches, then swaps an atomic pointer; readers hold the generation they started with.
Measured with a single RWMutex instead: read p99 rose to ~280 ms under a write storm.

**Write path from search-drain.** The drain sends the same documents (and the same close /
duplicate / delete ids) to facetd right after Meilisearch accepts a wave — no second read of
Postgres on the normal path. When the facetd write fails (down, not ready, timeout), the wave's job
ids go into `facetd_outbox`, a reference-only retry queue in the house outbox pattern; a pass in the
same drain run, only while facetd reports ready, claims them, re-derives each job's CURRENT state
from Postgres (upsert, close, duplicate or delete — the same predicates the two Meilisearch passes
use) and sends one batch. Re-deriving instead of storing the failed payload makes the retry
idempotent and immune to ordering. The queue is keyed by job id, so it is bounded by the catalogue
even while facetd is down for days; its depth is a metric and an alert.

**Durability of acknowledged writes.** A snapshot every five minutes alone would lose up to five
minutes of acknowledged writes on a crash, and nothing would re-send them. facetd appends each
`/write` batch to a write-ahead log (fsync) before applying it, replays the log after loading the
snapshot, and truncates it after a successful snapshot. Rebuild-recorded batches ride the same log.

**Mode and fallback.** `SEARCH_FACET_ENGINE` sets the start mode; a Redis key overrides it at runtime
(the pattern `freehire:pause:search-drain` already uses). In `facetd` mode a request has its own
timeout (`FACETD_TIMEOUT`, default 150 ms); on error/timeout/unsupported/not-ready it is answered by
Meilisearch. Shadow evaluation runs detached with its own timeout and a concurrency cap so it can
never slow or fail a user request.

## Risks / Trade-offs

- [A semantic difference in a filter we did not generate in the test] → shadow on production traffic
  with a difference budget before `facetd` mode; every difference logged with its filter.
- [facetd misses writes and drifts] → staleness gauge (newest applied drain batch vs Meilisearch),
  the existing serve-probe idea extended to facetd, nightly rebuild from Postgres.
- [~3 GB RSS on an already loaded core] → forward arrays only for facets where the walk wins
  (high-cardinality and multi-value); measured heap otherwise ~1.3 GB. MemoryHigh in the unit.
- [Two engines to operate] → a restart is fast (snapshot + write-ahead log, no Postgres); a lost
  or unreadable snapshot means a full rebuild through `cmd/reindex`, which takes hours, and
  Meilisearch serves the traffic meanwhile; `meili` mode is the rollback.

## Migration Plan

1. Ship facetd and the drain dual-write with mode `meili`; run a full rebuild; watch staleness.
2. Switch to `shadow`; read differences for at least a week of traffic including a bot wave.
3. Switch to `facetd` for facet requests; watch fallbacks and latency.
4. Rollback at any point: set the mode to `meili` (Redis key, no deploy).

## Open Questions

- Snapshot format: roaring's portable serialization per bitmap plus a header, or a single
  memory-mapped file — decide when measuring load time.
- Whether `/jobs/facets` should request the two huge facets (`company_slug`, `cities`) at all, or keep
  them on-demand as the spike assumed.
