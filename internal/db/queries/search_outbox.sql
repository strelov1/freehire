-- name: EnqueueSearchOutbox :exec
-- Queue a job for the live facet index. Called by cmd/ingest inside the same
-- transaction as the job's upsert, only when the write inserted or changed indexed
-- content (mirrors the gate the old inline SubmitJobs push used). ON CONFLICT keeps
-- exactly one live entry per job, so a job changed again before it drains is not
-- queued twice. job_posted_at denormalizes COALESCE(posted_at, created_at) onto the
-- outbox row so ClaimSearchOutboxBatch can sort by it without joining jobs on every
-- claim (see that query's doc comment); the single-row subquery here costs nothing
-- beyond the jobs PK lookup this call already implies.
INSERT INTO search_outbox (job_id, job_posted_at)
SELECT sqlc.arg(job_id)::bigint, COALESCE(posted_at, created_at)
FROM jobs
WHERE id = sqlc.arg(job_id)
ON CONFLICT (job_id) DO NOTHING;

-- name: ClaimSearchOutboxBatch :many
-- Claim a batch of live, unleased entries for OPEN canonical jobs, freshest job
-- first, by stamping claimed_at.
--
-- Orders by the outbox's OWN job_posted_at (denormalized at enqueue time from
-- COALESCE(jobs.posted_at, jobs.created_at) — see EnqueueSearchOutbox) rather than
-- joining jobs to compute it. A join-for-ordering here means Postgres cannot push the
-- LIMIT below the sort — it has to nested-loop-join and sort the ENTIRE claimable set
-- before taking the batch, independent of batch_size. This is the exact pattern
-- ClaimSemanticBatch was measured at 109s per claim call on ~906k claimable rows and
-- fixed the same way (see openspec/changes/prod-semantic-embed-steady-state/design.md
-- Decision 8, which flagged this query as carrying the identical, unaddressed risk).
--
-- The closed/duplicate_of check stays a per-row EXISTS rather than a JOIN: with the
-- job_posted_at index in place, Postgres can index-scan search_outbox in the exact
-- claim order and stop as soon as LIMIT rows pass the EXISTS lookup (a cheap jobs PK
-- probe), instead of materializing and sorting every claimable row up front. A job
-- that closed or became a non-canonical repost after being queued is simply skipped —
-- same behavior as before, just reached via an index scan instead of a full join.
-- NULLS LAST is defensive (EnqueueSearchOutbox always populates the column; this only
-- guards a row that predates the backfill migration).
--
-- FOR UPDATE OF o locks only outbox rows; SKIP LOCKED lets concurrent workers take
-- disjoint rows; the lease predicate reclaims entries whose worker died (stale
-- claimed_at), so no separate reaper process is needed.
WITH claimable AS (
    SELECT o.id, o.job_id
    FROM search_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND EXISTS (
          SELECT 1 FROM jobs j
          WHERE j.id = o.job_id
            AND j.closed_at IS NULL
            AND j.duplicate_of IS NULL
      )
    ORDER BY o.job_posted_at DESC NULLS LAST, o.job_id DESC
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

-- name: DeleteIneligibleSearchOutbox :execrows
-- Reap live entries ClaimSearchOutboxBatch can never take: their job has closed, become
-- a non-canonical repost, or gone. That query's EXISTS requires open AND canonical, so
-- these rows are correctly skipped — there is nothing left to index — but until this
-- existed nothing deleted them either, and they accumulated.
--
-- DeleteSearchOutboxCreatedBefore does not cover them. It additionally requires
-- jobs.updated_at to predate the reindex, and a job demoted to a duplicate is still in
-- its board's feed, so ingest keeps touching it and its updated_at keeps moving. Measured
-- on prod 2026-08-15: 1,618 such rows, 631 of the day-old ones behind a duplicate_of and
-- 138 behind a closed job, the oldest queued eight days earlier.
--
-- Dead-lettered rows (failed_at set) are deliberately left alone: they are a deliberate
-- record of repeated failure and are surfaced as freehire_queue_dead_letters, so reaping
-- them here would erase the evidence rather than the garbage.
--
-- Bounded by max_rows so one drain run cannot turn into an unbounded delete on the first
-- pass over a long-accumulated backlog; the next run takes the next slice.
DELETE FROM search_outbox
WHERE id IN (
    SELECT o.id
    FROM search_outbox o
    LEFT JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (j.id IS NULL OR j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)
    LIMIT sqlc.arg(max_rows)
);

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
