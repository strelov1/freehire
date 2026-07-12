-- name: EnqueuePendingJobs :execrows
-- Idempotent backfill: enqueue every OPEN job that is unenriched or below the target
-- schema version. Closed jobs (closed_at IS NOT NULL) are skipped — a dead posting no
-- user will see should not consume LLM budget. Jobs whose derived category is in
-- exclude_categories (enrich.NonTechCategories) are skipped too, so LLM budget stays
-- on technical roles; category is NOT NULL DEFAULT '', so an empty/unrecognized
-- category is never excluded (empty string <> ALL keeps the row). ON CONFLICT keeps
-- exactly one entry per (job_id, target_version), so running this every command
-- invocation never duplicates work.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
  AND category <> ALL(COALESCE(sqlc.arg(exclude_categories)::text[], '{}'))
ON CONFLICT (job_id, target_version) DO NOTHING;

-- name: ClaimEnrichmentBatch :many
-- Claim a batch of live, unleased entries for OPEN jobs, freshest job first, by
-- stamping claimed_at. The jobs join lets the claim order by posting freshness and
-- skip closed jobs, so LLM budget goes to live postings users will actually see.
-- Freshness is COALESCE(posted_at, created_at): jobs without a source post date
-- (telegram/linksource and some ATS) fall back to ingest time, so they rank by
-- recency instead of starving behind every dated job under NULLS LAST. FOR UPDATE OF o
-- locks only outbox rows (a bare FOR UPDATE would also lock jobs, making concurrent
-- claim waves contend); SKIP LOCKED lets concurrent workers take disjoint rows; the
-- lease predicate reclaims entries whose worker died (stale claimed_at), so no
-- separate reaper process is needed.
WITH claimable AS (
    SELECT o.id
    FROM enrichment_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND j.closed_at IS NULL
      AND j.duplicate_of IS NULL
    ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE enrichment_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.job_id, o.target_version;

-- name: DeleteEnrichmentEntry :exec
DELETE FROM enrichment_outbox
WHERE id = $1;

-- name: RecordEnrichmentFailure :one
-- Count a failed attempt: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally
-- left in place — its expiry gates the retry to a later run and doubles as the
-- crash reaper, so a failed entry is never reprocessed within the same run.
UPDATE enrichment_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
