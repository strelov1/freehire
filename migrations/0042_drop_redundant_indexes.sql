-- Drop two redundant indexes superseded by later partial indexes:
--
-- emails_user_received_idx (0014) on (user_id, received_at DESC) is fully covered
-- by emails_user_live_idx (0022), the same key columns WHERE deleted_at IS NULL:
-- every per-user inbox read (ListEmails, CountEmails) filters deleted_at IS NULL,
-- and the per-application listing (ListJobEmails, no deleted_at filter) is served
-- by emails_job_id_idx. No query reads deleted mail (no trash view).
--
-- companies_job_count_idx (0001) on (job_count DESC, name) is dead since
-- companies_hiring_job_count_idx (0023): every read that orders or filters by
-- job_count (ListCompanies, CountCompanies, the reindex keyset scan) scopes to
-- job_count > 0 and rides the partial index.
--
-- Applied to a fresh volume by initdb after 0041; on an existing prod volume drop
-- CONCURRENTLY out of band (a plain DROP INDEX takes an ACCESS EXCLUSIVE lock):
--   DROP INDEX CONCURRENTLY IF EXISTS emails_user_received_idx;
--   DROP INDEX CONCURRENTLY IF EXISTS companies_job_count_idx;
-- Before dropping on prod, confirm with pg_stat_user_indexes that idx_scan is ~0.
DROP INDEX IF EXISTS emails_user_received_idx;
DROP INDEX IF EXISTS companies_job_count_idx;
