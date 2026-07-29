-- Match a job page even when the posting on it is a duplicate.
--
-- 0042 indexed normalize_job_url(url) over OPEN CANONICAL rows, which is what a listing
-- wants: one row per dedup group. The extension asks a different question — "what is the
-- page in front of me?" — and there the answer is the vacancy, whichever row of the group
-- carries the URL the candidate is standing on. One in five open postings is a duplicate,
-- so the narrower index made every fifth page report that freehire does not have it.
--
-- This index drops `duplicate_of IS NULL` and keeps `closed_at IS NULL`; the query then
-- follows duplicate_of to the canonical row. The 0042 index stays: it is narrower and
-- still the right one for lookups that only want canonical rows.
--
-- Applied to a fresh volume by initdb after 0050. On the live database create it
-- CONCURRENTLY instead, so the build takes no write lock on a 3.3M-row table:
--
--   CREATE INDEX CONCURRENTLY jobs_normalized_url_open_idx
--       ON public.jobs (public.normalize_job_url(url)) WHERE closed_at IS NULL;
--
-- Run that from a session that survives a dropped connection (nohup/screen): a
-- CONCURRENTLY build killed midway leaves an INVALID index behind, which has to be
-- dropped before retrying.
CREATE INDEX jobs_normalized_url_open_idx
    ON public.jobs (public.normalize_job_url(url))
    WHERE (closed_at IS NULL);
