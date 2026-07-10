-- name: DeleteAllJobDailyStats :exec
-- First half of the atomic rebuild: clear the rollup. Run in the same transaction
-- as RebuildJobDailyStats so readers never see an empty table and reopen-orphaned
-- days (a day that had only closures, now reopened) are dropped rather than left
-- stale.
DELETE FROM job_daily_stats;

-- name: RebuildJobDailyStats :execrows
-- Second half of the atomic rebuild: recompute every active day from jobs. `added`
-- counts jobs by their created_at day; `removed` counts jobs by their CURRENT
-- closed_at day (NULL = still open, excluded). Days are UTC calendar dates
-- (AT TIME ZONE 'UTC') so buckets are stable regardless of session timezone. The
-- FULL OUTER JOIN yields one row per day that saw either an add or a removal.
INSERT INTO job_daily_stats (day, added, removed, computed_at)
SELECT
    COALESCE(a.day, r.day)  AS day,
    COALESCE(a.n, 0)::int   AS added,
    COALESCE(r.n, 0)::int   AS removed,
    now()
FROM (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs
    GROUP BY 1
) a
FULL OUTER JOIN (
    SELECT (closed_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs
    WHERE closed_at IS NOT NULL
    GROUP BY 1
) r ON a.day = r.day;

-- name: ListJobActivityDay :many
-- Dense daily series over [from, to]: generate_series makes the gap-free calendar,
-- LEFT JOIN fills each day's counts (missing days → 0). $1 = from, $2 = to.
SELECT
    d::date                 AS period,
    COALESCE(s.added, 0)::int   AS added,
    COALESCE(s.removed, 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN job_daily_stats s ON s.day = d::date
ORDER BY d;

-- name: ListJobActivityWeek :many
-- Dense weekly series: the same daily generate_series grouped to ISO weeks, so
-- every week overlapping [from, to] appears with its summed counts (empty → 0).
SELECT
    date_trunc('week', d)::date AS period,
    COALESCE(sum(s.added), 0)::int   AS added,
    COALESCE(sum(s.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN job_daily_stats s ON s.day = d::date
GROUP BY date_trunc('week', d)
ORDER BY date_trunc('week', d);

-- name: ListJobActivityMonth :many
-- Dense monthly series: the same daily generate_series grouped to calendar months.
SELECT
    date_trunc('month', d)::date AS period,
    COALESCE(sum(s.added), 0)::int   AS added,
    COALESCE(sum(s.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN job_daily_stats s ON s.day = d::date
GROUP BY date_trunc('month', d)
ORDER BY date_trunc('month', d);
