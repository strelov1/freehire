-- migrate: no-transaction
--
-- Recreates search_outbox_claim_idx with the null-order fix 0100 dropped it for:
-- `job_posted_at DESC NULLS LAST` now matches ClaimSearchOutboxBatch's ORDER BY exactly,
-- so the query planner can index-scan in claim order instead of a seq scan + sort over
-- the whole claimable set. Verified live on prod (2026-08-15): EXPLAIN on the exact
-- claim query shape went from a Seq Scan + Sort to a plain Index Scan.
--
-- Same CONCURRENTLY-needs-its-own-file reasoning as 0100/0097/0081; applied to a fresh
-- volume by initdb immediately after 0100; on an existing prod volume, build it by hand,
-- detached from the SSH session — a CONCURRENTLY build dies with its ssh session and
-- leaves an INVALID index behind.
CREATE INDEX CONCURRENTLY IF NOT EXISTS search_outbox_claim_idx ON public.search_outbox
    USING btree (job_posted_at DESC NULLS LAST, job_id DESC) WHERE (failed_at IS NULL);
