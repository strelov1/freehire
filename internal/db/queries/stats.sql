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

-- name: ListUserGrowth :many
-- Dense cumulative member-growth series: one UTC calendar day per row from the
-- first registration through today, each carrying the running total of members
-- registered on or before that day. A daily generate_series builds the gap-free
-- calendar (days with no new signups repeat the previous total), the LEFT JOIN
-- attaches each day's new-signup count, and the window SUM makes it cumulative, so
-- the series is monotonically non-decreasing. Aggregate only — no user identifier,
-- email, or other personal field is selected. With no members the series is empty
-- (min(day) is NULL, so generate_series yields no rows).
WITH daily AS (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM users
    GROUP BY 1
)
SELECT
    d::date AS day,
    sum(COALESCE(daily.n, 0)) OVER (ORDER BY d)::int AS total
FROM generate_series(
    (SELECT min(day) FROM daily),
    (now() AT TIME ZONE 'UTC')::date,
    interval '1 day'
) AS d
LEFT JOIN daily ON daily.day = d::date
ORDER BY d;

-- name: GetEngagementStats :one
-- Aggregate interaction counts from user_jobs — saved / applied / viewed — for the
-- public engagement endpoint. Aggregate-only: no user identifier or row-level field
-- is selected. viewed_at is NOT NULL on every row (set on RecordView), so "viewed"
-- equals the total interaction count.
SELECT
    count(*) FILTER (WHERE saved_at IS NOT NULL)::int   AS saved,
    count(*) FILTER (WHERE applied_at IS NOT NULL)::int AS applied,
    count(*) FILTER (WHERE viewed_at IS NOT NULL)::int  AS viewed
FROM user_jobs;

-- name: ListJobActivity :many
-- Dense activity series over [from, to] at the given granularity. A daily
-- generate_series builds the gap-free calendar; the LEFT JOIN fills each day's
-- counts (missing days → 0), and date_trunc(unit, ...) rolls those days up to the
-- requested bucket (day/week/month) so empty buckets still appear as zeros. `unit`
-- is a caller-validated date_trunc field (day/week/month), never raw user input.
SELECT
    date_trunc(sqlc.arg('unit')::text, d)::date AS period,
    COALESCE(sum(s.added), 0)::int   AS added,
    COALESCE(sum(s.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN job_daily_stats s ON s.day = d::date
GROUP BY date_trunc(sqlc.arg('unit')::text, d)
ORDER BY date_trunc(sqlc.arg('unit')::text, d);
