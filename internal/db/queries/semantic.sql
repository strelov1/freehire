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
--      tech-only" design — measured 2026-07-22 at only 35% of jobs_semantic's ~2.05M
--      docs carrying an is_tech tag, i.e. the same undifferentiated bulk the facet-index
--      and enrichment gates were tightened against. This enqueue change does not purge
--      the existing non-tech vectors already stamped in jobs_semantic — that needs a
--      one-time surgical Meili delete-batch (expensive: re-merges the whole index), not
--      a code change; this only stops the incremental gate from re-adding them.
--   2. UNINDEXABLE jobs that still carry an embed stamp (were embedded while open and
--      canonical) — a job now closed OR a non-canonical repost (duplicate_of set) — so
--      the worker removes their document from jobs_semantic and clears the stamp. This
--      mirrors the facet index: the full reindex --semantic also drops reposts (shared
--      splitJobs), so the incremental path must not re-add them.
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
-- non-canonical (duplicate_of) entry is the removal signal, so the worker must receive
-- it and branch on `closed` (true = remove the document).
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
-- Batch-load the persisted rows the embed worker builds documents from. A corrupted
-- row (SQLSTATE XX001) aborts the whole scan; the worker then retries the batch one id
-- at a time to isolate and dead-letter the bad row.
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

-- name: SetSemanticEmbedding :exec
-- Persist one job's semantic vector — the durable copy of what was just upserted into
-- the jobs_semantic index. Called once per embedded job inside the SAME transaction as
-- StampSemanticEmbeddedBatch on the open-job success path, so the stamp and the vector
-- commit together (a job is never marked embedded without its vector reaching Postgres).
-- Postgres thus becomes the source of truth for the vector: the nightly pg_dump backs it
-- up and reindex can rehydrate Meili from it without re-embedding. Idempotent by primary key.
UPDATE jobs
SET semantic_embedding = sqlc.arg(embedding)::real[]
WHERE id = sqlc.arg(id);

-- name: ClearSemanticEmbeddedBatch :exec
-- Clear a batch of jobs' embed provenance AND their durable vector after their documents
-- are removed from jobs_semantic (closed-job path). Run in the same transaction as
-- DeleteSemanticEntriesBatch. Dropping semantic_embedding keeps Postgres consistent with
-- the index: a closed job has no vector in either place.
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
-- DeleteJobSemanticChunks alone for a closed job), cmd/similar-backfill (reads, via
-- NearestJobsToJob), and GET /me/recommendations (reads, via NearestJobsToEmbedding).
-- ---------------------------------------------------------------------------

-- name: DeleteJobSemanticChunks :exec
-- Remove every chunk row for one job. Two callers: the open-job re-embed path issues
-- this immediately before InsertJobSemanticChunks in the same transaction (a job's
-- chunk COUNT can change between embeds — the source text was re-chunked — so there is
-- no stable per-chunk_index UPDATE target, "replace" has to be delete-then-insert, not
-- an upsert); the closed-job path issues this alone, mirroring the existing
-- ClearSemanticEmbeddedBatch clear. ON DELETE CASCADE from jobs already covers a hard
-- delete (cmd/prune) — this query is for the two soft paths cascade doesn't reach.
DELETE FROM job_semantic_chunks
WHERE job_id = sqlc.arg(job_id)::bigint;

-- name: InsertJobSemanticChunks :exec
-- Batch-insert one job's freshly-embedded chunks. chunk_indices and embeddings are
-- positionally paired parallel arrays (element i of one belongs with element i of the
-- other) — unnested separately and rejoined WITH ORDINALITY because sqlc cannot infer
-- the types of a multi-argument unnest over query parameters (same pattern as
-- pruning.sql's bulk job delete/archive). embeddings travels as vector literal TEXT
-- (e.g. "[0.1,0.2,...]"), not a native vector(768)[] array: pgx's driver.Valuer/
-- sql.Scanner fallback for pgvector.Vector (this repo registers no custom OID codec
-- for it) only covers a single scalar column value, not an array of them, so each
-- element casts to vector(768) individually in the SELECT instead. Always run
-- immediately after DeleteJobSemanticChunks in the same transaction as the embed
-- stamp — see that query's comment.
INSERT INTO job_semantic_chunks (job_id, chunk_index, embedding)
SELECT sqlc.arg(job_id)::bigint, idx.chunk_index, emb.embedding::vector(768)
FROM unnest(sqlc.arg(chunk_indices)::smallint[]) WITH ORDINALITY AS idx(chunk_index, n)
JOIN unnest(sqlc.arg(embeddings)::text[]) WITH ORDINALITY AS emb(embedding, n) USING (n);

-- name: NearestJobsToJob :many
-- The similar-jobs rollup for one source job (design.md Decision 5), consumed by
-- cmd/similar-backfill to populate jobs.similar_job_ids. A candidate job's distance to
-- the source is the MINIMUM cosine distance across every (source chunk, candidate
-- chunk) pair — the single nearest passage wins, not an average — so a long job with
-- one perfectly-matching paragraph outranks a job that is merely "somewhat close"
-- everywhere; this mirrors the chunking branch's own Meili-side scoring rule ("nearest
-- of a multi-vector document's vectors"), kept intentionally the same across the
-- storage-engine change. j1 (the source job) is joined once, not re-looked-up per c1
-- row via a correlated subquery, to read its company_slug for the exclusion below —
-- functionally identical to design.md's draft (which used
-- `company_slug IS DISTINCT FROM (SELECT company_slug FROM jobs WHERE id = c1.job_id)`)
-- but avoids re-executing a subquery per source-chunk row. Excludes: the source job
-- itself (c2.job_id <> c1.job_id), closed candidates, and — unless the source job has
-- no resolved company (company_slug = '', this repo's "unknown company" sentinel, see
-- jobs.company_slug NOT NULL DEFAULT '') — any candidate sharing the source's exact
-- company_slug, so two different companies that both merely lack a resolved slug don't
-- spuriously exclude each other.
SELECT j2.id AS job_id, MIN(c2.embedding <=> c1.embedding)::float8 AS distance
FROM job_semantic_chunks c1
JOIN jobs j1 ON j1.id = c1.job_id
JOIN job_semantic_chunks c2 ON c2.job_id <> c1.job_id
JOIN jobs j2 ON j2.id = c2.job_id AND j2.closed_at IS NULL
WHERE c1.job_id = sqlc.arg(job_id)::bigint
  AND (j1.company_slug = '' OR j2.company_slug IS DISTINCT FROM j1.company_slug)
GROUP BY j2.id
ORDER BY distance
LIMIT sqlc.arg(limit_count)::int;

-- name: NearestJobsToEmbedding :many
-- The same nearest-chunk-per-job rollup as NearestJobsToJob, but for GET
-- /me/recommendations: the query vector is a specific signed-in user's persisted CV
-- embedding (a résumé is short enough to embed as one passage, never chunked), not
-- another job's chunk set, so there is no self-join and no source company to exclude —
-- "similar to a job" and "matches a CV" are different questions, only the rollup shape
-- (minimum distance per candidate job across its chunks) is shared. Excludes only
-- closed jobs; callers layer the existing facet-filter WHERE clause and
-- LIMIT/OFFSET on top (see design.md Decision 5 / tasks.md 6.1).
SELECT c.job_id, MIN(c.embedding <=> sqlc.arg(query_vector)::vector(768))::float8 AS distance
FROM job_semantic_chunks c
JOIN jobs j ON j.id = c.job_id AND j.closed_at IS NULL
GROUP BY c.job_id
ORDER BY distance
LIMIT sqlc.arg(limit_count)::int;
