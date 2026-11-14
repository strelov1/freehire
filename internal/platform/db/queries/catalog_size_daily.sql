-- name: RecordCatalogSize :exec
-- Store one day's measured catalogue size. Replaces whatever the day held, a reconstructed
-- estimate included: a measurement always beats a reconstruction.
INSERT INTO catalog_size_daily (day, open_jobs, reconstructed, computed_at)
VALUES (sqlc.arg(day), sqlc.arg(open_jobs), false, sqlc.arg(computed_at))
ON CONFLICT (day) DO UPDATE
SET open_jobs = EXCLUDED.open_jobs, reconstructed = false, computed_at = EXCLUDED.computed_at;

-- name: RecordReconstructedCatalogSize :exec
-- Fill days the catalogue was never measured on. A day already holding a row keeps it: a
-- measured day must not be overwritten by an estimate, and a rerun of the backfill must
-- not move the past it already published.
INSERT INTO catalog_size_daily (day, open_jobs, reconstructed)
SELECT unnest(sqlc.arg(days)::date[]), unnest(sqlc.arg(open_jobs)::bigint[]), true
ON CONFLICT (day) DO NOTHING;

-- name: ListCatalogSize :many
-- The whole catalogue-size series, oldest first: one row a day, so it stays small. Days with
-- no row are absent, not zero.
SELECT day, open_jobs, reconstructed
FROM catalog_size_daily
ORDER BY day;

-- name: ListedJobDailyFlow :many
-- Per UTC day, how many listed postings (catalogstats' predicate minus closed_at) were
-- created and how many closed, for the one-off history reconstruction. A full scan: run it
-- on the analytics replica, never on a request path.
SELECT
    (COALESCE(a.day, r.day))::date AS day,
    COALESCE(a.n, 0)::bigint AS added,
    COALESCE(r.n, 0)::bigint AS removed
FROM (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs WHERE duplicate_of IS NULL AND NOT is_private
    GROUP BY 1
) a
FULL OUTER JOIN (
    SELECT (closed_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs WHERE closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private
    GROUP BY 1
) r ON a.day = r.day
ORDER BY 1;

-- name: FirstJobIDPerDay :many
-- The smallest job id created on each UTC day. Ids are allocated in creation order, so this
-- dates a row that no longer carries created_at (pruned_jobs) by the day its id falls in.
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, min(id)::bigint AS first_id
FROM jobs
GROUP BY 1
ORDER BY 1;

-- name: PrunedJobIDs :many
-- Every pruned posting's id and the day it was deleted, for the history reconstruction.
SELECT id, (pruned_at AT TIME ZONE 'UTC')::date AS pruned_day
FROM pruned_jobs;
