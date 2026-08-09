-- ClaimSemanticBatch orders by COALESCE(jobs.posted_at, jobs.created_at) via a join to
-- jobs, so Postgres cannot push the LIMIT below the sort: it nested-loop-joins EVERY
-- claimable row against jobs before sorting and taking the batch. Measured live on prod
-- at ~906k claimable rows: 109s for a single claim call, independent of batch size (see
-- openspec/changes/prod-semantic-embed-steady-state/design.md Decision 8 for the full
-- EXPLAIN ANALYZE). Denormalizing the sort key onto semantic_outbox itself lets the
-- claim query use a plain index scan with no join — the supporting index is
-- 0081_semantic_outbox_claim_idx.sql (CREATE INDEX CONCURRENTLY needs its own
-- no-transaction file: Postgres runs a multi-statement file sent as one query in an
-- implicit transaction, which CONCURRENTLY forbids, and this file's other statements
-- don't need that treatment — see 0081's own header for why they're split).
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
-- Applied to a fresh volume by initdb after 0079; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads/writes the column.
ALTER TABLE public.semantic_outbox ADD COLUMN job_posted_at timestamp with time zone;

-- One-time backfill for rows already queued before this column existed. Plain UPDATE:
-- takes only row-level locks as it proceeds, not a table-level one, so — unlike the
-- CONCURRENTLY index in 0081 — it does not need to run outside this migration's normal
-- transaction.
UPDATE public.semantic_outbox o
SET job_posted_at = COALESCE(j.posted_at, j.created_at)
FROM public.jobs j
WHERE j.id = o.job_id
  AND o.job_posted_at IS NULL;
