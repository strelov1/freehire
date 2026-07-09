## Context

`jobs_semantic` (userProvided/768-dim e5 `multilingual-e5-base`) is the hybrid/`/similar`/CV-recommendation index. Today its only writer is `cmd/reindex --semantic`, a swap-rebuild from zero: `Prepare` drops+recreates `jobs_semantic_rebuild`, `Push` streams every open job (embedding each batch via `internal/search/embed.go`), and `Promote` swaps atomically. This re-embeds the whole open catalogue every run, monopolizes Meilisearch's single task queue (deadlocking the facet reindex), and on the shared-CPU/CPU-TEI embedder takes days — so in practice only a fresh window is embedded and ~2.4M open jobs have no vector. `--since` is defeated because `UpsertJob` bumps `updated_at` every crawl, so its delta ≈ the whole table, and it never covers old un-embedded jobs.

Key existing pieces this change reuses unchanged:
- `search.Client.IndexSemanticJobs(ctx, docs)` — embeds a batch (`passage:`) and upserts documents with `_vectors` **in place** into live `jobs_semantic`. Already the `--since` in-place path.
- `search.Client.DeleteSemanticJobs(ctx, ids)` — removes docs from `jobs_semantic`.
- `search.FromJob(db.Job)` — builds the document from a persisted row (keeps enrichment facets).
- `internal/search/embed.go` — pluggable HTTP embedder (`EMBED_URL`/`EMBED_API_KEY`/`EMBED_CONCURRENCY`), retry/backoff, TEI `/v1/embeddings` batching.
- `jobs.content_hash` — maintained by `UpsertJob` for the incremental facet path; the change signal we key staleness on.
- `enrichment_outbox` + `internal/db/queries/enrichment.sql` + `cmd/enrich` — the exact pattern this mirrors.
- `enrich.NonTechCategories` — the non-tech exclusion list `cmd/enrich` already passes.

## Goals / Non-Goals

**Goals:**
- Embed only outstanding open jobs (missing, content-changed, or model-stale) — no re-embedding of already-current jobs.
- Keep `jobs_semantic` fresh going forward (new + changed jobs) via a cron worker, without a swap.
- Remove closed jobs from the index so `/similar` and recommendations don't surface dead postings between rebuilds.
- Exclude non-tech from day one at zero extra cost.
- Reuse the established enrichment-outbox mechanics verbatim (lease, SKIP LOCKED, retry, dead-letter, concurrency wave).

**Non-Goals:**
- Changing `reindex --semantic` — it stays the reconciler (settings, at-scale model migration, compaction, full closed-doc reconciliation).
- Embedding on the ingest hot path — embedding stays decoupled from `UpsertJob` (mirrors `SetJobEnrichment`).
- Improving embed quality/throughput (model choice, CV-summary embedding, truncation) — orthogonal, tracked elsewhere.
- A versioned-migration runner — out of scope; prod applies the migration manually before deploy (existing seam).

## Decisions

**1. Outbox queue + per-job provenance stamp (not a stateless Meili diff).**
`semantic_outbox` mirrors `enrichment_outbox` (id, job_id, `target_model text`, attempts, claimed_at, failed_at, last_error, created_at; `UNIQUE(job_id, target_model)`). `jobs` gains `semantic_embedded_model text` + `semantic_embedded_hash text` — the "done" marker, exactly as `enrichment_outbox` pairs with `jobs.enriched_at`/`enrichment_version`. Without the stamp, an idempotent backfill enqueue can't distinguish "already embedded" from "never embedded" and would re-enqueue the whole catalogue after each drain. *Alternative — diff open-job ids against a full `getDocuments` scan of `jobs_semantic`:* no schema change, but it detects only "missing" (not content-staleness without reading every doc's hash back), needs a full id-scan of Meili each run, and diverges from the repo's outbox convention. Rejected.

**2. `target_model` (text) is the staleness key, mirroring `users.resume_embedding_model`.** A model swap re-embeds the catalogue by making every `semantic_embedded_model` distinct from the new target. `content_hash` mismatch covers content drift. One enqueue predicate — `semantic_embedded_model IS DISTINCT FROM target OR semantic_embedded_hash IS DISTINCT FROM content_hash` — covers add + update + model-migration.

**3. New `cmd/embed` worker, not a `reindex` flag.** Queue-drain that is metered by a remote embedder and retryable is the direct sibling of `cmd/enrich`, which is its own worker rather than a reindex mode. `reindex` stays the "rebuild/swap" tool; `embed` is the "drain the semantic outbox" tool. Structure copies `cmd/enrich`: `worker.Main(run)` → `worker.Bootstrap` → require `MEILI_MASTER_KEY` + embedder config → `EnqueuePendingSemanticJobs(target_model, enrich.NonTechCategories)` → repeated claim waves sized to `EMBED_CONCURRENCY`, each wave drained concurrently.

**4. Closed-job removal via the same queue.** Enqueue also emits entries for closed-but-still-embedded jobs (`closed_at IS NOT NULL AND semantic_embedded_model IS NOT NULL`). `ClaimSemanticBatch` therefore must NOT filter `closed_at` out (unlike the enrichment claim). The worker branches per claimed job: open → embed + `IndexSemanticJobs` + `StampSemanticEmbedded` + delete entry (one txn); closed → `DeleteSemanticJobs` + `ClearSemanticEmbedded` + delete entry (one txn).

**5. Transaction boundary = the Meili write, then the PG stamp+delete.** The index upsert/delete is awaited first; on success a single PG txn stamps provenance and deletes the outbox row. A crash between the two re-runs the entry (embed is idempotent by primary key; delete is idempotent). This is the same at-least-once shape as `cmd/enrich`'s `SetJobEnrichment` + delete.

## Risks / Trade-offs

- **In-place upsert vs. swap monopolizing Meili's queue** → Small in-place `UpdateDocuments` batches interleave with facet reindex + incremental ingest pushes (unlike a full `--semantic` rebuild, which deadlocks them). This is a net improvement, but the worker still shares the single queue, so keep waves modest (batch sized like `cmd/enrich`).
- **At-least-once → a job may be re-embedded after a crash** → Idempotent by primary key; wasted work is bounded to one batch. Acceptable, matches enrichment.
- **Embed throughput is the wall (shared-CPU TEI ~3 docs/s)** → Unchanged by this design; point `cmd/embed` at a faster `EMBED_URL` (GPU endpoint) for the one-time ~2.4M backfill, then steady-state is tiny (~new/changed per day). Documented in the migration plan.
- **`target_model` string drift** → The worker's target model must equal the string stamped, or everything looks stale forever. Pin it to one source of truth (a `search` package constant / the embedder identity) rather than an ad-hoc literal.
- **Closed-job removal races a reopen** → A job closed then reopened between claim and process could be removed then re-enqueued next run; converges (reopened job is un-embedded → re-embedded). No stale-vector risk.
- **An open job reclassified non-tech after embedding keeps a stale vector** → The removal branch fires only on close, and the re-enqueue branch's category gate excludes it, so its vector lingers in `jobs_semantic` until it closes or a full `reindex --semantic` runs. Low probability (category derives from title, which rarely flips) and low impact (one stale hit); within scope — non-tech exclusion is a budget gate, not a purge.
- **No index perfectly backs the closed-inclusive freshness sort** → `jobs_open_enrich_freshness_idx` is partial (`WHERE closed_at IS NULL`), but `ClaimSemanticBatch` orders over closed jobs too. Fine while the outbox stays small (it drives the join); a large backlog (e.g. a full model-change re-embed) would sort unindexed. Seam, no action now.

## Migration Plan

1. Land migration (`semantic_outbox` + two `jobs` columns) — additive, no backfill; NULL stamps self-heal (every open job reads as "needs embedding" once). Apply on prod manually **before** deploying the new binaries (unapplied-migration-500 seam).
2. Deploy `cmd/embed` image + cron (sibling of `cmd/enrich`), with `MEILI_*` and `EMBED_*` env wired (freehire-ops compose).
3. One-time backfill: run `cmd/embed` pointed at a fast `EMBED_URL` (GPU endpoint) to drain the ~2.4M gap; **stop `freehire-reindexw.timer` during a large drain** only if it contends (small in-place batches generally coexist). Re-enable after.
4. Steady state: schedule `cmd/embed` on a cron; each run enqueues the day's new/changed jobs + any closed removals and drains them.
- **Rollback**: stop the `cmd/embed` cron. The migration is additive and inert without the worker; `jobs_semantic` reverts to being maintained solely by `reindex --semantic`.

## Open Questions

- Cron cadence and batch/concurrency defaults for steady state (tune against observed daily new/changed volume) — deferred to ops tuning; defaults mirror `cmd/enrich`.
