-- name: RecordEmployerVacancyApplyClick :exec
INSERT INTO public.employer_vacancy_apply_clicks AS daily (job_id, day, clicks)
SELECT id, (now() AT TIME ZONE 'UTC')::date, 1
FROM public.jobs_read
WHERE public_slug = sqlc.arg(public_slug)
  AND source = 'employer' AND created_by IS NOT NULL AND NOT is_private
  AND closed_at IS NULL
ON CONFLICT (job_id, day) DO UPDATE
SET clicks = daily.clicks + EXCLUDED.clicks;

-- name: GetEmployerVacancyAnalytics :one
-- Use one UTC window for all three counts. The view rollup processes completed days,
-- and its (day, job_id) primary key supports one point lookup per day.
WITH report_window AS (
    SELECT (now() AT TIME ZONE 'UTC')::date AS today
)
SELECT
    (SELECT coalesce(sum((
         SELECT v.page_uniques FROM public.job_daily_views v
         WHERE v.day = w.today - period.days_ago AND v.job_id = j.id
     )), 0)::bigint FROM generate_series(1, 30) AS period(days_ago)) AS views,
    (SELECT coalesce(sum(c.clicks), 0)::bigint FROM public.employer_vacancy_apply_clicks c
     WHERE c.job_id = j.id AND c.day >= w.today - 30 AND c.day < w.today) AS apply_clicks,
    (SELECT count(*) FROM public.direct_applications a
     WHERE a.job_id = j.id
       AND a.sent_at >= ((w.today - 30)::timestamp AT TIME ZONE 'UTC')
       AND a.sent_at < (w.today::timestamp AT TIME ZONE 'UTC')) AS applications
FROM public.jobs j
CROSS JOIN report_window w
WHERE j.public_slug = sqlc.arg(public_slug)
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(actor_id)::bigint));

-- name: GetEmployerOwnedJobID :one
-- The job id behind a public slug, only for an employer vacancy the actor's active team owns:
-- the same guard as GetEmployerVacancyAnalytics, so a stranger's slug is indistinguishable
-- from a missing one.
SELECT j.id FROM public.jobs j
WHERE j.public_slug = sqlc.arg(public_slug)
  AND j.source = 'employer' AND NOT j.is_private
  AND j.created_by IN (SELECT public.active_employer_team(sqlc.arg(actor_id)::bigint));

-- name: GetJobOutboundClickStats :many
-- One job's human outbound clicks in [from_at, to_at), as rows of (dim, key, clicks, visitors).
-- Bots are stored flagged and excluded here. Visitors are counted per UTC day (one per
-- visitor_hash), and only on the day rows; a click with no hash (no salt configured) counts as a
-- click but not as a visitor.
WITH c AS (
    SELECT (clicked_at AT TIME ZONE 'UTC')::date AS day, src, country, visitor_hash
    FROM public.job_outbound_clicks
    WHERE job_id = sqlc.arg(job_id) AND NOT is_likely_bot
      AND clicked_at >= sqlc.arg(from_at) AND clicked_at < sqlc.arg(to_at)
)
SELECT dim, key, clicks, visitors FROM (
    SELECT 'day'::text AS dim, to_char(day, 'YYYY-MM-DD') AS key, count(*)::bigint AS clicks,
           count(DISTINCT NULLIF(visitor_hash, ''))::bigint AS visitors
    FROM c GROUP BY day
    UNION ALL
    SELECT 'country', country, count(*), 0 FROM c GROUP BY country
    UNION ALL
    SELECT 'src', src, count(*), 0 FROM c GROUP BY src
) r
ORDER BY dim, CASE WHEN dim = 'day' THEN 0 ELSE clicks END DESC, key;
