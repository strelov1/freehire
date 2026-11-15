-- migrate: no-transaction
-- The application URL is also a posting identity for extension navigation. Build
-- concurrently: the catalogue is large, but only known destinations enter this index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_normalized_application_url_idx
    ON public.jobs (public.normalize_job_url(application_url))
    WHERE application_url <> '';
