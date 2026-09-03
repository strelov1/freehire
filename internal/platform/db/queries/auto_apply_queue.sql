-- name: ClaimAutoApplyBatch :many
-- Claim a batch of live, unleased submission attempts by stamping claimed_at. Mirrors
-- ClaimApplyFormBatch: FOR UPDATE OF q locks only queue rows (a bare FOR UPDATE would also
-- lock jobs, making concurrent claim waves contend), SKIP LOCKED lets concurrent workers
-- take disjoint rows, and the lease predicate reclaims entries whose worker died, so no
-- separate reaper process is needed. failed_at/blocked_at excluded so a dead-lettered or
-- parked attempt is never reclaimed by this query — auto_apply_queue_claimable_idx exists
-- for exactly this predicate.
--
-- Returns job.source, job.external_id and job.url because the caller builds the sidecar
-- request from the row alone — source doubles as the ATS provider name, the same vocabulary
-- internal/applyform's Provider field already uses, and external_id (board:posting-id) is
-- what internal/applyform's own schema fetchers need to reuse their existing per-provider
-- API calls rather than re-deriving them.
WITH claimable AS (
    SELECT q.id, q.user_id, q.job_id
    FROM auto_apply_queue q
    WHERE q.failed_at IS NULL
      AND q.blocked_at IS NULL
      AND (q.claimed_at IS NULL
           OR q.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY q.id
    FOR UPDATE OF q SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE auto_apply_queue q
SET claimed_at = now()
FROM claimable c
JOIN jobs j ON j.id = c.job_id
WHERE q.id = c.id
RETURNING q.id, q.user_id, q.job_id, j.source, j.external_id, j.url;

-- name: DeleteAutoApplyEntry :exec
-- Retire an attempt that submitted successfully. jobtracking's MarkJobApplied (called in
-- the same transaction, alongside LockJobForApply) is the durable record; the queue entry
-- has nothing left to say. Mirrors DeleteApplyFormEntry.
DELETE FROM auto_apply_queue
WHERE id = sqlc.arg(id);

-- name: MarkAutoApplyBlocked :exec
-- Park an attempt the sidecar could not fully resolve: record which fields stopped it and
-- why, and leave the lease in place. Unlike RecordAutoApplyFailure this is not a retry
-- countdown — a parked attempt needs new data, not another try — so attempts is left
-- untouched and blocked_at, not failed_at, is what excludes it from
-- auto_apply_queue_claimable_idx from here on.
UPDATE auto_apply_queue
SET blocked_at = now(),
    last_error = sqlc.arg(last_error),
    unmapped   = sqlc.arg(unmapped)
WHERE id = sqlc.arg(id);

-- name: RecordAutoApplyFailure :one
-- Count a transient failure: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally left in
-- place — its expiry gates the retry to a later run, so a failed entry is never
-- reprocessed within the same run. Mirrors RecordApplyFormFailure.
UPDATE auto_apply_queue
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
