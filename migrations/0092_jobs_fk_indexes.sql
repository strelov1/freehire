-- Mirrors 0043_jobs_user_fk_idx.sql, but for FKs into jobs instead of users. cmd/prune
-- (internal/db/queries/pruning.sql) is the only hard-delete path for jobs, and every
-- table below references jobs.id ON DELETE CASCADE/SET NULL with no index whose leading
-- column is the referencing one, so each row cmd/prune deletes runs one sequential scan
-- per such table for the referential-integrity query.
--
-- user_jobs.job_id is the case the review flagged directly: it carries two indexes
-- (0040, 0055) but both are partial on an unrelated column (vote, applied_at), so
-- neither covers the rows excluded by that predicate. job_reminders.job_id (0034),
-- ghost_reports.job_id (0053), and application_nudges.job_id (0083) have the same
-- shape — a leading-column index that is partial on status/retracted_at rather than on
-- job_id itself — so the RI query still scans for the rows the predicate excludes
-- (delivered/cancelled reminders, retracted ghost reports).
--
-- Same nullability split as 0043: NOT NULL/CASCADE columns get a plain index; nullable
-- SET NULL columns get a partial index on IS NOT NULL, which is exactly the set an RI
-- query can match and keeps the index near-empty on the columns that are barely populated.
-- A partial index cannot back a foreign-key CONSTRAINT (it isn't the referenced side of
-- one here — these all sit on the referencing side), and general Postgres guidance is
-- mixed on whether the planner always picks a partial index for an RI trigger's scan. This
-- codebase already has the answer for this exact shape: 0043 shipped WHERE ... IS NOT NULL
-- indexes to fix a real production DELETE-FROM-users timeout, and that fix held — if the
-- planner were not using them, the timeout would have persisted. Mirrored here rather than
-- switched to full indexes, which would double their size for coverage the IS NOT NULL
-- predicate already logically guarantees (job_id = $1 for any real $1 implies NOT NULL).
--
-- On a fresh initdb volume these plain CREATE INDEX statements are fine; on the live
-- prod DB they must be applied manually as CREATE INDEX CONCURRENTLY (this migration
-- runner wraps each file in a transaction, which CONCURRENTLY cannot run inside).

CREATE INDEX job_reports_job_id_idx ON public.job_reports (job_id);
CREATE INDEX subscription_matches_job_id_idx ON public.subscription_matches (job_id);
CREATE INDEX user_jobs_job_id_idx ON public.user_jobs (job_id);
CREATE INDEX user_job_analysis_job_id_idx ON public.user_job_analysis (job_id);
CREATE INDEX job_reminders_job_id_idx ON public.job_reminders (job_id);
CREATE INDEX application_nudges_job_id_idx ON public.application_nudges (job_id);
CREATE INDEX ghost_reports_job_id_idx ON public.ghost_reports (job_id);

CREATE INDEX job_submissions_job_id_idx ON public.job_submissions (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX cvs_job_id_idx ON public.cvs (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX referral_requests_job_id_idx ON public.referral_requests (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX assistant_sessions_job_id_idx ON public.assistant_sessions (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX applications_job_id_idx ON public.applications (job_id) WHERE job_id IS NOT NULL;
CREATE INDEX application_events_job_id_idx ON public.application_events (job_id) WHERE job_id IS NOT NULL;
