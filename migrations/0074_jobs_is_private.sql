-- Marks a job as visible only to its creator: excluded from Meilisearch, from the
-- DB-backed jobs list, and from the public job-detail/fit/tailor reads for anyone
-- else. Backs the jd-tailor-intake change's private JD path (pasted text or an
-- unrecognized/generic-scrape URL).
--
-- Plain transactional migration: ADD COLUMN ... DEFAULT false is metadata-only
-- (PG11+), no table rewrite or scan, ACCESS EXCLUSIVE held for microseconds. Every
-- existing row takes false, which is correct — every job in the catalog today is
-- public.
--
-- Apply BEFORE deploying code that reads is_private (every SELECT * job read regens
-- with this column, per this migration's sqlc run): an unapplied column is a 42703 on
-- every request that touches it, not a degraded feature — see migration 0068's header
-- for the same ordering rule.

ALTER TABLE public.jobs
    ADD COLUMN is_private boolean NOT NULL DEFAULT false;
