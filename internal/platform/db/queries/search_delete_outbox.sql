-- name: ClaimSearchDeleteOutboxBatch :many
-- Claim a batch of live, unleased removals by stamping claimed_at.
--
-- Claims retain missing rows: removal needs only the document id. The worker's
-- PrepareSearchDeletions cancels stale removals for open public canonical jobs
-- immediately before sending a wave to Meilisearch.
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
-- A job may reopen while its Meilisearch removal is in flight. Atomically queue
-- its current content for restoration before forgetting the removal. Missing or
-- closed jobs need no restoration, and private/repost rows must not be published.
WITH completed AS (
    DELETE FROM search_delete_outbox
    WHERE id = ANY(sqlc.arg(ids)::bigint[])
    RETURNING job_id
)
INSERT INTO search_outbox (job_id, job_posted_at)
SELECT j.id, COALESCE(j.posted_at, j.created_at)
FROM completed c JOIN jobs j ON j.id = c.job_id
WHERE j.closed_at IS NULL AND j.duplicate_of IS NULL AND NOT j.is_private
ON CONFLICT (job_id) DO NOTHING;

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
-- garbage, since a vanished job cannot be indexed. Here it is the whole point — a
-- hard-deleted job's document still has to leave the index.
--
-- **It has no Go caller, and that is the decision, not an oversight.** No outbox in this
-- repository reaps its dead letters — there is no DeleteDead… for search_outbox,
-- enrichment_outbox or semantic_outbox — so scheduling this one would answer the
-- give-up question differently for one queue than for the other nine. Worse, running it
-- would delete the only record that a removal was abandoned, which is exactly what
-- `freehire_queue_dead_letters{queue="search_delete_outbox"}` now publishes. Reaping is
-- how a stranded population stops being visible, not how it gets fixed.
--
-- What it would ALSO not fix: every enqueue is `ON CONFLICT (job_id) DO NOTHING` against
-- a UNIQUE key, so while a dead-lettered entry sits here that posting cannot be re-queued
-- for removal — not by a later close, and not by a hard delete. Deleting the
-- entry would re-open that door only for a posting something closes AGAIN, which a
-- closed posting is not. The bounded harm is the reason this is left as it stands: a
-- full `make reindex` streams only OPEN jobs into the fresh index, so the stale document
-- leaves on the next scheduled rebuild regardless. Re-arming the row (an `ON CONFLICT …
-- DO UPDATE` that clears failed_at where it is set) is the fix if the gauge ever shows
-- this happening; it is the identical CTE in the closing queries in jobs.sql (all but
-- CloseMisattributedSourceJobs, which queues nothing on purpose), and worth
-- doing on evidence rather than on the strength of the reading above.
DELETE FROM search_delete_outbox
WHERE failed_at IS NOT NULL
  AND failed_at < sqlc.arg(cutoff);

-- name: PrepareSearchDeletions :many
-- Cancel obsolete removals and requeue their current content before acknowledging
-- them. Lock only the reopened job rows for this statement: a concurrent close
-- waits, then inserts a NEW removal whose id an old acknowledgment cannot delete.
-- Missing, closed, private, or noncanonical jobs still need their document removed.
WITH reopened AS MATERIALIZED (
    SELECT j.id, j.posted_at, j.created_at
    FROM jobs j
    WHERE j.id = ANY(sqlc.arg(job_ids)::bigint[])
      AND j.closed_at IS NULL AND j.duplicate_of IS NULL AND NOT j.is_private
    ORDER BY j.id
    FOR UPDATE OF j
), cancelled AS (
    DELETE FROM search_delete_outbox o
    USING reopened r
    WHERE o.job_id = r.id
    RETURNING o.job_id
), requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT r.id, COALESCE(r.posted_at, r.created_at)
    FROM reopened r JOIN cancelled c ON c.job_id = r.id
    ON CONFLICT (job_id) DO NOTHING
)
SELECT requested.id::bigint AS job_id,
       (j.id IS NULL OR COALESCE(j.is_private, false) OR j.duplicate_of IS NOT NULL)::boolean AS hard_delete
FROM unnest(sqlc.arg(job_ids)::bigint[]) AS requested(id)
LEFT JOIN jobs j ON j.id = requested.id
WHERE NOT EXISTS (SELECT 1 FROM reopened r WHERE r.id = requested.id);
