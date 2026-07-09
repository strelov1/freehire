-- name: EnqueuePendingSemanticJobs :execrows
-- Idempotent backfill for the incremental semantic-embedding queue. Enqueues two
-- kinds of outstanding work at the target embedder model:
--   1. OPEN jobs whose stored vector is missing, content-stale, or model-stale —
--      i.e. semantic_embedded_model differs from the target OR semantic_embedded_hash
--      differs from the job's current content_hash. Jobs whose derived category is in
--      exclude_categories (enrich.NonTechCategories) are skipped so embed budget stays
--      on technical roles; category is NOT NULL DEFAULT '', so an empty/unrecognized
--      category is never excluded (empty string <> ALL keeps the row).
--   2. CLOSED jobs that still carry an embed stamp (were embedded while open) — so the
--      worker removes their now-dead document from jobs_semantic and clears the stamp.
-- ON CONFLICT keeps exactly one entry per (job_id, target_model), so running this every
-- command invocation never duplicates work.
INSERT INTO semantic_outbox (job_id, target_model)
SELECT id, sqlc.arg(target_model)::text
FROM jobs
WHERE (
        closed_at IS NULL
        AND (semantic_embedded_model IS DISTINCT FROM sqlc.arg(target_model)::text
             OR semantic_embedded_hash IS DISTINCT FROM content_hash)
        AND category <> ALL(COALESCE(sqlc.arg(exclude_categories)::text[], '{}'))
      )
   OR (closed_at IS NOT NULL AND semantic_embedded_model IS NOT NULL)
ON CONFLICT (job_id, target_model) DO NOTHING;

-- name: ClaimSemanticBatch :many
-- Claim a batch of live, unleased entries, freshest job first, by stamping claimed_at.
-- Unlike ClaimEnrichmentBatch this does NOT filter closed jobs out: a closed entry is
-- the removal signal, so the worker must receive it and branch on `closed`. The jobs
-- join supplies both the freshness order and the closed flag. Freshness is
-- COALESCE(posted_at, created_at): jobs without a source post date fall back to ingest
-- time so they rank by recency instead of starving under NULLS LAST. FOR UPDATE OF o
-- locks only outbox rows (a bare FOR UPDATE would also lock jobs, making concurrent
-- claim waves contend); SKIP LOCKED lets concurrent workers take disjoint rows; the
-- lease predicate reclaims entries whose worker died (stale claimed_at), so no separate
-- reaper process is needed.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM semantic_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE semantic_outbox o
SET claimed_at = now()
-- Join jobs off the claimable CTE (not the UPDATE target o, which Postgres forbids in
-- FROM) so the removal branch gets the job's closed flag without a second query.
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE o.id = c.id
RETURNING o.id, o.job_id, (j.closed_at IS NOT NULL)::boolean AS closed;

-- name: StampSemanticEmbedded :exec
-- Record that a job's content is embedded under the given model. Run in the same
-- transaction as DeleteSemanticEntry on the success path, so a crash between the index
-- write and this stamp is safely retried (idempotent re-embed). hash is the job's
-- content_hash AS IT WAS EMBEDDED (passed through, nullable): stamping the exact
-- embedded hash — not the row's current one — keeps the staleness check honest, so a
-- content change concurrent with the embed re-enqueues on the next run instead of being
-- silently marked current, and a NULL content_hash stamps NULL (NULL IS DISTINCT FROM
-- NULL is false, so it does not re-enqueue forever).
UPDATE jobs
SET semantic_embedded_model = sqlc.arg(model)::text,
    semantic_embedded_hash  = sqlc.narg(hash)
WHERE id = sqlc.arg(id);

-- name: ClearSemanticEmbedded :exec
-- Clear a job's embed provenance after its document is removed from jobs_semantic
-- (closed-job path). Run in the same transaction as DeleteSemanticEntry.
UPDATE jobs
SET semantic_embedded_model = NULL,
    semantic_embedded_hash  = NULL
WHERE id = sqlc.arg(id);

-- name: DeleteSemanticEntry :exec
DELETE FROM semantic_outbox
WHERE id = $1;

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
