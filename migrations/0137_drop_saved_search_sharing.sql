-- Public saved-search sharing ("boards", GET /api/v1/boards/:slug and the web /b/:slug
-- route) is retired in favor of job_lists (0135/0136), which share a fixed set of
-- specific jobs instead of a live query. See the
-- replace-board-sharing-with-collections change. Existing public board links simply
-- stop resolving; there is no automated migration into a job list, since a query is
-- not a set of jobs.

-- squawk-ignore require-concurrent-index-deletion -- saved_searches is small and low-traffic (per-user rows, not a crawl/search hot path); the momentary lock is not worth a no-transaction file
DROP INDEX public.saved_searches_public_slug_idx;

ALTER TABLE public.saved_searches
    -- squawk-ignore ban-drop-column -- the point of this migration: board sharing is retired and this column's only reader was that feature
    DROP COLUMN public_slug,
    -- squawk-ignore ban-drop-column -- same as public_slug above
    DROP COLUMN author_label;
