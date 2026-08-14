-- ClaimSearchOutboxBatch orders by COALESCE(jobs.posted_at, jobs.created_at) via a join to
-- jobs, so Postgres cannot push the LIMIT below the sort: it nested-loop-joins EVERY
-- claimable row against jobs before sorting and taking the batch — the identical
-- join-for-ordering shape semantic_outbox carried before 0080_semantic_outbox_job_posted_at.sql
-- (measured live on prod at ~906k claimable rows: 109s for a single claim call; see
-- openspec/changes/prod-semantic-embed-steady-state/design.md Decision 8, which explicitly
-- flagged ClaimSearchOutboxBatch as carrying the same unaddressed risk). Denormalizing the
-- sort key onto search_outbox itself lets the claim query index-scan in claim order — the
-- supporting index is 0097_search_outbox_claim_idx.sql (CREATE INDEX CONCURRENTLY needs its
-- own no-transaction file, same reasoning as 0081's header).
--
-- Nullable — not NOT NULL — deliberately: a SET NOT NULL on a large table takes an exclusive
-- lock to validate. The application always populates this column going forward
-- (EnqueueSearchOutbox); ORDER BY ... NULLS LAST in the claim query is defensive insurance
-- for any row that predates the backfill below, not a load-bearing requirement — jobs.created_at
-- (the COALESCE fallback) is itself NOT NULL DEFAULT now(), so a NULL can only arise from a row
-- this migration's own backfill missed.
--
-- Same staleness caveat as semantic_outbox's job_posted_at: jobs.posted_at is not immutable
-- post-ingest, and search_outbox's ON CONFLICT (job_id) DO NOTHING means an already-queued
-- row's job_posted_at can go stale relative to a later posted_at change. Accepted: it
-- self-heals the moment the row is claimed and re-derived fresh on the next enqueue, and
-- affects only claim ORDER (which job drains first under backlog), never correctness (which
-- jobs drain, or duplicate draining).
--
-- Applied to a fresh volume by initdb after 0095; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads/writes the column.
ALTER TABLE public.search_outbox ADD COLUMN job_posted_at timestamp with time zone;

-- One-time backfill for rows already queued before this column existed. Plain UPDATE: takes
-- only row-level locks as it proceeds, not a table-level one, so — unlike the CONCURRENTLY
-- index in 0097 — it does not need to run outside this migration's normal transaction.
UPDATE public.search_outbox o
SET job_posted_at = COALESCE(j.posted_at, j.created_at)
FROM public.jobs j
WHERE j.id = o.job_id
  AND o.job_posted_at IS NULL;
