## Context

`jobs.semantic_embedding` (`real[]`, 768-dim e5-base vectors) is already the source of truth for job embeddings — `cmd/embed` writes it, and `jobs_semantic` (Meilisearch) is today just a queryable projection of it, rebuildable from it (`reindex --semantic --from-pg`). Two live consumers query that projection: `/jobs/:slug/similar` (job → nearest jobs) and `/me/recommendations` (CV vector + facet filters → ranked jobs). A prior attempt to also default general keyword search to a semantic blend was rejected (`jobs_semantic` is now `is_tech`-gated, a smaller/different set than the `jobs` keyword index, and the SPA call site fans out to ~10 unrelated pages).

Once general hybrid search is off the table, `/similar` turns out not to need a live index at all — "which jobs are similar to job X" is a fixed answer per job, computable once and cached, exactly like `../telagon`'s `cmd/similar-backfill` already does for "similar channels". `/recommendations` is different: its query vector is a specific signed-in user's CV, combined with facet filters chosen at request time — genuinely not precomputable the same way. Confirmed with the user this session: the precompute path is a **separate worker** mirroring telagon, not folded into `cmd/embed`.

**Mid-implementation reversal on `/recommendations` (superseding the "stays a live pgvector query" plan below in Decisions 5/5a — see the removed decisions' history for why they existed):** investigating task 6 surfaced that no SQL facet-filter translator exists anywhere in this codebase (every filtered read goes through Meili's `search.FilterFromValues`), so migrating `/recommendations` meant either writing a large, correctness-risky new translator matching ~20+ facets' exact semantics, or a capped two-stage Meili-then-pgvector lookup. Weighing that real complexity against a feature of uncertain usage, the user chose (2026-08-14) to **remove `/me/recommendations` and its "Recommended" feed sort entirely, not migrate it** — see the `cv-recommendations` capability's spec delta (all three requirements REMOVED) and Migration Plan.

A local, non-prod spike this session validated the mechanics (pgvector installable on prod's self-managed PG18, HNSW build and cosine queries work) but hit real friction worth carrying forward as risk, not just a footnote — see Risks.

**Mid-implementation addition (superseding the original single-vector design below in Decision 1):** a sibling, unmerged branch (`worktree-semantic-embed-full-clean-chunked`, commit `134eac1a`) already diagnosed and designed a fix for a real, separate quality problem: today's embedding passage carries raw HTML and is truncated twice (once to `maxIndexedDescriptionRunes` for the facet index, again silently by TEI's token limit), so only the opening ~15-20% of an average description ever reaches the vector. That branch's fix — strip HTML to plain text, chunk the FULL text, embed every chunk — was held back only because it targeted Meilisearch's multi-vector document support and would have forced a second full re-embed mid-flight of a prod backfill that has since finished. Since this change is already re-architecting where vectors live and already needs to design a nearest-neighbour query from scratch, the user chose (2026-08-13) to fold the chunking fix in now rather than migrate to a single-vector-per-job pgvector schema and redo the migration again later once chunking eventually ships. The chunking/plaintext-extraction LOGIC itself (`chunkText`, `stripToPlainText` — see that branch's `internal/search/chunk.go`/`plaintext.go`) has no Meilisearch dependency and is reused verbatim; only the STORAGE shape changes (a chunks table instead of Meili's native multi-vector document).

**Also added mid-implementation:** the user asked that `/similar` exclude jobs from the same company as the source job (avoids a "similar jobs" list dominated by one employer's near-duplicate postings).

## Goals / Non-Goals

**Goals:**
- `/similar` becomes a plain indexed Postgres lookup — zero vector math on the request path.
- The full, HTML-free job description is represented in its embedding(s) — nothing past the first paragraph or two silently disappears, fixing a real quality problem, not just relocating the existing (truncated) vectors.
- `/similar` never recommends another posting from the same company as the source job.
- `jobs_semantic`, its rebuild path, `semantic_ratio`, and `/recommendations` (endpoint + feed sort + its CV-embedding write path) are fully removed — no dead code, no orphaned cron, no stale AGENTS.md claims.
- No outage window for `/similar` during the migration.

**Non-Goals:**
- Exact tokenizer-accurate chunk sizing — a conservative rune-count heuristic (ported from the chunking branch, see Decision 1a) is enough; TEI's own `--auto-truncate` is the safety net for a chunk that slightly overshoots.
- Reworking general keyword search — unaffected, already keyword-only in practice.
- Tuning `/similar`'s neighbour count beyond matching today's behavior (`defaultSimilarLimit=6`, `maxSimilarLimit=20`) — only the company-diversity rule and the richer source vectors change what's eligible, not how many are returned.
- Migrating `/recommendations` to pgvector, or preserving it in any form — removed, not carried forward (see Context).

## Decisions

**1. A `job_semantic_chunks` table (one row per chunk), not a single `vector(768)` column on `jobs`.** Once the source text is chunked (Decision 1a), a job has a *variable* number of vectors, not one — pgvector's `vector` type is fixed-arity per row, so "many vectors for one job" must be many rows, not a wider column (unlike Meili's native multi-vector `_vectors.default.embeddings` array, or the chunking branch's `real[]`-tolerates-either-shape trick, neither of which pgvector has an equivalent for). Schema:
```sql
CREATE TABLE job_semantic_chunks (
    job_id       bigint NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    chunk_index  smallint NOT NULL,
    embedding    vector(768) NOT NULL,
    PRIMARY KEY (job_id, chunk_index)
);
```
`ON DELETE CASCADE` matches this repo's actual hard-delete path (`cmd/prune` only) — a pruned job's chunks are pruned with it, no separate cleanup code needed. A closed-but-not-pruned job's chunks are explicitly deleted by `cmd/embed`'s closed-job path (mirrors today's `ClearSemanticEmbeddedBatch`). This *replaces* the originally-planned `jobs.semantic_embedding_vec vector(768)` column outright (that column, added by an earlier task in this same change, gets removed before it's ever used for real data — see Migration Plan) — `jobs.similar_job_ids`/`similar_computed_at` are unaffected by this revision, they describe the OUTPUT of the similarity computation, not the source vectors.

**1a. Reuse the chunking branch's plaintext/chunk logic verbatim; it has no Meilisearch dependency.** `stripToPlainText` (HTML → prose, preserving paragraph boundaries as newlines so chunk splits land between paragraphs) and `chunkText` (a ~2000-rune conservative budget per chunk, splitting on paragraph then word boundaries) are pure Go functions over strings — port them unchanged from `worktree-semantic-embed-full-clean-chunked`'s `internal/search/plaintext.go`/`chunk.go` into wherever they now belong given section 7 deletes most of `internal/search`'s semantic code (likely `internal/embed`, the package that survives and owns the embedding write path). Their existing unit tests port too.

**1b. No backfill script for the vector data — bump `embedderModel` and let the existing staleness pipeline re-embed everything.** The original Migration Plan (before chunking was folded in) called for a one-off Postgres-side conversion script (`real[]` → `vector(768)`, a cheap reshape of numbers already computed). That no longer applies: chunking changes the SOURCE TEXT fed to the embedder (HTML-stripped, full-length, split into pieces), not just the storage shape of the same numbers — there is no cheap in-Postgres reshape from the old single-vector `jobs.semantic_embedding` to the new chunked form, because the old vector was computed from different (truncated, HTML-laden) input text. Getting real chunked vectors requires calling TEI again for every job. This repo already has the right mechanism for "force everyone to re-embed": `semantic-embedding`'s existing staleness check re-enqueues any job whose `semantic_embedded_model` differs from `search.CurrentEmbedderModel()`. Bump that version string once (e.g. `intfloat/multilingual-e5-base` → `intfloat/multilingual-e5-base-chunked-v1`), and the normal `EnqueuePendingSemanticJobs` → `cmd/embed` drain re-embeds the whole catalogue through the new chunking pipeline with zero new one-off tooling. This is simpler than the original plan, not just different — a full task (the standalone backfill command from an earlier task in this same change) is deleted, not adapted.

**2. HNSW index on `job_semantic_chunks.embedding`, built `CONCURRENTLY`, in a low-load window, with a generously raised `maintenance_work_mem`.** Same reasoning as before this revision, but the row count is now larger than "one row per job" (a job with 3 chunks contributes 3 index entries) — budget accordingly, and treat the build-time estimate as even less certain than the original spike suggested (see Risks). The spike found the default `maintenance_work_mem` (64MB, and even a naively-set 2GB) is not enough to hold the build graph past ~570K-600K tuples before it spills to disk and slows drastically — budget a large session-scoped `maintenance_work_mem` (4-8GB) or accept a slower, disk-spilling build during an explicitly scheduled low-traffic window, same operational posture already used for Meili's own full rebuilds. `CREATE INDEX CONCURRENTLY` avoids locking `jobs`/`job_semantic_chunks` for the build's duration.

**3. `similar_job_ids bigint[]` + `similar_computed_at timestamptz` directly on `jobs`, not a separate table.** Unchanged by the chunking revision — see original reasoning: telagon's denormalized `similar_channels` table exists because telagon has no equivalent to this repo's `jobview` projection layer; an array of IDs plus the existing job-fetch-and-project machinery is less new surface, and `similar_computed_at` mirrors the existing provenance-stamp idiom.

**4. `cmd/similar-backfill` finds work by direct query, no new outbox table.** Unchanged in shape, updated in predicate: work is "a job with at least one row in `job_semantic_chunks` but `similar_computed_at IS NULL` (or older than its newest chunk)" rather than a single-column NULL check. Re-embedding (content change) already replaces a job's chunk rows and should also null `similar_computed_at` in the same transaction, so the backfill worker picks the job back up — one extra column write in `cmd/embed`'s existing stamp transaction, not a new pipeline.

**5. Nearest-neighbour-over-chunks, rolled up to one distance per candidate job, same-company jobs excluded.** A job's "similarity to job X" is the MINIMUM cosine distance across all (X's chunk, candidate's chunk) pairs — the nearest single passage wins, matching the chunking branch's own Meili-side scoring rule ("scores a multi-vector document by the nearest of its vectors") so the *ranking semantics* stay the same even though the storage engine changed. Shape:
```sql
SELECT j2.id, MIN(c2.embedding <=> c1.embedding) AS dist
FROM job_semantic_chunks c1
JOIN job_semantic_chunks c2 ON c2.job_id <> c1.job_id
JOIN jobs j2 ON j2.id = c2.job_id
    AND j2.closed_at IS NULL
    AND j2.company_slug IS DISTINCT FROM (SELECT company_slug FROM jobs WHERE id = c1.job_id)
WHERE c1.job_id = $1
GROUP BY j2.id
ORDER BY dist
LIMIT $2
```
(Exact predicate/index usage to be validated during implementation — this is a heavier query shape than a plain `ORDER BY embedding <=> $1`, a chunk-to-chunk cross join before the per-job rollup; see Risks. The same-company exclusion is a plain `company_slug` comparison, not a vector operation, and composes trivially with the rest.) This is now used only by `cmd/similar-backfill` — `/recommendations` (the other originally-planned consumer of this rollup shape) is removed, not migrated; see Context.

**5a. REMOVED — superseded by the decision to drop `/recommendations` entirely.** (Kept as a record: the original plan here was a two-stage Meili-facet-filter-then-pgvector-rank lookup, avoiding a new SQL facet translator. That design is moot once the endpoint itself is removed — see Context's "Mid-implementation reversal.")

**6. `/similar`'s rollout order avoids a window where it's broken; `/recommendations` is simply deleted, no rollout ordering needed for it.** See Migration Plan — the old Meili semantic path is only removed after `/similar`'s new read path is verified live. `/recommendations` has no "new path" to sequence against — its removal is a straightforward deletion (route, handler, SPA UI, CV-embedding write path), not a cutover.

## Risks / Trade-offs

- **[Risk] The local spike's numbers are now doubly unreliable for capacity planning.** They were already caveated (data-generation bug, contended laptop disk) for a *single-vector-per-job* index; the chunked design adds more rows than jobs (unknown multiplier — depends on the real description-length distribution, not measured this session) to the same index, so even the original "tens of minutes, possibly longer" estimate should be treated as a floor, not a ceiling. → **Mitigation**: unchanged in kind, stronger in degree — this is a monitored, scheduled prod operation (task 3.1/8.4), not an assumed-cheap step; get a real chunk-count-per-job distribution from a `SELECT count(*), avg(chunks_per_job)`-style query against a partial re-embed before committing to a maintenance window size.
- **[Risk] The full-catalogue re-embed (Decision 1b) is a real, possibly multi-hour-to-multi-day TEI cost, not a cheap toggle.** This repo's own `internal/embed/AGENTS.md`/memory already document that a full-catalogue embed at realistic TEI throughput took on the order of a day-plus historically. Bumping `embedderModel` re-triggers exactly that scale of work. → **Mitigation**: this is an explicit, scheduled prod operation (task 8.x), not bundled into a routine deploy; the existing incremental drain (freshest-first) means the site's *newest* postings get richer embeddings first, and `/similar` degrades gracefully (empty, not broken) for jobs not yet re-embedded, exactly as designed in Decision 4/Risk below.
- **[Risk] `/similar` regresses to "no results" for jobs the backfill hasn't reached yet**, right after cutover — now a larger window given the full re-embed is slower than the original "just reshape existing numbers" plan. → Mitigation: keep the Meili-backed `/similar` path alive until the initial `cmd/similar-backfill` full pass has meaningfully caught up (see Migration Plan); the endpoint's existing "empty is a valid, silently-degrading response" contract (per `similar-jobs` spec) already covers residual gaps gracefully either way.
- **[Risk] The chunk-join nearest-neighbour query (Decision 5) is a genuinely heavier query shape than the single-vector case the original spike measured.** Not yet benchmarked at any scale this session. → **Mitigation**: this query only ever runs inside `cmd/similar-backfill` (a background batch job, not the live `/similar` request path — Goal 1 is unchanged, the precomputed-lookup property holds regardless of how expensive computing it is), which is the query's only remaining caller now that `/recommendations` is removed rather than migrated. Still: measure it for real during implementation rather than assuming it scales fine.

## Migration Plan

1. Migration: `CREATE EXTENSION vector`; create `job_semantic_chunks` (Decision 1); add `jobs.similar_job_ids bigint[]`, `jobs.similar_computed_at timestamptz` (all nullable, instant/cheap to add). The already-implemented `jobs.semantic_embedding_vec vector(768)` column from before this revision is removed from the migration (edited in place — it has not been applied to any real, shared, persistent database yet, only replayed in ephemeral local/CI containers, so amending it is safe and avoids an add-then-immediately-drop column in the shipped migration history).
2. Port `stripToPlainText`/`chunkText` (Decision 1a). Rewrite `cmd/embed`'s open-job path to: chunk the plaintext description, embed each chunk, replace that job's rows in `job_semantic_chunks` (delete-then-insert, in the same transaction as the stamp), null `similar_computed_at`. Rewrite the closed-job path to delete the job's chunk rows.
3. Bump `embedderModel` (Decision 1b) to force the full-catalogue re-embed through the new pipeline — a scheduled prod operation, not bundled into a routine deploy (see Risks).
4. Build the HNSW index `CONCURRENTLY` on `job_semantic_chunks.embedding` in a scheduled low-load window (see Decision 2), once there's real chunk data to index against.
5. Ship `cmd/similar-backfill` (Decision 4/5, including the same-company exclusion); run its initial full pass; verify coverage.
6. Switch `/jobs/:slug/similar` to read `jobs.similar_job_ids` (old Meili-backed path removed only now).
7. Remove `/me/recommendations` outright: the route, `internal/handler/recommendations.go`, its résumé-upload CV-embedding write path, and the SPA's CV-sort UI (`JobsView.svelte`, `facetModel.ts`, `api.ts`'s `recommendations()`). No new path to cut over to — this is a deletion, not a migration.
8. Remove: `jobs_semantic` index and its Meili-side code (`internal/search/client.go`'s semantic-index functions), `cmd/reindex --semantic`/`--from-pg`/`--posted-within`, the Meili-write half of `cmd/embed`'s `Indexer`, `semantic_ratio` from the handler and SPA, semantic-index-related cron/timers.
9. Drop the live `jobs_semantic` Meili index on prod (disk reclaim).
10. **Separate, later change**: drop the old `real[]` `jobs.semantic_embedding` column once nothing reads it (not bundled here — reduces blast radius of this already-large change).

## Open Questions

- Exact `maintenance_work_mem` value and scheduling window for the prod HNSW build — now additionally uncertain in row count (chunks-per-job distribution unmeasured), needs a real number from real chunked data, not guessed here.
- Whether `cmd/similar-backfill`'s cron cadence should match `cmd/embed`'s or run less often (similarity lists don't need same-day freshness the way facets do) — default to daily unless implementation reveals a reason otherwise.
- Real wall-clock/cost estimate for the full-catalogue re-embed (Decision 1b/Migration step 3) — needs a measured rate from the new chunking pipeline (more TEI calls per job than before, since a job now contributes N chunk-embeds instead of 1), not assumed from the old single-vector throughput numbers this repo's memory already has on file.
