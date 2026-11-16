-- name: WithholdLinkedInDiscovery :one
-- This is a catalogue withdrawal, not an employer-death signal. Preserve the
-- posting/history while a native application method or ATS owner is unverified.
WITH locked AS MATERIALIZED (
    SELECT j.id FROM jobs j JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(job_id)::bigint AND j.source = 'linkedin'
      AND j.closed_at IS NULL AND NOT j.is_private
      AND j.duplicate_of IS NULL AND j.duplicate_of_role IS NULL
      AND j.duplicate_of_fuzzy IS NULL AND j.duplicate_of_aggregator IS NULL
    FOR NO KEY UPDATE OF j, a
), withdrawn AS (
    UPDATE jobs j SET closed_at = now(), closed_reason = 'native_source_pending', updated_at = now()
    FROM locked l WHERE j.id = l.id
    RETURNING j.id, COALESCE(j.posted_at, j.created_at) AS posted_at
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, posted_at FROM withdrawn
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT count(*)::bigint FROM withdrawn;

-- name: RestoreConfirmedLinkedInApplication :one
-- Called only after reading the primary on-site application control. Genuine
-- source closures and duplicate/private history are never reopened by this repair.
WITH locked AS MATERIALIZED (
    SELECT j.id FROM jobs j JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(job_id)::bigint AND j.source = 'linkedin'
      AND j.closed_reason = 'native_source_pending' AND NOT j.is_private
      AND j.duplicate_of IS NULL AND j.duplicate_of_role IS NULL
      AND j.duplicate_of_fuzzy IS NULL AND j.duplicate_of_aggregator IS NULL
    FOR NO KEY UPDATE OF j, a
), restored AS (
    UPDATE jobs j SET closed_at = NULL, closed_reason = '', updated_at = now()
    FROM locked l WHERE j.id = l.id
    RETURNING j.id, COALESCE(j.posted_at, j.created_at) AS posted_at
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, posted_at FROM restored
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT count(*)::bigint FROM restored;

-- name: RetirePendingLinkedInDiscovery :exec
UPDATE jobs SET closed_reason = 'ats_covered', updated_at = now()
WHERE id = $1 AND source = 'linkedin'
  AND closed_at IS NOT NULL AND closed_reason = 'native_source_pending';
