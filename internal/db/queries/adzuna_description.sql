-- name: EnqueueAdzunaDescriptionCapture :execrows
-- Transactional-outbox enqueue for the ingest write path: queue this one job for a full-
-- description fetch, gated on it not having been hydrated already.
--
-- The gate is a dedicated marker table (adzuna_description_hydrated) rather than a content
-- check on jobs.description, because once the queue entry is deleted after a successful
-- capture nothing else distinguishes "already hydrated" from "still carrying the API
-- snippet" — the row's own content_hash and updated_at move for unrelated reasons too.
--
-- Idempotent via the outbox's UNIQUE (job_id). Run in the same transaction as the job's
-- UpsertJob so a newly ingested job is queued atomically with its write. The caller is
-- responsible for only calling this for a job whose stored URL is Adzuna's own hosted
-- details page — the ad-network tracking redirect answers Access Denied and is never
-- queued (see adzunadesc.Eligible).
INSERT INTO adzuna_description_outbox (job_id)
SELECT j.id
FROM jobs j
WHERE j.id = sqlc.arg(job_id)::bigint
  AND NOT EXISTS (SELECT 1 FROM adzuna_description_hydrated h WHERE h.job_id = j.id)
ON CONFLICT (job_id) DO NOTHING;

-- name: ClaimAdzunaDescriptionBatch :many
-- Claim a batch of live, unleased captures, freshest posting first, by stamping
-- claimed_at. Mirrors ClaimApplyFormBatch: FOR UPDATE OF o locks only outbox rows, SKIP
-- LOCKED lets concurrent workers take disjoint rows, and the lease predicate reclaims
-- entries whose worker died, so no separate reaper process is needed.
--
-- The claim returns the job's external_id and url because the fetcher needs the posting
-- id and the exact stored URL to hit the same page the ingest write recorded.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM adzuna_description_outbox o
    JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE adzuna_description_outbox o
SET claimed_at = now()
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE o.id = c.id
RETURNING o.id, o.job_id, j.external_id, j.url, j.description;

-- name: MarkAdzunaDescriptionHydrated :exec
-- Record that this job's description is now the full text, closing the enqueue gate for
-- good. A re-hydration is a deliberate act (drop the row, which reopens the gate), not
-- something a crawl does by accident — mirrors apply_forms' own refresh story.
INSERT INTO adzuna_description_hydrated (job_id)
VALUES (sqlc.arg(job_id))
ON CONFLICT (job_id) DO UPDATE SET hydrated_at = now();

-- name: DeleteAdzunaDescriptionEntry :exec
-- Retire a capture that succeeded (or that will never succeed, e.g. the posting is gone).
DELETE FROM adzuna_description_outbox
WHERE id = sqlc.arg(id);

-- name: RecordAdzunaDescriptionFailure :one
-- Count a failed capture: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally left in
-- place — its expiry gates the retry to a later run. Mirrors RecordApplyFormFailure.
UPDATE adzuna_description_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
