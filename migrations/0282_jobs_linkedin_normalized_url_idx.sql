-- migrate: no-transaction
-- Legacy LinkedIn identities are reused by URL even after closure. The existing
-- normalized URL indexes cover open rows only; keep this small source-specific
-- index available for closed history too, ordered like FindLinkedInJobByURL.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_linkedin_normalized_url_idx
    ON public.jobs (public.normalize_job_url(url), (duplicate_of IS NOT NULL), id)
    WHERE source = 'linkedin' AND NOT is_private;
