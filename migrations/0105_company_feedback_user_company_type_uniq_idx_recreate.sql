-- migrate: no-transaction
--
-- Recreates the company feedback uniqueness arbiter that 0104 dropped, now
-- scoped to (user_id, company_slug, feedback_type) instead of just
-- (user_id, company_slug) — the per-category rule 0104's header explains.
-- Same CONCURRENTLY-needs-its-own-file reasoning as 0104/0086/0098: Postgres
-- forbids CONCURRENTLY inside a transaction block, and a plain CREATE UNIQUE
-- INDEX would hold a SHARE lock blocking writes to company_feedback for the
-- whole build.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY, immediately after 0104 in the same
-- maintenance window — see 0104's header for why the gap between them
-- matters. On an existing prod volume, build it by hand, detached from the
-- SSH session (systemd-run or nohup) — a CONCURRENTLY build dies with its
-- ssh session and leaves an INVALID index behind, the same warning
-- 0078/0081/0086 give.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS company_feedback_user_company_type_uniq_idx
    ON public.company_feedback (user_id, company_slug, feedback_type) WHERE user_id IS NOT NULL;
