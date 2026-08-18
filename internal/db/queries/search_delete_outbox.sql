-- name: ClaimSearchDeleteOutboxBatch :many
-- Claim a batch of live, unleased removals by stamping claimed_at.
--
-- Deliberately has NO `EXISTS (SELECT 1 FROM jobs ...)` guard, unlike
-- ClaimSearchOutboxBatch. That query skips entries whose job closed or became a duplicate,
-- because there is nothing left to index. Here the opposite holds: a job that is closed,
-- demoted, or hard-deleted is exactly what this queue exists to act on, and a removal needs
-- only the primary key. Adding the guard would make the queue skip every entry it was
-- created for.
--
-- Claim order is insertion order (the id index), not freshest-first. Indexing sorts by
-- job_posted_at because a searcher notices a missing new posting sooner than a missing old
-- one; for removal one stale document is as wrong as another, so the simpler order wins and
-- the partial index serves it directly.
--
-- FOR UPDATE OF o locks only queue rows; SKIP LOCKED lets concurrent workers take disjoint
-- rows; the lease predicate reclaims entries whose worker died (stale claimed_at), so no
-- separate reaper process is needed.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM search_delete_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY o.id
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE search_delete_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.job_id, o.attempts;

-- name: CompleteSearchDeleteOutbox :exec
-- Drop the entries for removals that landed. Deleting a document that was never indexed is
-- a no-op in Meilisearch, so "landed" includes the common case where the job had no document
-- to begin with — there is nothing to distinguish and nothing to retry.
DELETE FROM search_delete_outbox
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: RecordSearchDeleteOutboxFailure :one
-- Record a failed attempt against one entry, dead-lettering it once it passes max_attempts so
-- a permanently poisonous entry stops being reclaimed by the lease forever.
--
-- Returns failed_at so the caller reads the dead-letter decision back rather than recomputing
-- it, which is what keeps the threshold in one place. Mirrors RecordSearchOutboxFailure.
UPDATE search_delete_outbox
SET attempts   = attempts + 1,
    claimed_at = NULL,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() ELSE NULL END
WHERE id = sqlc.arg(id)
RETURNING failed_at;

-- name: DeleteDeadSearchDeleteOutbox :execrows
-- Housekeeping: drop dead-lettered entries older than the cutoff.
--
-- Note what this does NOT reap: entries whose job row is gone. For search_outbox that is
-- garbage, since a vanished job cannot be indexed. Here it is the whole point — cmd/prune
-- hard-deletes jobs, and their documents still have to leave the index.
DELETE FROM search_delete_outbox
WHERE failed_at IS NOT NULL
  AND failed_at < sqlc.arg(cutoff);
