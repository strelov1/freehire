-- name: SetJobApplicationURL :one
-- Curator-owned destination: never written by UpsertJob. Lock posting then activity
-- before queueing, like the posting writers, and fence competing curator edits.
WITH locked AS MATERIALIZED (
    SELECT j.id
    FROM jobs j JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(job_id)
      AND NOT j.is_private
      AND j.application_url = sqlc.arg(expected_application_url)
      AND j.application_url <> sqlc.arg(application_url)
    FOR NO KEY UPDATE OF j, a
), changed AS (
    UPDATE jobs j
    SET application_url = sqlc.arg(application_url), updated_at = now()
    FROM locked l WHERE j.id = l.id
    RETURNING j.id, COALESCE(j.posted_at, j.created_at) AS job_posted_at
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, job_posted_at FROM changed
    -- Replace an existing lease's identity so its completion cannot delete this
    -- newer update, and revive dead letters (same contract as RequeueSearchOutbox).
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT id FROM changed;
