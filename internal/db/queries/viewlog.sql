-- View-log aggregation queries, used by cmd/rollup-views. The worker parses nginx
-- access logs off the request path (see internal/viewlog), then resolves slugs and
-- applies per-(day, job) unique counts here.

-- name: ResolveSlugsToJobIDs :many
-- Map public slugs to job ids. Unknown slugs are simply absent from the result, so
-- the worker skips views for jobs that no longer exist.
SELECT id, public_slug
FROM jobs
WHERE public_slug = ANY(sqlc.arg('slugs')::text[]);

-- name: ApplyDailyView :batchexec
-- Apply one (day, job) unique count additively: upsert the daily rollup and add the
-- same delta to jobs.view_count, in one statement. The data-modifying CTE runs even
-- though the primary query does not read it. Issued as a pgx batch (one call per
-- tuple) so a file's rows land in a single round trip; view_count accumulates across
-- a job's day-rows, and additivity lets a day spanning two rotated files sum right.
WITH ins AS (
    INSERT INTO job_daily_views (day, job_id, uniques)
    VALUES (sqlc.arg('day'), sqlc.arg('job_id'), sqlc.arg('delta'))
    ON CONFLICT (day, job_id)
        DO UPDATE SET uniques = job_daily_views.uniques + EXCLUDED.uniques
)
UPDATE jobs SET view_count = view_count + sqlc.arg('delta')
WHERE id = sqlc.arg('job_id');

-- name: IsViewLogFileProcessed :one
-- Cursor read: has this rotated file (by content signature) been applied? The
-- signature is stable across rename and gzip, so a re-run recognizes the same file.
SELECT EXISTS(
    SELECT 1 FROM processed_view_logs WHERE signature = sqlc.arg('signature')
);

-- name: MarkViewLogFileProcessed :exec
-- Cursor write: mark a rotated file applied. Idempotent — a concurrent/rerun mark
-- is a no-op, so the file is never double-applied.
INSERT INTO processed_view_logs (signature, filename)
VALUES (sqlc.arg('signature'), sqlc.arg('filename'))
ON CONFLICT (signature) DO NOTHING;
