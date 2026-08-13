-- A multi-job subscription digest (Total > 1) has no single job to point
-- public_slug at, but the person still wants to see WHICH jobs triggered it —
-- a snapshot as of delivery, not a live re-run of the saved search (results
-- drift; a job in this list may since have closed or dropped out of a re-run).
-- `jobs` holds a small JSON array of {title, company, slug} (same shape as
-- DigestJob, no internal id — see 0090's own comment for why), NULL for every
-- other kind, which always resolves via public_slug instead.

ALTER TABLE public.user_notifications
    ADD COLUMN jobs jsonb;
