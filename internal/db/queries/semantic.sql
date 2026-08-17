-- name: EnqueuePendingSemanticJobs :execrows
-- Idempotent backfill for the incremental semantic-embedding queue. Enqueues two
-- kinds of outstanding work at the target embedder model:
--   1. OPEN jobs whose stored vector is missing, content-stale, or model-stale —
--      i.e. semantic_embedded_model differs from the target OR semantic_embedded_hash
--      differs from the job's current content_hash — AND confirmed technical
--      (is_tech IS TRUE), the same gate EnqueueJobEnrichment/EnqueuePendingJobs use
--      (jobs.sql, enrichment.sql) so embed spend is not wasted on postings that will
--      never surface via keyword/category search either (see search.CategoryUnresolved,
--      internal/search/document.go). Before this the gate was category-based
--      (category <> ALL(NonTechCategories)), a deliberate "category-gated, not
--      tech-only" design — measured 2026-07-22 at only 35% of the (now-removed)
--      jobs_semantic Meili index's ~2.05M docs carrying an is_tech tag, i.e. the same
--      undifferentiated bulk the facet-index and enrichment gates were tightened
--      against.
--   2. UNINDEXABLE jobs that still carry an embed stamp (were embedded while open and
--      canonical) — a job now closed OR a non-canonical repost (duplicate_of set) — so
--      the worker clears their stamp, legacy vector, and job_semantic_chunks rows
--      (Store.CompleteClosed; there is no search index left to remove a document from —
--      see openspec/changes/drop-hybrid-search-pgvector-similar).
-- ON CONFLICT keeps exactly one entry per (job_id, target_model), so running this every
-- command invocation never duplicates work. job_posted_at denormalizes
-- COALESCE(posted_at, created_at) onto the outbox row so ClaimSemanticBatch can sort by
-- it without joining jobs on every claim (see that query's doc comment).
INSERT INTO semantic_outbox (job_id, target_model, job_posted_at)
SELECT id, sqlc.arg(target_model)::text, COALESCE(posted_at, created_at)
FROM jobs
WHERE (
        closed_at IS NULL AND duplicate_of IS NULL
        AND (semantic_embedded_model IS DISTINCT FROM sqlc.arg(target_model)::text
             OR semantic_embedded_hash IS DISTINCT FROM content_hash)
        AND is_tech IS TRUE
      )
   OR ((closed_at IS NOT NULL OR duplicate_of IS NOT NULL) AND semantic_embedded_model IS NOT NULL)
ON CONFLICT (job_id, target_model) DO NOTHING;

-- name: ClaimSemanticBatch :many
-- Claim a batch of live, unleased entries, freshest job first, by stamping claimed_at.
-- Unlike ClaimEnrichmentBatch this does NOT filter unindexable jobs out: a closed OR
-- non-canonical (duplicate_of) entry is the clear-state signal, so the worker must
-- receive it and branch on `closed` (true = clear its embed state instead of embedding).
--
-- Orders by the outbox's OWN job_posted_at (denormalized at enqueue time from
-- COALESCE(jobs.posted_at, jobs.created_at) — see EnqueuePendingSemanticJobs) rather
-- than joining jobs to compute it. A join-for-ordering here means Postgres cannot push
-- the LIMIT below the sort — it has to nested-loop-join and sort the ENTIRE claimable
-- set before taking the batch, independent of batch_size. Measured live on prod at
-- ~906k claimable rows: 109s for a single claim call. See
-- openspec/changes/prod-semantic-embed-steady-state/design.md Decision 8. NULLS LAST
-- is defensive (every row written by EnqueuePendingSemanticJobs always populates the
-- column; this only guards a row that predates the backfill migration).
--
-- FOR UPDATE OF o locks only outbox rows (a bare FOR UPDATE would also lock jobs,
-- making concurrent claim waves contend); SKIP LOCKED lets concurrent workers take
-- disjoint rows; the lease predicate reclaims entries whose worker died (stale
-- claimed_at), so no separate reaper process is needed.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM semantic_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY o.job_posted_at DESC NULLS LAST, o.job_id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE semantic_outbox o
SET claimed_at = now()
-- Join jobs off the claimable CTE (not the UPDATE target o, which Postgres forbids in
-- FROM) so the removal branch gets the job's closed flag without a second query. This
-- join is over just the claimed batch (batch_size rows), not the whole claimable set —
-- cheap, unlike the ordering join this replaces above.
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE o.id = c.id
RETURNING o.id, o.job_id, (j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)::boolean AS closed;

-- name: GetJobsByIDs :many
-- Batch-load persisted rows by id. Two callers: the embed worker builds documents
-- from them (a corrupted row, SQLSTATE XX001, aborts the whole scan there; the
-- worker then retries the batch one id at a time to isolate and dead-letter the bad
-- row), and the /similar handler projects them to the public job wire shape.
SELECT *
FROM jobs
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: StampSemanticEmbeddedBatch :exec
-- Record that a batch of jobs' content is embedded under the given model. Run in the
-- same transaction as DeleteSemanticEntriesBatch on the success path, so a crash between
-- the index write and this stamp is safely retried (idempotent re-embed). The stamp
-- copies each job's CURRENT content_hash (nullable-safe: a NULL content_hash stamps NULL
-- so NULL IS DISTINCT FROM NULL stays false and the job is not re-enqueued forever).
-- Caveat: if an ingest commits a new content_hash in the tiny window between the embed
-- (which read the old content) and this stamp, the stamp records the NEW hash while the
-- vector reflects the old one — so the enqueue predicate sees a match and does NOT
-- re-enqueue it next run; that job carries a one-revision-stale vector until its content
-- changes AGAIN (which re-enqueues it). The window is one embed-duration and the drift
-- self-corrects on the next real change, so this is accepted over threading the exact
-- embedded hash through a nullable text[] per batch.
UPDATE jobs
SET semantic_embedded_model = sqlc.arg(model)::text,
    semantic_embedded_hash  = content_hash
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ClearSemanticEmbeddedBatch :exec
-- Clear a batch of jobs' embed provenance AND null the legacy jobs.semantic_embedding
-- column (closed-job path). Run in the same transaction as DeleteSemanticEntriesBatch
-- and DeleteJobSemanticChunks (see that query). Nothing writes semantic_embedding on
-- the open-job path anymore — job_semantic_chunks is the queryable representation now
-- (see openspec/changes/drop-hybrid-search-pgvector-similar) — but this still nulls it
-- on close: cheap, and it keeps a closed job's row free of a stale value from before
-- that write was removed, without needing a one-off backfill to clean up the column.
-- The column itself stays (dropping it is a separate, later change).
UPDATE jobs
SET semantic_embedded_model = NULL,
    semantic_embedded_hash  = NULL,
    semantic_embedding      = NULL
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: DeleteSemanticEntriesBatch :exec
DELETE FROM semantic_outbox
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: RecordSemanticFailure :one
-- Count a failed attempt: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally left
-- in place — its expiry gates the retry to a later run and doubles as the crash reaper,
-- so a failed entry is never reprocessed within the same run. Mirrors
-- RecordEnrichmentFailure.
UPDATE semantic_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;

-- ---------------------------------------------------------------------------
-- job_semantic_chunks: pgvector-backed per-chunk embeddings (see migration 0092
-- and openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decisions 1/5).
-- A job's description is HTML-stripped and split into one or more chunks, each with
-- its own vector(768) row here — replacing the single, doubly-truncated
-- jobs.semantic_embedding vector as the queryable representation. Consumers:
-- cmd/embed (writes, via DeleteJobSemanticChunks + InsertJobSemanticChunks or
-- DeleteJobSemanticChunks alone for a closed job) and cmd/similar-backfill (reads,
-- via NearestJobsToJob). GET /me/recommendations was removed rather than migrated
-- to pgvector (see drop-hybrid-search-pgvector-similar's Context: "Mid-implementation
-- reversal"); it never got a caller.
-- ---------------------------------------------------------------------------

-- name: DeleteJobSemanticChunks :exec
-- Remove every chunk row for a BATCH of jobs in one round trip — mirrors every other
-- batch mutation in this file (StampSemanticEmbeddedBatch, ClearSemanticEmbeddedBatch,
-- ClearSimilarComputedAtBatch, DeleteSemanticEntriesBatch), all one `WHERE id = ANY($1)`
-- call rather than one call per job: this pipeline was historically bottlenecked by
-- per-row Postgres round trips during a bulk backfill, not by GPU/TEI throughput, and
-- the upcoming full-catalogue re-embed (openspec/changes/
-- drop-hybrid-search-pgvector-similar, ~1.6-2M jobs) would reintroduce exactly that
-- regression at EMBED_BATCH_SIZE (default 500) deletes per transaction if this looped
-- per job instead. Two callers, both batched: the open-job re-embed path issues ONE
-- call for the whole wave's job ids immediately before the per-job
-- InsertJobSemanticChunks loop, in the same transaction (a job's chunk COUNT can change
-- between embeds — the source text was re-chunked — so there is no stable
-- per-chunk_index UPDATE target, "replace" has to be delete-then-insert, not an
-- upsert); the closed-job path issues ONE call for its whole batch alone, actually
-- mirroring ClearSemanticEmbeddedBatch's own batched round-trip shape, not just its
-- clear semantics. ON DELETE CASCADE from jobs already covers a hard delete
-- (cmd/prune) — this query is for the two soft paths cascade doesn't reach.
DELETE FROM job_semantic_chunks
WHERE job_id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: InsertJobSemanticChunks :exec
-- Batch-insert EVERY job's freshly-embedded chunks in the wave in one round trip, not
-- one call per job: job_ids/chunk_indices/embeddings are positionally paired parallel
-- arrays FLATTENED across the whole batch (one element per chunk, not per job — a job
-- contributing 3 chunks repeats its id 3 times at the matching offsets) — unnested
-- separately and rejoined WITH ORDINALITY because sqlc cannot infer the types of a
-- multi-argument unnest over query parameters (same pattern as pruning.sql's bulk job
-- delete/archive). embeddings travels as vector literal TEXT (e.g. "[0.1,0.2,...]"),
-- not a native vector(768)[] array: pgx's driver.Valuer/sql.Scanner fallback for
-- pgvector.Vector (this repo registers no custom OID codec for it) only covers a
-- single scalar column value, not an array of them, so each element casts to
-- vector(768) individually in the SELECT instead. Always run immediately after
-- DeleteJobSemanticChunks in the same transaction as the embed stamp — see that
-- query's comment.
--
-- One call for the whole wave, not one per job, for the same reason
-- DeleteJobSemanticChunks/StampSemanticEmbeddedBatch already are: a per-job round trip
-- here was exactly what made a prior HF GPU bulk-embed Postgres-bound instead of
-- GPU-bound (~19 docs/s against a GPU embedding a wave in ~7s) — see
-- hire-semantic-vectors-in-pg's "THE REAL BOTTLENECK IS POSTGRES, NOT THE GPU" note.
INSERT INTO job_semantic_chunks (job_id, chunk_index, embedding)
SELECT ids.job_id, idx.chunk_index, emb.embedding::vector(768)
FROM unnest(sqlc.arg(job_ids)::bigint[]) WITH ORDINALITY AS ids(job_id, n)
JOIN unnest(sqlc.arg(chunk_indices)::smallint[]) WITH ORDINALITY AS idx(chunk_index, n) USING (n)
JOIN unnest(sqlc.arg(embeddings)::text[]) WITH ORDINALITY AS emb(embedding, n) USING (n);

-- name: ClearSimilarComputedAtBatch :exec
-- Null a batch of jobs' precomputed-similarity staleness stamp. Run in the SAME
-- transaction as the open-job embed stamp / chunk replace (cmd/embed) — a job whose
-- chunks were just replaced has a stale (or absent) jobs.similar_job_ids, so this
-- clears similar_computed_at unconditionally for the whole open batch, letting
-- cmd/similar-backfill's incremental predicate ("similar_computed_at IS NULL, or a job
-- with no chunk rows at all is simply never selected") pick the job back up. Cheap and
-- idempotent — nulling an already-NULL column is a no-op write, not worth a
-- conditional guard.
UPDATE jobs
SET similar_computed_at = NULL
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: NearestJobsToJob :many
-- The similar-jobs rollup for one source job (design.md Decision 5), consumed by
-- cmd/similar-backfill to populate jobs.similar_job_ids. A candidate job's distance to
-- the source is the MINIMUM cosine distance across every (source chunk, candidate
-- chunk) pair — the single nearest passage wins, not an average — so a long job with
-- one perfectly-matching paragraph outranks a job that is merely "somewhat close"
-- everywhere; this mirrors the chunking branch's own Meili-side scoring rule ("nearest
-- of a multi-vector document's vectors"), kept intentionally the same across the
-- storage-engine change.
--
-- Rewritten from a plain GROUP BY over the full (source chunk × every other chunk)
-- cross join to a LATERAL join, one ANN probe per source chunk — measured live on prod
-- at 730k total chunk rows (a fraction of the eventual target): the cross-join form
-- took 152.7s for a SINGLE job (confirmed via EXPLAIN ANALYZE), because pgvector's
-- HNSW index only accelerates `ORDER BY embedding <=> const LIMIT k`, a shape the
-- cross-join's `MIN(...) ... GROUP BY` never produces — Postgres has no choice but to
-- compute every pairwise distance before it can take a minimum. Each source chunk's
-- LATERAL subquery is exactly that index-servable shape (see migration 0110's HNSW
-- index): it asks for the `over_fetch` nearest chunks to ONE source chunk, over-fetching
-- past `limit_count` to absorb whatever the closed/same-company filters below discard.
-- over_fetch too small under-fills the final top-N; too large gives back the cross-join
-- query's own cost — cmd/similar-backfill picks it as a multiple of limit_count.
--
-- The company/closed exclusion moves to the outer SELECT (against jobs, not repeated per
-- LATERAL probe) — unless the source job has no resolved company (company_slug = '',
-- this repo's "unknown company" sentinel, see jobs.company_slug NOT NULL DEFAULT ''),
-- any candidate sharing the source's exact company_slug is excluded, so two different
-- companies that both merely lack a resolved slug don't spuriously exclude each other.
WITH source_chunks AS (
    SELECT embedding
    FROM job_semantic_chunks
    WHERE job_id = sqlc.arg(job_id)::bigint
),
source_job AS (
    SELECT company_slug FROM jobs WHERE id = sqlc.arg(job_id)::bigint
),
nearest AS (
    SELECT n.job_id, n.distance
    FROM source_chunks sc
    CROSS JOIN LATERAL (
        SELECT c2.job_id, (c2.embedding <=> sc.embedding)::float8 AS distance
        FROM job_semantic_chunks c2
        WHERE c2.job_id <> sqlc.arg(job_id)::bigint
        ORDER BY c2.embedding <=> sc.embedding
        LIMIT sqlc.arg(over_fetch)::int
    ) n
)
SELECT j2.id AS job_id, MIN(nearest.distance)::float8 AS distance
FROM nearest
JOIN jobs j2 ON j2.id = nearest.job_id AND j2.closed_at IS NULL
CROSS JOIN source_job sj
WHERE (sj.company_slug = '' OR j2.company_slug IS DISTINCT FROM sj.company_slug)
GROUP BY j2.id
ORDER BY distance
LIMIT sqlc.arg(limit_count)::int;

-- name: GetJobSemanticGeneration :one
-- The source job's chunk-generation marker (design.md's NearestJobsToJob rollup has no
-- row to carry this on when a job's every candidate gets excluded, so it is its own
-- query, read in the same round trip as NearestJobsToJob rather than folded into it).
-- semantic_embedded_hash is stamped from content_hash by StampSemanticEmbeddedBatch in
-- the same transaction that writes a job's current job_semantic_chunks rows, so it
-- changes exactly when cmd/embed replaces those rows. cmd/similar-backfill passes the
-- value read here back to SetSimilarJobIDs as a conditional-write guard: if cmd/embed
-- re-embeds this job between this read and that write, the source's chunks (and the
-- NearestJobsToJob result computed from them) are already stale, and the guard drops
-- the write instead of stamping similar_computed_at over data the concurrent re-embed
-- already invalidated.
SELECT semantic_embedded_hash
FROM jobs
WHERE id = sqlc.arg(job_id)::bigint;

-- ---------------------------------------------------------------------------
-- cmd/similar-backfill: the run-once-and-exit worker that populates
-- jobs.similar_job_ids/similar_computed_at from NearestJobsToJob above. Finds
-- outstanding work by direct query, not a claimed/leased outbox table (design.md
-- Decision 4 — telagon's cmd/similar-backfill, the original inspiration, has no
-- outbox table either: the predicate below is idempotent and re-orderable, so a
-- batched full-table scan needs no lease/dead-letter machinery to stay safe under a
-- re-run or an overlapping manual invocation).
-- ---------------------------------------------------------------------------

-- name: SelectJobsNeedingSimilarBackfill :many
-- Jobs needing a (re)computed precomputed similar-jobs list: at least one
-- job_semantic_chunks row (so NearestJobsToJob has something to search from) but no
-- current list (similar_computed_at IS NULL). A plain IS NULL check, not "IS NULL OR
-- older than the newest chunk" — cmd/embed's CompleteOpen already nulls
-- similar_computed_at on EVERY chunk replace (ClearSimilarComputedAtBatch, called
-- unconditionally for the whole re-embedded batch, even when the new chunk count
-- happens to match the old one), so "missing" and "stale" already collapse to the same
-- NULL check; there is no case where a job's chunks changed but its
-- similar_computed_at stayed non-NULL. A job with zero chunk rows (never embedded, or
-- its description was too short/empty to chunk) is simply never selected — the EXISTS
-- clause already requires at least one row, no separate guard needed. A closed source
-- job is NOT excluded here (only closed CANDIDATES are, inside NearestJobsToJob) —
-- cmd/embed's own closed-job path deletes a job's chunk rows once its outbox entry
-- drains, which removes it from this predicate on its own; excluding closed_at here
-- too would just duplicate that cleanup with a narrower race window, not add safety.
-- Ordered freshest-job-first (mirrors ClaimSemanticBatch's ordering) so newly-posted
-- jobs get a similar-jobs list before older backlog on a still-draining catalogue.
SELECT j.id AS job_id
FROM jobs j
WHERE j.similar_computed_at IS NULL
  AND EXISTS (SELECT 1 FROM job_semantic_chunks c WHERE c.job_id = j.id)
ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: SetSimilarJobIDs :execrows
-- Write one job's precomputed similar-jobs list and stamp similar_computed_at
-- together, so a job is never marked computed without its list landing. A nil/empty
-- similar_job_ids is a valid, intentional write (a job whose only close matches were
-- excluded by NearestJobsToJob's same-company rule ends up with a short or empty
-- list, not an error) — it still stamps the job so
-- SelectJobsNeedingSimilarBackfill's incremental predicate does not pick it up again
-- every run. One row at a time, not batched like cmd/embed's writes: each job's array
-- value is unique per row, so there is no shared payload to amortize across a wave the
-- way a single Meili task amortizes cmd/embed's batch upsert.
--
-- The generation guard (IS NOT DISTINCT FROM, since a job with zero chunk rows would
-- never reach here but the comparison stays NULL-safe) makes this write a no-op — zero
-- rows affected, reported to the caller via :execrows — if cmd/embed replaced this
-- job's chunks (and cleared similar_computed_at itself, ClearSimilarComputedAtBatch)
-- after GetJobSemanticGeneration/NearestJobsToJob read it but before this write lands.
-- Without the guard this UPDATE would stamp similar_computed_at over a list computed
-- from chunks that no longer exist, and the job's now-current chunks would never be
-- backfilled until their NEXT content change.
UPDATE jobs
SET similar_job_ids = sqlc.arg(similar_job_ids)::bigint[],
    similar_computed_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND semantic_embedded_hash IS NOT DISTINCT FROM sqlc.narg(expected_generation);
