-- View-log aggregation queries, used by cmd/rollup-views. The worker parses nginx
-- access logs off the request path (see internal/application/viewlog), then resolves slugs and
-- applies per-(day, job) unique counts here.

-- name: ResolveSlugsToJobIDs :many
-- Map public slugs to job ids. Unknown slugs are simply absent from the result, so
-- the worker skips views for jobs that no longer exist. The assistant's
-- `present_jobs` tool uses it for the same absence: a slug the model invented is
-- missing here, and is dropped from the deck rather than shown to the user.
SELECT id, public_slug
FROM jobs
WHERE public_slug = ANY(sqlc.arg('slugs')::text[]);

-- name: ApplyDailyView :batchexec
-- Apply one (day, job) unique count additively: upsert the daily rollup and add the
-- total delta to jobs.view_count, in one statement. The data-modifying CTE runs even
-- though the primary query does not read it. Issued as a pgx batch (one call per
-- tuple) so a file's rows land in a single round trip; view_count accumulates across
-- a job's day-rows, and additivity lets a day spanning two rotated files sum right.
--
-- Two deltas, not one plus a breakdown. `total_delta` counts the visitors who
-- produced EITHER signal and is what `uniques` has always held — jobs.view_count and
-- GET /api/v1/stats/catalog both read from it, so it must not move. `page_delta`
-- counts the visitors who opened the PAGE, the only bot-filtered signal of the two.
-- A visitor who did both is one visitor in each, so the two do not sum with an API
-- count; the only relation between them is page_delta <= total_delta.
WITH ins AS (
    INSERT INTO job_daily_views (day, job_id, uniques, page_uniques)
    VALUES (sqlc.arg('day'), sqlc.arg('job_id'), sqlc.arg('total_delta'), sqlc.arg('page_delta'))
    ON CONFLICT (day, job_id)
        DO UPDATE SET uniques      = job_daily_views.uniques + EXCLUDED.uniques,
                      page_uniques = job_daily_views.page_uniques + EXCLUDED.page_uniques
)
UPDATE jobs SET view_count = view_count + sqlc.arg('total_delta')
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
