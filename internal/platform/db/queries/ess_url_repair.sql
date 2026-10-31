-- name: RepairESSJobURL :one
-- One-off repair for ESS's legacy query-only vacancy links. Compare the URL and
-- hash read by the worker so a concurrent ingest cannot receive a stale hash.
-- Closed rows are repaired too because a later liveness refresh can reopen them.
WITH updated AS (
    UPDATE jobs AS j
    SET url = sqlc.arg(url),
        content_hash = sqlc.narg(content_hash),
        updated_at = now()
    WHERE j.id = sqlc.arg(id)
      AND j.source = 'ess'
      AND NOT j.is_private
      AND j.url = sqlc.arg(old_url)
      AND j.content_hash IS NOT DISTINCT FROM sqlc.narg(old_content_hash)
      AND j.url IS DISTINCT FROM sqlc.arg(url)
    RETURNING j.id, j.closed_at, COALESCE(j.posted_at, j.created_at) AS eff_posted_at
), enqueued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE closed_at IS NULL
    ON CONFLICT (job_id) DO NOTHING
    RETURNING job_id
)
SELECT (SELECT count(*) FROM updated)::bigint AS updated_count,
       (SELECT count(*) FROM enqueued)::bigint AS enqueued_count;
