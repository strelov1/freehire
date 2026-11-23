-- name: EnqueuePendingJobs :one
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
--
-- include_non_tech (ENRICH_INCLUDE_NON_TECH, off by default) lifts the is_tech gate for this
-- backfill only, so every open posting with a body is enriched and its category asked of the
-- model (#4412). The ingest-time EnqueueJobEnrichment stays tech-only: it is the fast path for
-- a new technical posting, and the next enrich run's backfill picks up the rest.
--
-- One call enqueues one chunk: up to chunk_size eligible jobs below before_id, newest id
-- first along jobs_sitemap_idx, and returns the lowest id it took as the next cursor (0 once
-- nothing is left). A whole-catalogue INSERT ran past 45 minutes with include_non_tech, so
-- deploys killed it and rolled it back, and its uncommitted rows blocked ingest's own
-- enqueue on the same keys; a chunk commits on its own. Zero before_id and zero chunk_size
-- mean no cursor and no limit (one pass, as the integration tests use it).
WITH batch AS (
    SELECT id
    FROM jobs
    WHERE id < COALESCE(NULLIF(sqlc.arg(before_id)::bigint, 0), 9223372036854775807)
      AND closed_at IS NULL
      AND duplicate_of IS NULL
      AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
      AND (is_tech IS TRUE OR sqlc.arg(include_non_tech)::boolean)
      AND description <> ''
    ORDER BY id DESC
    LIMIT NULLIF(sqlc.arg(chunk_size)::int, 0)
), ins AS (
    INSERT INTO enrichment_outbox (job_id, target_version)
    SELECT id, sqlc.arg(target_version)::int FROM batch
    ON CONFLICT (job_id, target_version) DO NOTHING
    RETURNING 1
)
SELECT COALESCE((SELECT min(id) FROM batch), 0)::bigint AS last_id,
       (SELECT count(*) FROM ins)::bigint AS enqueued;

-- name: EnqueueEnrichmentForCompanySlugs :execrows
-- Scoped, one-off re-enqueue used by cmd/backfill-company-type-hint: force OPEN,
-- eligible jobs of the given company_slugs back into enrichment_outbox at the CURRENT
-- target version, regardless of their existing enrichment_version. Unlike
-- EnqueuePendingJobs (which only re-queues rows BELOW the target version), this also
-- picks up a job already enriched under the current version — the case after adding a
-- company to enrich.CompanyTypeHints, whose existing postings need a second pass with
-- the new prompt hint, not a version bump that would re-enrich the whole catalogue.
-- Same eligibility gate as EnqueuePendingJobs, so a re-run never queues work
-- ClaimEnrichmentBatch would refuse anyway. ON CONFLICT leaves a row already pending
-- (not yet claimed) alone, so running this twice in a row costs nothing.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE company_slug = ANY(sqlc.arg(company_slugs)::text[])
  AND closed_at IS NULL
  AND duplicate_of IS NULL
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

-- name: DeleteIneligibleEnrichmentOutbox :one
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
-- One call reads one chunk: chunk_size entries above after_job_id in job_id order (index-only on
-- enrichment_outbox_job_id_target_version_key), checks their jobs against jobs_sitemap_idx — the
-- open-and-canonical partial index, so the probe is the claim's own filter — and returns the last
-- job_id it read as the next cursor (0 once nothing is left). Ineligibility belongs to the job, so
-- every live entry of a doomed job goes, even a version past the chunk's edge. A single statement
-- over the whole queue ran 6 minutes on average and up to 102 on prod (2026-11-22), holding its
-- snapshot all along; a chunk is a short statement of its own. cmd/enrich walks the chunks and
-- passes what is left of its bound as max_rows; a call that hits max_rows has not finished its
-- chunk, so its cursor is only good for stopping.
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
WITH chunk AS MATERIALIZED (
    SELECT o.job_id
    FROM enrichment_outbox o
    WHERE o.job_id > sqlc.arg(after_job_id)::bigint
    ORDER BY o.job_id
    LIMIT sqlc.arg(chunk_size)::int
), doomed AS MATERIALIZED (
    SELECT o.id
    FROM enrichment_outbox o
    WHERE o.job_id IN (
        SELECT c.job_id FROM chunk c
        WHERE NOT EXISTS (
            SELECT 1 FROM jobs j
            WHERE j.id = c.job_id AND j.closed_at IS NULL AND j.duplicate_of IS NULL
        )
    )
      -- Redundant bounds that keep the lookup on the job_id index instead of a full scan.
      AND o.job_id > sqlc.arg(after_job_id)::bigint
      AND o.job_id <= (SELECT max(job_id) FROM chunk)
      AND o.failed_at IS NULL
    LIMIT sqlc.arg(max_rows)::int
), del AS (
    DELETE FROM enrichment_outbox o
    USING doomed d
    WHERE o.id = d.id
    RETURNING 1
)
SELECT COALESCE((SELECT max(job_id) FROM chunk), 0)::bigint AS last_job_id,
       (SELECT count(*) FROM del)::bigint AS deleted;

-- name: DeleteEnrichmentEntry :exec
DELETE FROM enrichment_outbox
WHERE id = $1;

-- name: RecordEnrichmentFailure :one
-- Count a failed attempt: bump attempts, record the error, and decide whether to
-- dead-letter (set failed_at). The lease (claimed_at) is intentionally left in place —
-- its expiry gates the retry to a later run and doubles as the crash reaper, so a
-- failed entry is never reprocessed within the same run.
--
-- Which bound applies depends on who is at fault (internal/ai/enrich.postingAtFault):
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
