-- name: PromoteJobSource :one
-- Transfer a verified discovery posting to its native ATS identity without changing
-- its id, public slug, or referencing application history. The native unique key
-- refuses a collision rather than overwriting another posting. Lock posting then
-- activity before the queue, matching ordinary ingest and curator writers.
WITH locked AS MATERIALIZED (
    SELECT j.id
    FROM jobs j JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(job_id)
      AND j.source = sqlc.arg(expected_source)
      AND j.external_id = sqlc.arg(expected_external_id)
      AND j.url = sqlc.arg(expected_url)
      AND j.application_url = sqlc.arg(expected_application_url)
      AND (j.closed_at IS NULL OR j.closed_reason = 'native_source_pending')
      AND NOT j.is_private AND j.duplicate_of IS NULL
      AND j.duplicate_of_role IS NULL AND j.duplicate_of_fuzzy IS NULL AND j.duplicate_of_aggregator IS NULL
    FOR NO KEY UPDATE OF j, a
), moved AS (
    UPDATE jobs j
    SET source = sqlc.arg(source), external_id = sqlc.arg(external_id),
        url = sqlc.arg(url), application_url = '', content_hash = '', updated_at = now(),
        enrichment = '{}'::jsonb, enriched_at = NULL, enrichment_version = 0
    FROM locked l WHERE j.id = l.id
    RETURNING j.id, COALESCE(j.posted_at, j.created_at) AS job_posted_at
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, job_posted_at FROM moved
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
), invalidated_enrichment AS (
    DELETE FROM enrichment_outbox o USING moved m WHERE o.job_id = m.id
)
SELECT id FROM moved;
