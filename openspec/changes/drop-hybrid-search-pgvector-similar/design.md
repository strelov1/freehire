## Context

`jobs.semantic_embedding` (`real[]`, 768-dim e5-base vectors) is already the source of truth for job embeddings — `cmd/embed` writes it, and `jobs_semantic` (Meilisearch) is today just a queryable projection of it, rebuildable from it (`reindex --semantic --from-pg`). Two live consumers query that projection: `/jobs/:slug/similar` (job → nearest jobs) and `/me/recommendations` (CV vector + facet filters → ranked jobs). A prior attempt to also default general keyword search to a semantic blend was rejected (`jobs_semantic` is now `is_tech`-gated, a smaller/different set than the `jobs` keyword index, and the SPA call site fans out to ~10 unrelated pages).

Once general hybrid search is off the table, `/similar` turns out not to need a live index at all — "which jobs are similar to job X" is a fixed answer per job, computable once and cached, exactly like `../telagon`'s `cmd/similar-backfill` already does for "similar channels". `/recommendations` is different: its query vector is a specific signed-in user's CV, combined with facet filters chosen at request time — genuinely not precomputable, and stays a live query. Confirmed with the user this session (two direct decisions): the precompute path is a **separate worker** mirroring telagon, not folded into `cmd/embed`; and `/recommendations` **stays a live pgvector query**, the only place vector search remains request-time.

A local, non-prod spike this session validated the mechanics (pgvector installable on prod's self-managed PG18, HNSW build and cosine queries work) but hit real friction worth carrying forward as risk, not just a footnote — see Risks.

## Goals / Non-Goals

**Goals:**
- `/similar` becomes a plain indexed Postgres lookup — zero vector math on the request path.
- `/recommendations` keeps working, on pgvector instead of Meili's hybrid search — and gets *more* correct in the process (recall: `jobs_semantic` is `is_tech`-gated and only 1,021,471/1,608,951-ish docs vs. what `jobs.semantic_embedding` actually covers).
- `jobs_semantic`, its rebuild path, and `semantic_ratio` are fully removed — no dead code, no orphaned cron, no stale AGENTS.md claims.
- No outage window for either `/similar` or `/recommendations` during the migration.

**Non-Goals:**
- Changing how embeddings are *computed* (TEI, e5-base, `passage:`/`query:` prefixing) — unaffected.
- Reworking general keyword search — unaffected, already keyword-only in practice.
- Tuning `/similar`'s neighbour quality/count beyond matching today's behavior (`defaultSimilarLimit=6`, `maxSimilarLimit=20`).

## Decisions

**1. New `vector(768)` column, not an in-place type change.** `ALTER TABLE jobs ALTER COLUMN semantic_embedding TYPE vector(768) USING ...` forces a full-table rewrite under an `ACCESS EXCLUSIVE` lock — on a ~2M-row, write-heavy table (ingest writes constantly), that blocks all reads/writes for the rewrite's duration. Instead: add a new nullable `jobs.semantic_embedding_vec vector(768)` column (instant — Postgres's fast-path for adding a nullable column with no default), backfill it in batches via a one-off script (same shape as the existing `cmd/backfill-semantic-vectors` that originally seeded `real[]` from Meili), and have `cmd/embed` start writing both columns going forward during the transition (or just the new one once the backfill script has caught up — see Migration Plan). The old `real[]` column is dropped in a later, separate migration once nothing reads it — never in the same change that's still relying on it.

**2. HNSW index, built `CONCURRENTLY`, in a low-load window, with a generously raised `maintenance_work_mem`.** The spike found the default `maintenance_work_mem` (64MB, and even a naively-set 2GB) is not enough to hold the build graph past ~570K-600K tuples before it spills to disk and slows drastically — at prod's ~1.6-2M row scale, budget for either a large session-scoped `maintenance_work_mem` (4-8GB, matching how this repo already reasons about `work_mem`/`maintenance_work_mem` for other large operations) or accept a slower, disk-spilling build during an explicitly scheduled low-traffic window — same operational posture already used for Meili's own full rebuilds (`REINDEX_MIN_FREE_GB`, "never stack a rebuild with other heavy work"). `CREATE INDEX CONCURRENTLY` (pgvector supports this for HNSW) avoids locking `jobs` for the build's duration — non-negotiable given `jobs` is ingest's hottest table.

**3. `similar_job_ids bigint[]` + `similar_computed_at timestamptz` directly on `jobs`, not a separate table.** telagon's `similar_channels` table (with denormalized display columns) exists because telagon has no equivalent to this repo's `jobview` projection layer. Here, `/similar`'s handler already needs to fetch full job rows and project them to the public wire shape for its response — an array of IDs plus the existing job-fetch-and-project machinery is less new surface than a denormalized table that would need its own staleness handling (company renamed, title edited, job closed — all already handled correctly by reading the live `jobs` row). `similar_computed_at` mirrors the existing `semantic_embedded_model`/`semantic_embedded_hash` stamp pattern (`internal/embed/AGENTS.md`) — the same "provenance stamp marks done-ness" idiom this codebase already uses everywhere.

**4. `cmd/similar-backfill` finds work by direct query, no new outbox table.** Mirrors telagon's `GetChannelIDsWithoutEmbeddingSimilar` (a plain `WHERE semantic_embedding_vec IS NOT NULL AND similar_computed_at IS NULL` scan, batched, no queue table) rather than adding a third outbox (after `enrichment_outbox`, `semantic_outbox`) for what is fundamentally a "catch up whatever's missing" batch job with a natural idempotent completion check. Re-embedding (content change) already updates `semantic_embedding_vec` and should also null `similar_computed_at` so the backfill worker picks the job back up — one extra column write in `cmd/embed`'s existing stamp transaction, not a new pipeline.

**5. Closed jobs self-heal out of `similar_job_ids` at read time, no proactive fixup.** A job referenced in another job's `similar_job_ids` can close after that list was computed. Rather than eagerly scrubbing every list a closing job might appear in (expensive, unbounded fan-in), `/similar`'s read path fetches the referenced jobs and (like every other public job read) only returns still-open ones — a closed neighbour just quietly drops out of the response, same as the current Meili-backed behavior already does implicitly (the semantic index only ever held open jobs).

**6. Rollout order avoids any window where both `/similar` and `/recommendations` are broken.** See Migration Plan — the old Meili semantic path is only removed after both new read paths are verified live.

## Risks / Trade-offs

- **[Risk] The local spike's numbers are not trustworthy at face value.** The session's own spike hit a data-generation bug (accidentally duplicated vectors, invalidating early latency numbers), then repeated disk-I/O slowdowns on a contended laptop that never let a full 1.6-2M-row build complete cleanly. What *is* validated: pgvector installs and works correctly, HNSW build and cosine queries are mechanically correct, and query latency at a smaller, correctly-generated scale (100K rows) was low (single-digit ms). What is **not** independently re-validated at full prod scale: exact HNSW build wall-clock time and disk footprint at ~1.6-2M rows. → **Mitigation**: budget the first real build as an explicitly-scheduled, monitored operation (like any Meili full rebuild already is), not an assumed-cheap step; treat the build-time estimate as "tens of minutes, possibly longer if memory-constrained," not a promised number.
- **[Risk] `/similar` regresses to "no results" for jobs the backfill hasn't reached yet**, right after cutover. → Mitigation: keep the Meili-backed `/similar` path alive until the initial `cmd/similar-backfill` full pass has meaningfully caught up (see Migration Plan); the endpoint's existing "empty is a valid, silently-degrading response" contract (per `similar-jobs` spec) already covers residual gaps gracefully either way.
- **[Trade-off] `/recommendations` becomes MORE correct but also changes results** for any user whose recommendations previously excluded a job purely because `jobs_semantic`'s `is_tech` gate dropped it — pgvector reads `jobs.semantic_embedding_vec` directly, unaffected by that gate. This is a desired side effect of the fix, not a regression, but worth calling out as a visible behavior change, not a pure refactor.

## Migration Plan

1. Migration: `CREATE EXTENSION vector`; add `jobs.semantic_embedding_vec vector(768)`, `jobs.similar_job_ids bigint[]`, `jobs.similar_computed_at timestamptz` (all nullable, instant to add).
2. One-off batched backfill: `semantic_embedding_vec` from the existing `real[]` `semantic_embedding` (analogous to the original `cmd/backfill-semantic-vectors`). `cmd/embed` starts writing `semantic_embedding_vec` (and nulling `similar_computed_at` on change) going forward, in the same stamp transaction it already writes `semantic_embedding` in — no new transaction, no new queue.
3. Build the HNSW index `CONCURRENTLY` in a scheduled low-load window (see Decision 2).
4. Ship `cmd/similar-backfill`; run its initial full pass; verify coverage (e.g. `similar_computed_at IS NOT NULL` count approaching the embedded-job count).
5. Switch `/jobs/:slug/similar` to read `jobs.similar_job_ids` (old Meili-backed path removed only now).
6. Switch `/me/recommendations` to the live pgvector query (old Meili-backed path removed only now).
7. Remove: `jobs_semantic` index and its Meili-side code (`internal/search/client.go`'s semantic-index functions), `cmd/reindex --semantic`/`--from-pg`/`--posted-within`, the Meili-write half of `cmd/embed`'s `Indexer`, `semantic_ratio` from the handler and SPA, semantic-index-related cron/timers.
8. Drop the live `jobs_semantic` Meili index on prod (disk reclaim).
9. **Separate, later change**: drop the old `real[]` `semantic_embedding` column once nothing reads it (not bundled here — reduces blast radius of this already-large change).

## Open Questions

- Exact `maintenance_work_mem` value and scheduling window for the prod HNSW build — a real number needs a real (not laptop-contended) measurement or a conservative default with monitoring, decided during implementation rather than guessed here.
- Whether `cmd/similar-backfill`'s cron cadence should match `cmd/embed`'s or run less often (similarity lists don't need same-day freshness the way facets do) — default to daily unless implementation reveals a reason otherwise.
