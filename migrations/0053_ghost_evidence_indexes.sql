-- Indexes the ghost-signal evidence lookup needs on the public read paths.
--
-- Found in review of the change that added those lookups, not in production: at
-- today's volumes both scans are invisible, and both grow with user activity on
-- endpoints that are already the most-read in the system. The catalogue-scale lesson
-- (see hire-jobs-list-slow-query in the project's history) is that a seq scan on a
-- list endpoint is cheap to add and expensive to find later.
--
-- Applied to a fresh volume by initdb after 0052; on an existing prod volume run this
-- manually (SET ROLE hire). Safe to apply before or after the image — these only make
-- existing queries faster. On prod, prefer CREATE INDEX CONCURRENTLY by hand: the
-- statements below are the initdb form, and a plain CREATE INDEX takes a write lock
-- (CONCURRENTLY cannot run inside the migration runner's transaction). Note also that
-- a CONCURRENTLY build dies with its ssh session and leaves an INVALID index behind,
-- so run it under nohup/screen.

-- ListGhostApplicationEvidence filters user_jobs by job_id and applied_at. The only
-- existing job-side index is user_jobs_job_id_vote_idx, which is partial on
-- `vote IS NOT NULL` and therefore does NOT serve this predicate — the lookup would
-- sequentially scan user_jobs on every job list page, search page and job detail read.
-- Partial on the same terms as the query, so the index stays proportional to the
-- applications rather than to every view and save ever recorded.
CREATE INDEX IF NOT EXISTS user_jobs_job_id_applied_idx
    ON public.user_jobs (job_id) WHERE applied_at IS NOT NULL;

-- The same lookup asks, per candidate application, whether any unconfirmed suggestion
-- points at it (emails.suggested_job_id). emails_job_id_idx covers the CONFIRMED side
-- (job_id) but nothing covers the suggested side. Partial for the same reason: a
-- suggestion is a small minority of stored mail.
CREATE INDEX IF NOT EXISTS emails_suggested_job_id_idx
    ON public.emails (suggested_job_id) WHERE suggested_job_id IS NOT NULL;
