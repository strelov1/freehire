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
            -- A private posting must never be drained into the public facet index, and
            -- "InsertPrivateJob enqueues none" is not enough to stop it: the role-duplicate
            -- pass requeues every row whose duplicate_of goes back to NULL, and canon is
            -- MIN(id) among OPEN rows with failover when the canon closes. So a pasted JD
            -- marked a duplicate of the public posting it was copied from is ELECTED canon
            -- the moment that public one soft-closes, and is requeued from there — title,
            -- company, description and public_slug into the index anyone searches.
            -- Constructed against Postgres before this line existed; the row was claimed.
            -- cmd/reindex already drops private rows on the full rebuild; this is the same
            -- rule on the incremental path, which is the one that had no predicate at all.
            AND NOT j.is_private
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
      -- The exact complement of ClaimSearchOutboxBatch's EXISTS (open, canonical, NOT
      -- is_private). A private posting (a user's pasted JD) is never indexed, so its
      -- entry can never be claimed; without OR j.is_private it stayed queued forever,
      -- counted in depth and pinned the oldest-age gauge to its own age.
      AND (j.id IS NULL OR j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL OR j.is_private)
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

-- name: RequeueSearchOutbox :exec
-- Queue a job whose row was rewritten after an entry may already have been claimed. Unlike
-- EnqueueSearchOutbox it re-stamps a live entry (a new id, so the drain's completion of the
-- claimed one does not delete it) and clears its dead-letter state, so the document is pushed
-- again from the row as it stands now — the shape SetJobEngagement uses for the same reason.
INSERT INTO search_outbox (job_id, job_posted_at)
SELECT j.id, COALESCE(j.posted_at, j.created_at)
FROM jobs j
WHERE j.id = sqlc.arg(job_id) AND j.closed_at IS NULL AND NOT j.is_private
ON CONFLICT (job_id) DO UPDATE
    SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL;

-- name: EnqueueSearchAgeCrossings :execrows
-- Queue the open canonical postings whose search document changed in the window (since, until]
-- through their age alone, with no write to the row: their created_at age crossed one of
-- age_days — the reality class turns at fixed ages (jobreality.AgeCrossingDays) — or their
-- posted_at age crossed one of posted_age_days, where the fake-freshness flag turns
-- (jobreality.PostedAgeCrossingDays). Nothing else would ever queue them, so without this pass
-- they waited for a full rebuild. EnqueueSearchUnseenCrossings is the other half of the pass.
--
-- Each band is a range scan: created_at on jobs_open_created_idx, posted_at on
-- jobs_open_enrich_freshness_idx (COALESCE(posted_at, created_at) is posted_at whenever
-- posted_at is set). One OR over the three conditions read the whole jobs heap, 41 minutes on
-- average on prod (2026-11-22), and a bare posted_at range made the planner skip-scan
-- jobs_company_slug_idx instead.
--
-- DO NOTHING and id order, unlike the per-row writers' re-stamp: this one statement upserts
-- thousands of entries while every ingest transaction upserts its own, and the re-stamp's row
-- locks taken in scan order deadlocked against them on its first prod run (2026-11-05). An
-- entry already queued is pushed from the row as it stands when claimed. A wave already in
-- flight at the crossing misses it and deletes its entry; the caller's next run re-covers the
-- last hour of this window for exactly that case (cmd/reindex timeCrossingsOverlap).
INSERT INTO search_outbox (job_id, job_posted_at)
SELECT c.id, c.posted_at
FROM (
    SELECT j.id, COALESCE(j.posted_at, j.created_at) AS posted_at
    FROM unnest(sqlc.arg(age_days)::int[]) AS d(days)
    JOIN jobs j
      ON j.closed_at IS NULL
     AND j.created_at >  sqlc.arg(since)::timestamptz - make_interval(days => d.days)
     AND j.created_at <= sqlc.arg(until)::timestamptz - make_interval(days => d.days)
    WHERE j.duplicate_of IS NULL AND NOT j.is_private
    UNION
    SELECT j.id, COALESCE(j.posted_at, j.created_at)
    FROM unnest(sqlc.arg(posted_age_days)::int[]) AS d(days)
    JOIN jobs j
      ON j.closed_at IS NULL
     AND j.posted_at IS NOT NULL
     AND COALESCE(j.posted_at, j.created_at) >  sqlc.arg(since)::timestamptz - make_interval(days => d.days)
     AND COALESCE(j.posted_at, j.created_at) <= sqlc.arg(until)::timestamptz - make_interval(days => d.days)
    WHERE j.duplicate_of IS NULL AND NOT j.is_private
) c
ORDER BY c.id
ON CONFLICT (job_id) DO NOTHING;

-- name: EnqueueSearchUnseenCrossings :one
-- Queue the open canonical postings whose source's unseen window ran out in (since, until]
-- (search.Withheld), so the drain takes the document out. sources/unseen_seconds is that
-- per-source map; a source absent from it is never withheld on this ground. A posting crosses
-- at last_seen_at + window.
--
-- last_seen_at (job_activity's) is deliberately in no index — the liveness touch must stay
-- heap-only — so this walks job_activity in primary-key order, chunk_size rows after after_id
-- per call, and returns the last job_id it read as the next cursor (0 once nothing is left).
-- Each call is a short statement of its own, so no snapshot is held across the whole table.
-- A chunk is first cut to the rows inside one of the distinct windows' bands (a handful of
-- windows, not one per source), and only those reach the jobs lookup.
--
-- DO NOTHING and id order for the same reasons as EnqueueSearchAgeCrossings.
WITH bands AS MATERIALIZED (
    SELECT DISTINCT
        sqlc.arg(since)::timestamptz - make_interval(secs => s.secs) AS lo,
        sqlc.arg(until)::timestamptz - make_interval(secs => s.secs) AS hi
    FROM unnest(sqlc.arg(unseen_seconds)::bigint[]) AS s(secs)
), chunk AS MATERIALIZED (
    SELECT a.job_id, a.last_seen_at
    FROM job_activity a
    WHERE a.job_id > sqlc.arg(after_id)::bigint
    ORDER BY a.job_id
    LIMIT sqlc.arg(chunk_size)::int
), candidates AS MATERIALIZED (
    SELECT c.job_id, c.last_seen_at
    FROM chunk c
    WHERE EXISTS (SELECT 1 FROM bands b WHERE c.last_seen_at > b.lo AND c.last_seen_at <= b.hi)
), ins AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT j.id, COALESCE(j.posted_at, j.created_at)
    FROM candidates c
    JOIN jobs j ON j.id = c.job_id
    CROSS JOIN LATERAL (
        SELECT (sqlc.arg(unseen_seconds)::bigint[])[array_position(sqlc.arg(sources)::text[], j.source)] AS secs
    ) w
    WHERE j.closed_at IS NULL AND j.duplicate_of IS NULL AND NOT j.is_private
      AND w.secs IS NOT NULL
      AND c.last_seen_at >  sqlc.arg(since)::timestamptz - make_interval(secs => w.secs)
      AND c.last_seen_at <= sqlc.arg(until)::timestamptz - make_interval(secs => w.secs)
    ORDER BY j.id
    ON CONFLICT (job_id) DO NOTHING
    RETURNING 1
)
SELECT COALESCE((SELECT max(job_id) FROM chunk), 0)::bigint AS last_id,
       (SELECT count(*) FROM ins)::bigint AS enqueued;
