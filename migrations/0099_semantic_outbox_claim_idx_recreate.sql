-- migrate: no-transaction
--
-- Recreates semantic_outbox_claim_idx with the null-order fix 0098 dropped it for:
-- `job_posted_at DESC NULLS LAST` now matches ClaimSemanticBatch's ORDER BY exactly, so
-- the query planner can index-scan in claim order instead of a seq scan + sort over the
-- whole claimable set. Verified live on prod (2026-08-15): EXPLAIN on the exact claim
-- query shape went from a Parallel Seq Scan + Sort (cost ~49700) to a plain Index Scan
-- (cost ~4.9) against ~950k claimable rows.
--
-- Same CONCURRENTLY-needs-its-own-file reasoning as 0098/0081/0097; applied to a fresh
-- volume by initdb immediately after 0098; on an existing prod volume, build it by hand,
-- detached from the SSH session (systemd-run or nohup) — a CONCURRENTLY build dies with
-- its ssh session and leaves an INVALID index behind.
CREATE INDEX CONCURRENTLY IF NOT EXISTS semantic_outbox_claim_idx ON public.semantic_outbox
    USING btree (job_posted_at DESC NULLS LAST, job_id DESC) WHERE (failed_at IS NULL);
