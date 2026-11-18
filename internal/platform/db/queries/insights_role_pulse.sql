-- ---------------------------------------------------------------------------
-- Role pulse (competition, time to close, inflow per category)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsRolePulse :exec
DELETE FROM insights_role_pulse;

-- name: ComputeInsightsRolePulse :many
-- Compute on the read connection; publish the result separately on the writer.
-- Three figures per category over canonical, public postings, each for the last 30 days
-- and the 30 before (openspec/changes/insights-role-pulse/design.md has the prod
-- measurements behind every choice below).
-- COMPETITION is page views over exposure. Views are job_daily_views.page_uniques, never
-- uniques (AGENTS.md "View counts"). Exposure is the posting-days a posting was OPEN ON
-- FREEHIRE inside the window: from created_at, not from LEAST(posted_at, created_at) —
-- the one figure in this file where first-seen is the right date, because nobody can view
-- a posting here before we found it, and the existence rule would charge a backlog posting
-- for weeks it was not on the site. A view on a day the posting was already closed is not
-- counted, so numerator and denominator are the same population. Both windows end on the
-- freshest COMPLETE views day (before today, UTC) rather than on now(): cmd/rollup-views
-- usually lands a day after it ends, and a now()-anchored window would set 29 days of
-- views against 30 of exposure.
-- TIME TO CLOSE is the median days from existence to close over closes that describe the
-- POSTING: unseen, feed_removed, probe_expired, employer_closed. Not expired (a 45-day age
-- rule — its median is 45 by construction), feed_empty or board_unreachable (our crawler's
-- health), board_retired, moderated or source_misattributed (our own decisions), or ''
-- (closed before reasons were recorded). A close dated before existence is left out, as
-- ComputeInsightsCompanyLifetime does.
-- INFLOW counts postings whose existence date, LEAST(posted_at, created_at) (insights.sql's
-- header), falls in the last 30 days — so a posting published before the window but found
-- during it is not "new" — and deliberately has NO earlier
-- window. Measured 2026-10-30, the comparison was dominated by our own coverage growth:
-- a board first crawled this month shows its earlier postings only if they were still
-- open when we found it, so healthcare read +155% while stable-coverage categories
-- (devops, data_engineering) moved -1% and -20%; neither a stable-employer panel nor a
-- discovered-within-N-days rule removed it (the latter made healthcare +392%).
WITH anchor AS (


    SELECT coalesce(max(day), (sqlc.arg('as_of')::timestamptz AT TIME ZONE 'UTC')::date - 1) AS d
    FROM job_daily_views
    WHERE day < (sqlc.arg('as_of')::timestamptz AT TIME ZONE 'UTC')::date
), w AS (
    SELECT
        d,
        (d - 29)::timestamp AT TIME ZONE 'UTC' AS cur_start,
        (d + 1)::timestamp  AT TIME ZONE 'UTC' AS cur_end,
        (d - 59)::timestamp AT TIME ZONE 'UTC' AS prev_start
    FROM anchor
), p AS (
    SELECT
        j.id,
        j.category,
        j.created_at,
        j.closed_at,
        LEAST(j.posted_at, j.created_at) AS existed_at,
        (j.closed_reason IN ('unseen', 'feed_removed', 'probe_expired', 'employer_closed')
            AND j.closed_at >= LEAST(j.posted_at, j.created_at)) AS counted_close
    FROM jobs j, w
    WHERE j.category <> '' AND j.duplicate_of IS NULL AND NOT j.is_private
      AND (j.closed_at IS NULL OR j.closed_at > LEAST(sqlc.arg('prior_start')::timestamptz, w.prev_start))
), postings AS (
    SELECT
        p.category,
        sum(GREATEST(0, EXTRACT(EPOCH FROM (LEAST(coalesce(p.closed_at, sqlc.arg('as_of')::timestamptz), w.cur_end) - GREATEST(p.created_at, w.cur_start))) / 86400.0))::float8 AS exposure_days,
        sum(GREATEST(0, EXTRACT(EPOCH FROM (LEAST(coalesce(p.closed_at, sqlc.arg('as_of')::timestamptz), w.cur_start) - GREATEST(p.created_at, w.prev_start))) / 86400.0))::float8 AS exposure_days_prev,
        count(*) FILTER (WHERE p.counted_close AND p.closed_at > sqlc.arg('window_start')::timestamptz) AS closed,
        count(*) FILTER (WHERE p.counted_close AND p.closed_at > sqlc.arg('prior_start')::timestamptz AND p.closed_at <= sqlc.arg('window_start')::timestamptz) AS closed_prev,
        (percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (p.closed_at - p.existed_at)) / 86400.0)
            FILTER (WHERE p.counted_close AND p.closed_at > sqlc.arg('window_start')::timestamptz))::real AS median_close_days,
        (percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (p.closed_at - p.existed_at)) / 86400.0)
            FILTER (WHERE p.counted_close AND p.closed_at > sqlc.arg('prior_start')::timestamptz AND p.closed_at <= sqlc.arg('window_start')::timestamptz))::real AS median_close_days_prev,
        count(*) FILTER (WHERE p.existed_at > sqlc.arg('window_start')::timestamptz) AS inflow
    FROM p, w
    GROUP BY p.category
), views AS (
    SELECT
        p.category,
        sum(v.page_uniques) FILTER (WHERE v.day > w.d - 30) AS views,
        sum(v.page_uniques) FILTER (WHERE v.day <= w.d - 30) AS views_prev
    FROM job_daily_views v
    JOIN p ON p.id = v.job_id
    CROSS JOIN w
    WHERE v.day > w.d - 60 AND v.day <= w.d
      AND (p.closed_at IS NULL OR v.day <= (p.closed_at AT TIME ZONE 'UTC')::date)
    GROUP BY p.category
)

SELECT
    (s.category)::text AS category,
    (w.d)::date AS views_through,
    (coalesce(v.views, 0))::bigint AS views,
    (coalesce(v.views_prev, 0))::bigint AS views_prev,
    (s.exposure_days)::double precision AS exposure_days,
    (s.exposure_days_prev)::double precision AS exposure_days_prev,
    (s.closed)::bigint AS closed,
    (s.closed_prev)::bigint AS closed_prev,
    coalesce((s.median_close_days)::real, 0::real)::real AS median_close_days,
    coalesce((s.median_close_days_prev)::real, 0::real)::real AS median_close_days_prev,
    (s.inflow)::bigint AS inflow,
    ((s.median_close_days) IS NOT NULL)::boolean AS median_close_days_valid,
    ((s.median_close_days_prev) IS NOT NULL)::boolean AS median_close_days_prev_valid
FROM postings s
CROSS JOIN w
LEFT JOIN views v ON v.category = s.category;

-- name: GetInsightsRolePulse :one
-- One category's pulse. No row means the category had nothing open or closed in either
-- window; the caller serves zeros.
SELECT views_through, views, views_prev, exposure_days, exposure_days_prev,
       closed, closed_prev, median_close_days, median_close_days_prev, inflow
FROM insights_role_pulse
WHERE category = sqlc.arg('category')::text;
