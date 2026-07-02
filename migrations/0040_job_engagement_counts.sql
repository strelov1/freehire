-- Materialized engagement counters on jobs: the number of distinct signed-in
-- users who have viewed a job and who have marked it applied. Incremented in the
-- tracking write path (RecordJobView / MarkJobApplied) on the first-time
-- transition only, so refreshes and repeat actions never inflate them; served
-- straight off the row by jobview.FromRow with no read-time COUNT.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS view_count    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS applied_count INTEGER NOT NULL DEFAULT 0;

-- Backfill from existing interactions. Every user_jobs row has viewed_at set
-- (NOT NULL DEFAULT now()), so its row count is the distinct-viewer count;
-- applied_count filters on applied_at.
UPDATE jobs j SET
    view_count    = sub.views,
    applied_count = sub.applied
FROM (
    SELECT job_id,
           count(*)                                       AS views,
           count(*) FILTER (WHERE applied_at IS NOT NULL) AS applied
    FROM user_jobs
    GROUP BY job_id
) sub
WHERE sub.job_id = j.id;
