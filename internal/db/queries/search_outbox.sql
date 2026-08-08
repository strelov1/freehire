-- name: EnqueueSearchOutbox :exec
-- Queue a job for the live facet index. Called by cmd/ingest inside the same
-- transaction as the job's upsert, only when the write inserted or changed indexed
-- content (mirrors the gate the old inline SubmitJobs push used). ON CONFLICT keeps
-- exactly one live entry per job, so a job changed again before it drains is not
-- queued twice.
INSERT INTO search_outbox (job_id)
VALUES (sqlc.arg(job_id))
ON CONFLICT (job_id) DO NOTHING;

-- name: ClaimSearchOutboxBatch :many
-- Claim a batch of live, unleased entries for OPEN canonical jobs, freshest job
-- first, by stamping claimed_at. Mirrors ClaimEnrichmentBatch: the jobs join lets the
-- claim skip a job that closed or became a non-canonical repost after it was queued
-- (that reconciles on the next full reindex, same as before this queue existed) and
-- order by posting freshness. FOR UPDATE OF o locks only outbox rows; SKIP LOCKED
-- lets concurrent workers take disjoint rows; the lease predicate reclaims entries
-- whose worker died (stale claimed_at), so no separate reaper process is needed.
WITH claimable AS (
    SELECT o.id
    FROM search_outbox o
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
UPDATE search_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.job_id;

-- name: DeleteSearchOutboxEntries :exec
DELETE FROM search_outbox
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: DeleteSearchOutboxCreatedBefore :execrows
-- A full facet reindex reads every job's CURRENT content directly from Postgres, so
-- any entry queued before the run started is provably already reflected in the
-- freshly-swapped live index — cmd/reindex calls this once, after a successful
-- Promote(), with the run's own start timestamp. Entries queued during the run are
-- left alone: some represent a job that changed again after the reindex's scan
-- already passed its row, so a future search-drain run still needs them.
--
-- created_at alone is NOT enough: EnqueueSearchOutbox's ON CONFLICT (job_id) DO
-- NOTHING means created_at is stamped only on a job's FIRST enqueue since its last
-- drain, so a job re-changed while its outbox row is still pending (e.g. because
-- search-drain is paused for the reindex's whole duration) keeps its OLD created_at.
-- jobs.updated_at is stamped in the same transaction as every EnqueueSearchOutbox
-- call (cmd/ingest/store.go), so requiring it to also predate the cutoff catches a
-- job re-changed during the run even when its outbox row's created_at did not move.
DELETE FROM search_outbox o
USING jobs j
WHERE o.job_id = j.id
  AND o.created_at < sqlc.arg(before)
  AND j.updated_at < sqlc.arg(before);

-- name: RecordSearchOutboxFailure :one
-- Count a failed attempt: bump attempts, record the error, and dead-letter (set
-- failed_at) once attempts reach the max. The lease (claimed_at) is intentionally
-- left in place — its expiry gates the retry to a later run and doubles as the
-- crash reaper, so a failed entry is never reprocessed within the same run.
UPDATE search_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
