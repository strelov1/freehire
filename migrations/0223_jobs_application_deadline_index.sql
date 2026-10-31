-- migrate: no-transaction
-- The deadline sweep reads only open postings with a source-stated deadline.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_application_deadline_idx
    ON public.jobs (application_deadline)
    WHERE application_deadline IS NOT NULL AND closed_at IS NULL;
