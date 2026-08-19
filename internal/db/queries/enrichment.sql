-- name: EnqueuePendingJobs :execrows
-- Idempotent backfill: enqueue every OPEN job that is unenriched or below the target
-- schema version. Closed jobs (closed_at IS NOT NULL) are skipped — a dead posting no
-- user will see should not consume LLM budget. Gated on the same is_tech = true and
-- description <> '' conditions EnqueueJobEnrichment uses (see that query's comment for
-- why the is_tech IS NULL bucket — unresolved by both the title dictionary and the
-- description — is deliberately excluded, not just the confirmed-non-tech
-- is_tech = false one, and why a blank description is excluded regardless of category)
-- so a version bump or a fresh backfill run re-evaluates the whole catalogue under the
-- identical rule, not a looser one. ON CONFLICT keeps exactly one entry per (job_id,
-- target_version), so running this every command invocation never duplicates work.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
  AND is_tech IS TRUE
  AND description <> ''
ON CONFLICT (job_id, target_version) DO NOTHING;

-- name: ClaimEnrichmentBatch :many
-- Claim a batch of live, unleased entries for OPEN jobs, freshest job first, by
-- stamping claimed_at. The jobs join lets the claim order by posting freshness and
-- skip closed jobs, so LLM budget goes to live postings users will actually see.
-- Freshness is COALESCE(posted_at, created_at): jobs without a source post date
-- (telegram/linksource and some ATS) fall back to ingest time, so they rank by
-- recency instead of starving behind every dated job under NULLS LAST. FOR UPDATE OF o
-- locks only outbox rows (a bare FOR UPDATE would also lock jobs, making concurrent
-- claim waves contend); SKIP LOCKED lets concurrent workers take disjoint rows; the
-- lease predicate reclaims entries whose worker died (stale claimed_at), so no
-- separate reaper process is needed.
WITH claimable AS (
    SELECT o.id
    FROM enrichment_outbox o
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
UPDATE enrichment_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.job_id, o.target_version;

-- name: DeleteIneligibleEnrichmentOutbox :execrows
-- Reap live entries ClaimEnrichmentBatch can never take: their job has closed, become a
-- non-canonical repost, or gone. That query's inner join plus its closed/duplicate filter
-- correctly skips them — a dead or hidden posting should not spend LLM budget — but until
-- this existed nothing deleted them either, and they accumulated. The same gap
-- DeleteIneligibleSearchOutbox closed for search_outbox, on the queue beside it.
--
-- Measured on prod 2026-08-19, before the first run: of 1,118,601 entries, 486,178 sat
-- behind a closed job, 216,283 behind a duplicate, and 19 behind no job at all. Only
-- 484,651 were claimable — 57% of the queue was unreachable, and the depth gauge read it
-- as work.
--
-- Deleting is not lossy: EnqueuePendingJobs re-enqueues every open, non-duplicate,
-- unenriched, tech job with a description, so an entry reaped here comes back on its own
-- if the job becomes eligible again.
--
-- Dead-lettered rows (failed_at set) are deliberately left alone, exactly as in the search
-- reaper: they are a record of repeated failure, surfaced as freehire_queue_dead_letters,
-- so reaping them would erase the evidence rather than the garbage.
--
-- Bounded by max_rows so one enrich run cannot turn into an unbounded delete on the first
-- pass over a long-accumulated backlog; the next run takes the next slice.
--
-- There is a race here, and it is left unserialized deliberately. A job can become eligible
-- again (reopened, or released from duplicate_of) while this statement runs: the enqueue that
-- follows that lifecycle write hits ON CONFLICT DO NOTHING because this row still exists, and
-- then this deletes it, leaving an eligible job with no entry. Three things bound it. The
-- window is one statement, not a transaction — once the row is gone the next
-- EnqueueJobEnrichment inserts normally. Runner.Run reaps BEFORE it enqueues, so its own
-- EnqueuePendingJobs re-adds anything that became eligible up to that point. And
-- EnqueuePendingJobs re-evaluates the whole catalogue at the start of every run, so the worst
-- case is one cron period of delay for one job, not a lost posting.
--
-- Serializing it would mean locking jobs rows from a housekeeping statement — contending with
-- ingest on the hottest table in the schema to prevent an hour's delay on a job that is not
-- yet enriched anyway. DeleteIneligibleSearchOutbox carries the identical race for the identical
-- reason.
DELETE FROM enrichment_outbox
WHERE id IN (
    SELECT o.id
    FROM enrichment_outbox o
    LEFT JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (j.id IS NULL OR j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)
    LIMIT sqlc.arg(max_rows)
);

-- name: DeleteEnrichmentEntry :exec
DELETE FROM enrichment_outbox
WHERE id = $1;

-- name: RecordEnrichmentFailure :one
-- Count a failed attempt: bump attempts, record the error, and decide whether to
-- dead-letter (set failed_at). The lease (claimed_at) is intentionally left in place —
-- its expiry gates the retry to a later run and doubles as the crash reaper, so a
-- failed entry is never reprocessed within the same run.
--
-- Which bound applies depends on who is at fault (internal/enrich.postingAtFault):
--
--   posting_at_fault  → the attempt ceiling. The posting cannot be enriched, so each
--                       try is a real try at something that may be impossible.
--   otherwise         → the entry's queue age. A gateway error says nothing about the
--                       posting, and an attempt counter does not measure how long an
--                       outage lasts: a claimed entry is re-claimable once its lease
--                       expires, so an entry at the head of the queue accrues roughly
--                       twelve attempts an hour while the gateway is down. Three
--                       attempts is fifteen minutes. Both July 2026 LiteLLM outages ran
--                       for days and permanently dead-lettered 172,875 enrichable
--                       postings between them — every one of them then invisible to
--                       search, since an unenriched job has no category to index.
--
-- The age bound still exists so an entry nothing can ever serve stops eventually.
UPDATE enrichment_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN sqlc.arg(posting_at_fault)::boolean
                         THEN CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() END
                     ELSE CASE
                              -- A non-positive window means "never bury on age". Left
                              -- as an arithmetic comparison it would mean the opposite:
                              -- created_at < now() - 0 is true for every row, so a
                              -- caller that forgot to set the window would dead-letter
                              -- everything on its first failure — precisely the bug
                              -- this statement exists to fix. A misconfiguration must
                              -- cost retries, not postings.
                              WHEN sqlc.arg(upstream_grace_days)::int > 0
                                  AND created_at < now() - make_interval(days => sqlc.arg(upstream_grace_days)::int)
                                  THEN now()
                          END
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
