-- migrate: no-transaction
--
-- ON DELETE CASCADE looks the child rows up by the foreign-key column, and Postgres
-- does not index that side. auto_apply_queue.job_id and cover_letters.job_id are only
-- the second column of UNIQUE / PRIMARY KEY (user_id, job_id), so a job delete cannot
-- use them. application_interviews.application_id has no index (0070 indexes
-- user_uid, provider_event and user_starts). Deleting a job or an application then
-- seq-scans the child table and holds the lock for the scan.
--
-- CONCURRENTLY, so the build takes SHARE UPDATE EXCLUSIVE and does not block writes
-- for the whole scan. The runner applies a no-transaction file one statement at a
-- time, which is the transaction boundary CONCURRENTLY needs; initdb's psql commits
-- each statement the same way. IF NOT EXISTS because a failure part-way leaves the
-- file unrecorded and the next run must be safe to repeat.
CREATE INDEX CONCURRENTLY IF NOT EXISTS auto_apply_queue_job_id_idx
    ON public.auto_apply_queue (job_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS cover_letters_job_id_idx
    ON public.cover_letters (job_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS application_interviews_application_id_idx
    ON public.application_interviews (application_id);
