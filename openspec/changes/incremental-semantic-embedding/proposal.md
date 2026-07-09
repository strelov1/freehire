## Why

The semantic index (`jobs_semantic`) can only be built by a full `reindex --semantic`, which is a swap-rebuild from zero: it re-embeds the entire open catalogue every run, monopolizes Meilisearch's single task queue (deadlocking the facet reindex), and — on a shared-CPU embedder — takes days. Because of this, only a fresh ~647k-job window is embedded; the other ~2.4M open jobs never appear in `/similar` or CV recommendations, and a newly-ingested or content-changed job stays semantically invisible until the next full rebuild. There is no way to embed *only what is missing or changed*.

## What Changes

- Add a queue-driven, incremental semantic-embedding pipeline that mirrors the existing `cmd/enrich` + `enrichment_outbox` design.
- New `semantic_outbox` table and `jobs.semantic_embedded_model` / `jobs.semantic_embedded_hash` provenance stamps (new migration).
- New `cmd/embed` run-once-and-exit worker: enqueues open jobs that are un-embedded, content-changed (via `content_hash`), or embedded under a stale model; drains the queue in a concurrency wave; embeds each job (`passage:`) and upserts its vector **in place** into the live `jobs_semantic` (no swap), then stamps provenance and deletes the outbox row in one transaction.
- The worker also **removes** documents for jobs that closed after being embedded, so `/similar` and recommendations do not surface dead postings between full rebuilds.
- Non-tech categories are excluded from embedding from day one, reusing `enrich.NonTechCategories` (the same `exclude_categories` argument `cmd/enrich` already passes).
- `reindex --semantic` is unchanged and remains the reconciler (full swap-rebuild for settings changes, at-scale model migration, and compaction).

## Capabilities

### New Capabilities
- `semantic-embedding`: the incremental pipeline that keeps `jobs_semantic` current — a transactional outbox queue, per-job embed provenance, and a `cmd/embed` worker that adds/updates open jobs and removes closed ones in place, independent of the full `reindex --semantic` rebuild.

### Modified Capabilities
<!-- None. similar-jobs / cv-recommendations / job-search consume the semantic index; its coverage and freshness improve but their spec-level requirements do not change. -->

## Impact

- **Schema**: new migration adding `semantic_outbox` and two `jobs` provenance columns; sqlc regen (`internal/db`).
- **Queries**: new `internal/db/queries/semantic.sql` (enqueue/claim/stamp/clear/delete/record-failure), mirroring `enrichment.sql`.
- **New worker**: `cmd/embed` (Bootstrap + `worker.Main`, observability, `EMBED_URL`/`EMBED_API_KEY`/`EMBED_CONCURRENCY` + `MEILI_*` config).
- **Reuse (no change)**: `search.Client.IndexSemanticJobs`, `DeleteSemanticJobs`, `search.FromJob`, `internal/search/embed.go`, `internal/jobhash`, `enrich.NonTechCategories`.
- **Ops**: one more cron worker (like `cmd/enrich`); migration applied manually on prod before deploy (versioned-migration seam). No API or frontend change.
