-- migrate: no-transaction
--
-- ClaimSemanticBatch orders by COALESCE(jobs.posted_at, jobs.created_at) via a join to
-- jobs, so Postgres cannot push the LIMIT below the sort: it nested-loop-joins EVERY
-- claimable row against jobs before sorting and taking the batch. Measured live on prod
-- at ~906k claimable rows: 109s for a single claim call, independent of batch size (see
-- openspec/changes/prod-semantic-embed-steady-state/design.md Decision 8 for the full
-- EXPLAIN ANALYZE). Denormalizing the sort key onto semantic_outbox itself lets the
-- claim query use a plain index scan with no join.
--
-- Nullable — not NOT NULL — deliberately: a SET NOT NULL on a 900k+ row table takes an
-- exclusive lock to validate. The application always populates this column going
-- forward (EnqueuePendingSemanticJobs); ORDER BY ... NULLS LAST in the claim query is
-- defensive insurance for any row that predates the backfill below, not a load-bearing
-- requirement — jobs.created_at (the COALESCE fallback) is itself NOT NULL DEFAULT
-- now(), so a NULL can only arise from a row this migration's own backfill missed.
--
-- Correction: jobs.posted_at is NOT immutable post-ingest — UpsertJob's ON CONFLICT DO
-- UPDATE overwrites it unconditionally on every re-ingest (internal/db/queries/jobs.sql),
-- and a moderator edit can change it too. semantic_outbox's ON CONFLICT (job_id,
-- target_model) DO NOTHING means an already-queued row's job_posted_at can go stale
-- relative to a later posted_at change — the same staleness class this table's
-- created_at column already accepts under the identical ON CONFLICT DO NOTHING. Accepted:
-- it self-heals the moment the row is claimed and re-derived fresh next enqueue, and
-- affects only claim ORDER (which job gets embedded first under backlog), never
-- correctness (which jobs get embedded, or duplicated).
--
-- WHY no-transaction, and why the index is CONCURRENTLY: mirrors 0078
-- (jobs_source_id_idx) for the identical reason — semantic_outbox is under continuous
-- prod write traffic (cmd/ingest's enqueue, cmd/embed's claim/update/failure paths), and
-- a plain CREATE INDEX holds a SHARE lock blocking writes for the whole build.
-- CONCURRENTLY takes SHARE UPDATE EXCLUSIVE instead, blocking neither readers nor
-- writers, at the cost of two table passes. ADD COLUMN (no default) is metadata-only
-- (PG11+); the backfill UPDATE below takes only row-level locks as it proceeds, not a
-- table-level one, so it does not need the same treatment.
--
-- Applied to a fresh volume by initdb after 0079; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads/writes the column.
ALTER TABLE public.semantic_outbox ADD COLUMN job_posted_at timestamp with time zone;

-- One-time backfill for rows already queued before this column existed.
UPDATE public.semantic_outbox o
SET job_posted_at = COALESCE(j.posted_at, j.created_at)
FROM public.jobs j
WHERE j.id = o.job_id
  AND o.job_posted_at IS NULL;

-- Partial index over the claimable set (mirrors semantic_outbox_claimable_idx's WHERE
-- clause), pre-sorted in the exact order ClaimSemanticBatch's CTE now requests — a plain
-- index scan with LIMIT, no join, no materialize-then-sort.
CREATE INDEX CONCURRENTLY IF NOT EXISTS semantic_outbox_claim_idx ON public.semantic_outbox
    USING btree (job_posted_at DESC, job_id DESC) WHERE (failed_at IS NULL);
