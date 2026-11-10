-- name: EnqueueFacetdOutbox :exec
-- Queue jobs whose facetd write failed. A job already queued is re-stamped with a NEW id
-- (and its attempts and dead-letter state cleared): a retry that has claimed the old id and
-- loaded the job BEFORE this failed write completes by deleting that old id only, so the
-- newer state is not forgotten. Ids are sorted so concurrent enqueues lock in one order.
INSERT INTO facetd_outbox (job_id)
SELECT DISTINCT j FROM unnest(sqlc.arg(job_ids)::bigint[]) AS t(j) ORDER BY j
ON CONFLICT (job_id) DO UPDATE
    SET id = DEFAULT, created_at = now(), claimed_at = NULL, attempts = 0, last_error = '', failed_at = NULL;

-- name: ClaimFacetdOutboxBatch :many
-- Claim a batch of live, unleased entries by stamping claimed_at; the lease predicate
-- reclaims entries whose worker died. No jobs join: a missing job is a delete to send.
WITH claimable AS (
    SELECT o.id
    FROM facetd_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY o.id
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE facetd_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.job_id;

-- name: CompleteFacetdOutbox :exec
DELETE FROM facetd_outbox
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: RecordFacetdOutboxFailure :execrows
-- Count a failed attempt for a whole claimed batch (one facetd write serves them all),
-- releasing the lease so a later run retries, and dead-lettering entries that reach the max.
UPDATE facetd_outbox
SET attempts   = attempts + 1,
    claimed_at = NULL,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() ELSE NULL END
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: FacetdOutboxStats :one
-- Queue depth (live entries) and dead letters, for the depth metric.
SELECT count(*) FILTER (WHERE failed_at IS NULL)::bigint AS depth,
       count(*) FILTER (WHERE failed_at IS NOT NULL)::bigint AS dead_letters
FROM facetd_outbox;
