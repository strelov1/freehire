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
