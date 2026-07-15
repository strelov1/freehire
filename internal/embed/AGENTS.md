# Semantic embedding conventions

## Scope
Incremental semantic embedding via the `semantic_outbox` queue, mirroring the enrichment pipeline; `reindex --semantic` remains the reconciler.

## Always true
- Work flows through `semantic_outbox` — a reference-only queue (`job_id` + `target_model` + lease/retry, not a copy of the job).
- Paired with `jobs.semantic_embedded_model`/`semantic_embedded_hash` provenance stamps (the "done" marker, like `enriched_at`/`enrichment_version`).
- `EnqueuePendingSemanticJobs` enqueues open jobs where `semantic_embedded_model IS DISTINCT FROM target_model OR semantic_embedded_hash IS DISTINCT FROM content_hash` (one predicate covers add + content-update + model-migration) and closed-but-still-embedded jobs (for removal), excluding non-tech via `enrich.NonTechCategories`.
- `target_model` is `search.CurrentEmbedderModel()`, the single source of truth shared with the CV-vector staleness guard.
- `ClaimSemanticBatch` does not filter `closed_at` out (the removal signal must reach the worker), returning a `closed` flag so the worker branches.
- Open → `IndexSemanticJobs` (embed `passage:` + in-place upsert with `_vectors`, built from the persisted row via `search.FromJob`, so enrichment facets survive) then stamp + delete the outbox row in one txn.
- Closed → `DeleteSemanticJobs` + clear the stamp + delete the row in one txn.
- The stamp records the exact embedded `content_hash` (nullable, passed through — a NULL stamps NULL so `IS DISTINCT FROM` doesn't re-enqueue forever).
- At-least-once like enrich (a crash between the Meili write and the PG txn re-embeds, idempotent by primary key).
- `reindex --semantic` is unchanged and remains the reconciler (full swap-rebuild: settings, at-scale model migration, compaction, and the authoritative closed-doc reconciliation).

## How it works
The semantic index (`jobs_semantic`, userProvided e5 vectors) can otherwise only be built by a full `reindex --semantic` — a swap-rebuild from zero that re-embeds the whole open catalogue and monopolizes Meilisearch's single task queue. `cmd/embed` fills the gap the same way `cmd/enrich` fills enrichment: work flows through `semantic_outbox` paired with provenance stamps. `ClaimSemanticBatch` is the enrichment claim with one deliberate divergence — it does not filter `closed_at` out, returning a `closed` flag so the worker branches between indexing open jobs and deleting closed ones. The runner lives in `internal/embed` behind `Store` + `Indexer` ports (unit-tested with fakes); `cmd/embed` wires the concrete adapters. A change concurrent with the embed re-enqueues next run instead of being marked current. Tuning via `EMBED_CONCURRENCY`/`EMBED_LEASE_SECONDS`/`EMBED_MAX_ATTEMPTS` (`config.LoadEmbed`); point `EMBED_URL` at a fast GPU endpoint for the one-time catalogue backfill, then steady-state drains only the day's new/changed/closed.

## Limitations
None currently listed.
