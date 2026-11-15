-- Trends & Insights rollups (insights_*), recomputed by cmd/rollup-stats as an
-- atomic delete-and-reinsert, and the read queries the public /api/v1/insights/*
-- endpoints serve from them. All rollups are a pure function of current `jobs`
-- state; @prev_ts (the growth-window start) and @min_sample are supplied by the
-- worker so the window and sample floor stay out of the SQL and are test-injectable.
--
-- Every prior-window count asks whether a posting EXISTED at @prev_ts, and a posting
-- exists from LEAST(posted_at, created_at) — never from created_at alone. created_at
-- is when freehire FOUND it, so on first-seen alone a board onboarded inside the
-- window publishes its whole backlog as growth: measured 2026-10-30, 387,324 of the
-- 876,973 open healthcare postings first seen in the last 30 days had been published
-- before that window. LEAST skips a NULL posted_at (falling back to first-seen) and
-- cannot let a posted_at re-stamped on repost push existence later than first-seen.
-- A test in this package fails if a bare `created_at <=` prior-window count returns.

-- ---------------------------------------------------------------------------
-- Role demand
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsRoleStats :exec
DELETE FROM insights_role_stats;

-- name: ComputeInsightsRoleStatsGlobal :many
-- Compute on the read connection; publish the result separately on the writer.
-- Country-agnostic ('' bucket) role demand. open_count = jobs open now
-- (closed_at IS NULL); open_count_prev = jobs open as of @prev_ts. The inner
-- aggregate is wrapped so the "non-zero in either window" filter names the counts
-- once instead of repeating the FILTER expressions in a HAVING.
SELECT
    (category)::text AS category,
    (seniority)::text AS seniority,
    ('')::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        category,
        seniority,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs
    WHERE category <> '' AND seniority <> ''
    GROUP BY category, seniority
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ComputeInsightsRoleStatsByCountry :many
-- Compute on the read connection; publish the result separately on the writer.
-- Per-country role demand: a job contributes once to each of its countries.
SELECT
    (category)::text AS category,
    (seniority)::text AS seniority,
    (country)::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        category,
        seniority,
        country,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(countries) AS country
    WHERE category <> '' AND seniority <> ''
    GROUP BY category, seniority, country
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ListInsightsRoles :many
-- Ranked roles within one country slice ('' = all countries), ordered by raw
-- demand or by growth (open_count - open_count_prev), demand as the tiebreak.
-- An empty @category means all categories (the original behavior); a non-empty
-- @category restricts the ranking to that category's seniorities, and a non-empty
-- @seniority narrows it to ONE role. Until @seniority existed the parameter was not
-- read at all, so a caller who sent it was answered with every seniority — the
-- dropped-filter-widens-the-answer trap in its silent form, and the endpoint has no
-- meta.ignored_params to have reported it.
SELECT category, seniority, open_count, (open_count - open_count_prev)::int AS growth
FROM insights_role_stats
WHERE country = sqlc.arg('country')
  AND (sqlc.arg('category')::text = '' OR category = sqlc.arg('category'))
  AND (sqlc.arg('seniority')::text = '' OR seniority = sqlc.arg('seniority'))
ORDER BY
    (CASE WHEN sqlc.arg('sort')::text = 'growth' THEN (open_count - open_count_prev) ELSE open_count END) DESC,
    open_count DESC
LIMIT sqlc.arg('lim')::int;

-- ---------------------------------------------------------------------------
-- Role × country grid
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsRoleCountryStats :exec
DELETE FROM insights_role_country_stats;

-- name: ComputeInsightsRoleCountryStatsGlobal :many
-- Compute on the read connection; publish the result separately on the writer.
-- The all-countries column ('' bucket): one count per posting, however many countries
-- it lists — grouping by category alone is what keeps a two-country posting from
-- counting twice here. Unlike insights_role_stats it needs no seniority and counts
-- only canonical, public postings (see migration 0214 for both).
SELECT
    (category)::text AS category,
    ('')::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        category,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs
    WHERE category <> '' AND duplicate_of IS NULL AND NOT is_private
    GROUP BY category
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ComputeInsightsRoleCountryStatsByCountry :many
-- Compute on the read connection; publish the result separately on the writer.
-- One column per country: a posting contributes once to each country it lists, and a
-- posting with none reaches the '' bucket only.
SELECT
    (category)::text AS category,
    (country)::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        category,
        country,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(countries) AS country
    WHERE category <> '' AND duplicate_of IS NULL AND NOT is_private
    GROUP BY category, country
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ListInsightsRoleCountryColumns :many
-- The grid's columns: the countries holding the most open postings, largest first.
-- Summing a country's cells counts each posting once — it has exactly one category.
SELECT country
FROM insights_role_country_stats
WHERE country <> ''
GROUP BY country
ORDER BY sum(open_count) DESC, country
LIMIT sqlc.arg('lim')::int;

-- name: ListInsightsRoleCountryCells :many
-- Every cell of the all-countries column plus every cell of the named countries.
SELECT category, country, open_count, open_count_prev
FROM insights_role_country_stats
WHERE country = '' OR country = ANY(sqlc.arg('countries')::text[])
ORDER BY category, country;

-- name: GetInsightsRoleOpenCount :one
-- One category's open count in the all-countries cell: every open canonical public
-- posting of the category, whatever its seniority. No row means nothing open at either
-- point; the caller serves zero.
SELECT open_count
FROM insights_role_country_stats
WHERE category = sqlc.arg('category')::text AND country = '';

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
-- INFLOW counts postings whose existence date falls in the last 30 days — the header's
-- rule, applied to "new" instead of "open at the start" — and deliberately has NO earlier
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

-- ---------------------------------------------------------------------------
-- Per-role skill demand
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsRoleSkillStats :exec
DELETE FROM insights_role_skill_stats;

-- name: ComputeInsightsRoleSkillStats :many
-- Compute on the read connection; publish the result separately on the writer.
-- The skill distribution WITHIN one role. Same shape as
-- ComputeInsightsRoleStatsByCountry's `FROM jobs, unnest(countries)` above — a job
-- contributes once per skill it carries — so its cost is one already measured in
-- production, and `skills` is a small text[] rather than the TOASTed description.
-- Open postings only: a closed posting's skills describe a vacancy nobody can apply
-- to. No growth column, unlike the sibling rollups: "docker is up 4% within senior
-- backend" is a second question, and answering it would need a prior-window pass
-- over the same unnest.
SELECT
    (category)::text AS category,
    (seniority)::text AS seniority,
    (skill)::text AS skill,
    (count(*)::int)::integer AS open_count
FROM jobs, unnest(skills) AS skill
WHERE closed_at IS NULL
  AND NOT is_private
  AND category <> '' AND seniority <> ''
GROUP BY category, seniority, skill
HAVING count(*) >= sqlc.arg('min_sample')::int;

-- name: DeleteAllInsightsRoleSkillSample :exec
DELETE FROM insights_role_skill_sample;

-- name: ComputeInsightsRoleSkillSample :many
-- Compute on the read connection; publish the result separately on the writer.
-- The denominator every share in insights_role_skill_stats divides by: the role's
-- open postings that carry AT LEAST ONE tagged skill. Measured 2026-09-18, 11% of
-- the eligible postings carry none, so dividing by the role's open count would fold
-- our own tagging gap into every published share — and because that gap differs per
-- role, two roles' shares would stop being comparable.
-- Takes NO @min_sample, deliberately. The floor selects which SKILLS are published;
-- applied here it would delete the denominator of a share that did clear the floor,
-- leaving a numerator with nothing to divide by.
-- The NOT is_private clause must stay in step with the distribution's: a numerator
-- and a denominator counting different populations is a share that is quietly wrong
-- rather than visibly broken.
SELECT
    (category)::text AS category,
    (seniority)::text AS seniority,
    (count(*)::int)::integer AS sample_size
FROM jobs
WHERE closed_at IS NULL
  AND NOT is_private
  AND category <> '' AND seniority <> ''
  AND cardinality(skills) > 0
GROUP BY category, seniority;

-- name: ListInsightsRoleSkills :many
-- One role's skills, most-demanded first. The caller divides by the role's
-- sample_size (GetInsightsRoleSkillSample), never by its open_count.
SELECT skill, open_count
FROM insights_role_skill_stats
WHERE category = sqlc.arg('category')::text
  AND seniority = sqlc.arg('seniority')::text
ORDER BY open_count DESC, skill
LIMIT sqlc.arg('lim')::int;

-- name: GetInsightsRoleSkillSample :one
-- One role's share denominator. A role with no skill-bearing postings has no row
-- here, and the caller MUST read that as a sample of zero rather than as an error:
-- it means the role exists and nothing in it was taggable, which is a real answer.
SELECT sample_size
FROM insights_role_skill_sample
WHERE category = sqlc.arg('category')::text
  AND seniority = sqlc.arg('seniority')::text;

-- ---------------------------------------------------------------------------
-- Skill demand
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsSkillStats :exec
DELETE FROM insights_skill_stats;

-- name: ComputeInsightsSkillStatsGlobal :many
-- Compute on the read connection; publish the result separately on the writer.
-- Country- and category-agnostic skill demand (both '' buckets).
SELECT
    (skill)::text AS skill,
    ('')::text AS category,
    ('')::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        skill,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(skills) AS skill
    GROUP BY skill
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ComputeInsightsSkillStatsByCategory :many
-- Compute on the read connection; publish the result separately on the writer.
-- Per-category skill demand (country '' bucket).
SELECT
    (skill)::text AS skill,
    (category)::text AS category,
    ('')::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        skill,
        category,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(skills) AS skill
    WHERE category <> ''
    GROUP BY skill, category
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ComputeInsightsSkillStatsByCountry :many
-- Compute on the read connection; publish the result separately on the writer.
-- Per-country skill demand (category '' bucket).
SELECT
    (skill)::text AS skill,
    ('')::text AS category,
    (country)::text AS country,
    (open_count)::integer AS open_count,
    (open_count_prev)::integer AS open_count_prev
FROM (
    SELECT
        skill,
        country,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(skills) AS skill, unnest(countries) AS country
    GROUP BY skill, country
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ListInsightsSkills :many
-- Ranked skills within one (category, country) scope; scoping is one-dimensional
-- (either category or country carries a value, the other is ''), matching what the
-- rollup materializes.
SELECT skill, open_count, (open_count - open_count_prev)::int AS growth
FROM insights_skill_stats
WHERE category = sqlc.arg('category') AND country = sqlc.arg('country')
ORDER BY
    (CASE WHEN sqlc.arg('sort')::text = 'growth' THEN (open_count - open_count_prev) ELSE open_count END) DESC,
    open_count DESC
LIMIT sqlc.arg('lim')::int;

-- ---------------------------------------------------------------------------
-- Skill demand history (the personal GET /me/market-pulse read)
-- ---------------------------------------------------------------------------

-- name: SnapshotInsightsSkillHistory :execrows
-- Appends this ISO week's global (category='', country='') skill open-counts
-- to the history table, reading the rollup this same transaction just
-- rebuilt rather than re-aggregating jobs. ON CONFLICT DO NOTHING is what
-- makes this safe to call on every intra-day rollup-stats run: only the
-- first run of a given week actually inserts, every later run this week is
-- a no-op.
INSERT INTO insights_skill_history (skill, week_start, open_count)
SELECT skill, sqlc.arg('week_start')::date, open_count
FROM insights_skill_stats
WHERE category = '' AND country = ''
ON CONFLICT (skill, week_start) DO NOTHING;

-- name: PruneInsightsSkillHistory :execrows
-- Drops snapshot rows older than the retention window (~26 weeks), keeping
-- the table bounded.
DELETE FROM insights_skill_history
WHERE week_start < sqlc.arg('cutoff')::date;

-- name: ListInsightsSkillHistory :many
-- All retained weekly snapshots for a set of skills (a caller's own profile
-- skills), newest first per skill.
SELECT skill, week_start, open_count
FROM insights_skill_history
WHERE skill = ANY(sqlc.arg('skills')::text[])
ORDER BY skill, week_start DESC;

-- ---------------------------------------------------------------------------
-- Salary bands
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsSalaryStats :exec
DELETE FROM insights_salary_stats;

-- name: ComputeInsightsSalaryStatsGlobal :many
-- Compute on the read connection; publish the result separately on the writer.
-- Country-agnostic salary bands. CUBE(category, seniority) emits every scope the
-- endpoint can ask for — the exact (category, seniority) band, the category-only
-- band (seniority ''), the seniority-only band (category ''), and the overall band
-- (both '') — all per (currency, period), which is never aggregated across. The
-- figure per job is its EFFECTIVE salary (jobs_effective_salary, migration 0216:
-- manual, else source, else enrichment — the min/max midpoint or whichever bound is
-- present). Only bands at/above @min_sample are stored.
SELECT
    (coalesce(category, ''))::text AS category,
    (coalesce(seniority, ''))::text AS seniority,
    ('')::text AS country,
    (currency)::text AS currency,
    (period)::text AS period,
    (count(*)::int)::integer AS sample_size,
    (percentile_cont(0.25) WITHIN GROUP (ORDER BY sal)::int)::integer AS p25,
    (percentile_cont(0.50) WITHIN GROUP (ORDER BY sal)::int)::integer AS p50,
    (percentile_cont(0.75) WITHIN GROUP (ORDER BY sal)::int)::integer AS p75,
    (percentile_cont(ARRAY[0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40, 0.45, 0.50,
                           0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95])
        WITHIN GROUP (ORDER BY sal))::integer[] AS quantiles
FROM (
    SELECT category, seniority, currency, period, amount AS sal
    FROM jobs_effective_salary
    WHERE closed_at IS NULL AND NOT is_private
      AND category <> '' AND seniority <> ''
) s
WHERE sal IS NOT NULL
GROUP BY currency, period, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int;

-- name: ComputeInsightsSalaryStatsByCountry :many
-- Compute on the read connection; publish the result separately on the writer.
-- Per-country salary bands (same CUBE of role scope, country from unnest).
SELECT
    (coalesce(category, ''))::text AS category,
    (coalesce(seniority, ''))::text AS seniority,
    (country)::text AS country,
    (currency)::text AS currency,
    (period)::text AS period,
    (count(*)::int)::integer AS sample_size,
    (percentile_cont(0.25) WITHIN GROUP (ORDER BY sal)::int)::integer AS p25,
    (percentile_cont(0.50) WITHIN GROUP (ORDER BY sal)::int)::integer AS p50,
    (percentile_cont(0.75) WITHIN GROUP (ORDER BY sal)::int)::integer AS p75,
    (percentile_cont(ARRAY[0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40, 0.45, 0.50,
                           0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95])
        WITHIN GROUP (ORDER BY sal))::integer[] AS quantiles
FROM (
    SELECT category, seniority, country, currency, period, amount AS sal
    FROM jobs_effective_salary, unnest(countries) AS country
    WHERE closed_at IS NULL AND NOT is_private
      AND category <> '' AND seniority <> ''
) s
WHERE sal IS NOT NULL
GROUP BY country, currency, period, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int;

-- name: DeleteAllInsightsSalaryMonthly :exec
DELETE FROM insights_salary_monthly;

-- name: ComputeInsightsSalaryMonthly :many
-- Compute on the read connection; publish the result separately on the writer.
-- The salary history: effective-salary percentiles per (currency, period, month a
-- posting was PUBLISHED), the current month and the eleven before it, for every role ×
-- grade scope — CUBE(category, seniority), '' meaning every value, so "all roles" and
-- "all grades" are rows of their own. Country-agnostic: the per-country history is
-- ComputeInsightsSalaryMonthlyByCountry, kept apart because unnesting countries would
-- count a posting once per country here. Open and closed postings alike — a posting
-- closed since still records what was offered that month — and canonical, public ones
-- only. A month a posting belongs to is its existence date (see the file header), so
-- one found in June but published in March counts in March. A posting without a grade
-- counts toward its role's all-grades row but has no grade row: grouping(seniority) = 0
-- with seniority '' would collide with the all-grades row. Months under @min_sample
-- are not stored: a line through a handful of postings a month is noise drawn as a trend.
SELECT
    (coalesce(category, ''))::text AS category,
    (coalesce(seniority, ''))::text AS seniority,
    ('')::text AS country,
    (currency)::text AS currency,
    (period)::text AS period,
    (month)::date AS month,
    (count(*)::int)::integer AS sample_size,
    (percentile_cont(0.25) WITHIN GROUP (ORDER BY amount)::int)::integer AS p25,
    (percentile_cont(0.50) WITHIN GROUP (ORDER BY amount)::int)::integer AS p50,
    (percentile_cont(0.75) WITHIN GROUP (ORDER BY amount)::int)::integer AS p75
FROM (
    SELECT category, seniority, currency, period, amount,
           date_trunc('month', LEAST(posted_at, created_at) AT TIME ZONE 'UTC')::date AS month
    FROM jobs_effective_salary
    WHERE category <> '' AND duplicate_of IS NULL AND NOT is_private
      AND LEAST(posted_at, created_at) AT TIME ZONE 'UTC' >= date_trunc('month', sqlc.arg('as_of')::timestamptz AT TIME ZONE 'UTC') - interval '11 months'
      AND LEAST(posted_at, created_at) <= sqlc.arg('as_of')::timestamptz
) s
GROUP BY currency, period, month, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int
   AND (grouping(seniority) = 1 OR seniority <> '');

-- name: ComputeInsightsSalaryMonthlyByCountry :many
-- Compute on the read connection; publish the result separately on the writer.
-- The per-country salary history: ComputeInsightsSalaryMonthly's scopes and rules with
-- the country each posting is in (unnest), so a posting in two countries counts in both.
SELECT
    (coalesce(category, ''))::text AS category,
    (coalesce(seniority, ''))::text AS seniority,
    (country)::text AS country,
    (currency)::text AS currency,
    (period)::text AS period,
    (month)::date AS month,
    (count(*)::int)::integer AS sample_size,
    (percentile_cont(0.25) WITHIN GROUP (ORDER BY amount)::int)::integer AS p25,
    (percentile_cont(0.50) WITHIN GROUP (ORDER BY amount)::int)::integer AS p50,
    (percentile_cont(0.75) WITHIN GROUP (ORDER BY amount)::int)::integer AS p75
FROM (
    SELECT category, seniority, country, currency, period, amount,
           date_trunc('month', LEAST(posted_at, created_at) AT TIME ZONE 'UTC')::date AS month
    FROM jobs_effective_salary, unnest(countries) AS country
    WHERE category <> '' AND duplicate_of IS NULL AND NOT is_private
      AND LEAST(posted_at, created_at) AT TIME ZONE 'UTC' >= date_trunc('month', sqlc.arg('as_of')::timestamptz AT TIME ZONE 'UTC') - interval '11 months'
      AND LEAST(posted_at, created_at) <= sqlc.arg('as_of')::timestamptz
) s
GROUP BY country, currency, period, month, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int
   AND (grouping(seniority) = 1 OR seniority <> '');

-- name: ListInsightsSalaryHistory :many
-- One role × grade × country scope's stored months ('' = every value), every
-- (currency, period) it has.
SELECT currency, period, month, sample_size, p25, p50, p75
FROM insights_salary_monthly
WHERE category = sqlc.arg('category')::text
  AND seniority = sqlc.arg('seniority')::text
  AND country = sqlc.arg('country')::text
ORDER BY currency, period, month;

-- name: ListInsightsSalary :many
-- Salary bands for one role × country scope, one row per (currency, period),
-- richest samples first. Currencies are never combined.
SELECT currency, period, sample_size, p25, p50, p75, quantiles
FROM insights_salary_stats
WHERE category = sqlc.arg('category')
  AND seniority = sqlc.arg('seniority')
  AND country = sqlc.arg('country')
ORDER BY sample_size DESC;

-- name: ListInsightsSalaryByCategory :many
-- All-seniority salary bands for one category (country-agnostic '' bucket), so a
-- per-category salary page is one call: one row per (seniority, currency, period),
-- richest samples first. seniority '' is the category-wide band; the named
-- seniorities are the per-grade bands. Bands below the sample floor are already
-- absent from the rollup.
SELECT seniority, currency, period, sample_size, p25, p50, p75, quantiles
FROM insights_salary_stats
WHERE category = sqlc.arg('category')
  AND country = ''
ORDER BY seniority, sample_size DESC;

-- ---------------------------------------------------------------------------
-- Hiring velocity (faceted)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsVelocityDaily :exec
DELETE FROM insights_velocity_daily;

-- name: ComputeInsightsVelocityDaily :many
-- Compute on the read connection; publish the result separately on the writer.
-- Faceted added/removed-per-day rollup from a SINGLE scan of jobs: a CROSS JOIN
-- LATERAL expands each job into its events instead of the previous 8 separate
-- full-table scans (one per facet axis × added/removed), which at catalogue scale
-- dominated the recompute. Per job the lateral emits: added events on the
-- created_at day and (if closed) removed events on the closed_at day, across the
-- 'all', category, seniority, and country (unnest) axes. Days are UTC calendar
-- dates; the outer GROUP BY aggregates per (day, facet_kind, facet_value).
SELECT
    (day)::date AS day,
    (facet_kind)::text AS facet_kind,
    (facet_value)::text AS facet_value,
    (sum(added)::int)::integer AS added,
    (sum(removed)::int)::integer AS removed
FROM jobs j
CROSS JOIN LATERAL (

    SELECT (j.created_at AT TIME ZONE 'UTC')::date AS day, f.fk AS facet_kind, f.fv AS facet_value, 1 AS added, 0 AS removed
    FROM (SELECT 'all'::text AS fk, ''::text AS fv UNION ALL SELECT 'category', j.category UNION ALL SELECT 'seniority', j.seniority) AS f
    WHERE f.fk = 'all' OR f.fv <> ''
    UNION ALL

    SELECT (j.created_at AT TIME ZONE 'UTC')::date, 'country', c, 1, 0
    FROM unnest(j.countries) AS c
    UNION ALL

    SELECT (j.closed_at AT TIME ZONE 'UTC')::date, f.fk, f.fv, 0, 1
    FROM (SELECT 'all'::text AS fk, ''::text AS fv UNION ALL SELECT 'category', j.category UNION ALL SELECT 'seniority', j.seniority) AS f
    WHERE j.closed_at IS NOT NULL AND (f.fk = 'all' OR f.fv <> '')
    UNION ALL

    SELECT (j.closed_at AT TIME ZONE 'UTC')::date, 'country', c, 0, 1
    FROM unnest(j.countries) AS c
    WHERE j.closed_at IS NOT NULL
) e
GROUP BY day, facet_kind, facet_value;

-- name: ListInsightsVelocity :many
-- Dense, gap-free added/removed series over [from_ts, to_ts] at @unit granularity
-- for one facet slice. A daily generate_series fills missing days with zeros; @unit
-- is a caller-validated date_trunc field (day/week/month).
SELECT
    date_trunc(sqlc.arg('unit')::text, d)::date AS period,
    coalesce(sum(v.added), 0)::int   AS added,
    coalesce(sum(v.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN insights_velocity_daily v
    ON v.day = d::date
   AND v.facet_kind = sqlc.arg('facet_kind')
   AND v.facet_value = sqlc.arg('facet_value')
GROUP BY date_trunc(sqlc.arg('unit')::text, d)
ORDER BY date_trunc(sqlc.arg('unit')::text, d);
