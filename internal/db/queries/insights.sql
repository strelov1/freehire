-- Trends & Insights rollups (insights_*), recomputed by cmd/rollup-stats as an
-- atomic delete-and-reinsert, and the read queries the public /api/v1/insights/*
-- endpoints serve from them. All rollups are a pure function of current `jobs`
-- state; @prev_ts (the growth-window start) and @min_sample are supplied by the
-- worker so the window and sample floor stay out of the SQL and are test-injectable.

-- ---------------------------------------------------------------------------
-- Role demand
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsRoleStats :exec
DELETE FROM insights_role_stats;

-- name: RebuildInsightsRoleStatsGlobal :execrows
-- Country-agnostic ('' bucket) role demand. open_count = jobs open now
-- (closed_at IS NULL); open_count_prev = jobs open as of @prev_ts. The inner
-- aggregate is wrapped so the "non-zero in either window" filter names the counts
-- once instead of repeating the FILTER expressions in a HAVING.
INSERT INTO insights_role_stats (category, seniority, country, open_count, open_count_prev)
SELECT category, seniority, '', open_count, open_count_prev
FROM (
    SELECT
        category,
        seniority,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs
    WHERE category <> '' AND seniority <> ''
    GROUP BY category, seniority
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: RebuildInsightsRoleStatsByCountry :execrows
-- Per-country role demand: a job contributes once to each of its countries.
INSERT INTO insights_role_stats (category, seniority, country, open_count, open_count_prev)
SELECT category, seniority, country, open_count, open_count_prev
FROM (
    SELECT
        category,
        seniority,
        country,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(countries) AS country
    WHERE category <> '' AND seniority <> ''
    GROUP BY category, seniority, country
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: ListInsightsRoles :many
-- Ranked roles within one country slice ('' = all countries), ordered by raw
-- demand or by growth (open_count - open_count_prev), demand as the tiebreak.
-- An empty @category means all categories (the original behavior); a non-empty
-- @category restricts the ranking to that category's seniorities.
SELECT category, seniority, open_count, (open_count - open_count_prev)::int AS growth
FROM insights_role_stats
WHERE country = sqlc.arg('country')
  AND (sqlc.arg('category')::text = '' OR category = sqlc.arg('category'))
ORDER BY
    (CASE WHEN sqlc.arg('sort')::text = 'growth' THEN (open_count - open_count_prev) ELSE open_count END) DESC,
    open_count DESC
LIMIT sqlc.arg('lim')::int;

-- ---------------------------------------------------------------------------
-- Skill demand
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsSkillStats :exec
DELETE FROM insights_skill_stats;

-- name: RebuildInsightsSkillStatsGlobal :execrows
-- Country- and category-agnostic skill demand (both '' buckets).
INSERT INTO insights_skill_stats (skill, category, country, open_count, open_count_prev)
SELECT skill, '', '', open_count, open_count_prev
FROM (
    SELECT
        skill,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(skills) AS skill
    GROUP BY skill
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: RebuildInsightsSkillStatsByCategory :execrows
-- Per-category skill demand (country '' bucket).
INSERT INTO insights_skill_stats (skill, category, country, open_count, open_count_prev)
SELECT skill, category, '', open_count, open_count_prev
FROM (
    SELECT
        skill,
        category,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
    FROM jobs, unnest(skills) AS skill
    WHERE category <> ''
    GROUP BY skill, category
) t
WHERE open_count > 0 OR open_count_prev > 0;

-- name: RebuildInsightsSkillStatsByCountry :execrows
-- Per-country skill demand (category '' bucket).
INSERT INTO insights_skill_stats (skill, category, country, open_count, open_count_prev)
SELECT skill, '', country, open_count, open_count_prev
FROM (
    SELECT
        skill,
        country,
        count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
        count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
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
-- Salary bands
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsSalaryStats :exec
DELETE FROM insights_salary_stats;

-- name: RebuildInsightsSalaryStatsGlobal :execrows
-- Country-agnostic salary bands. CUBE(category, seniority) emits every scope the
-- endpoint can ask for — the exact (category, seniority) band, the category-only
-- band (seniority ''), the seniority-only band (category ''), and the overall band
-- (both '') — all per (currency, period), which is never aggregated across. The
-- representative figure per job is the min/max midpoint (or whichever bound is
-- present). Only bands at/above @min_sample are stored.
INSERT INTO insights_salary_stats (category, seniority, country, currency, period, sample_size, p25, p50, p75)
SELECT
    coalesce(category, ''),
    coalesce(seniority, ''),
    '',
    currency,
    period,
    count(*)::int,
    percentile_cont(0.25) WITHIN GROUP (ORDER BY sal)::int,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY sal)::int,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY sal)::int
FROM (
    SELECT
        category,
        seniority,
        upper(enrichment->>'salary_currency') AS currency,
        enrichment->>'salary_period'   AS period,
        coalesce(
            (nullif(enrichment->>'salary_min', '')::numeric + nullif(enrichment->>'salary_max', '')::numeric) / 2,
            nullif(enrichment->>'salary_min', '')::numeric,
            nullif(enrichment->>'salary_max', '')::numeric
        ) AS sal
    FROM jobs
    WHERE closed_at IS NULL
      AND category <> '' AND seniority <> ''
      AND coalesce(enrichment->>'salary_currency', '') <> ''
      AND coalesce(enrichment->>'salary_period', '') <> ''
) s
WHERE sal IS NOT NULL
GROUP BY currency, period, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int;

-- name: RebuildInsightsSalaryStatsByCountry :execrows
-- Per-country salary bands (same CUBE of role scope, country from unnest).
INSERT INTO insights_salary_stats (category, seniority, country, currency, period, sample_size, p25, p50, p75)
SELECT
    coalesce(category, ''),
    coalesce(seniority, ''),
    country,
    currency,
    period,
    count(*)::int,
    percentile_cont(0.25) WITHIN GROUP (ORDER BY sal)::int,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY sal)::int,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY sal)::int
FROM (
    SELECT
        category,
        seniority,
        country,
        upper(enrichment->>'salary_currency') AS currency,
        enrichment->>'salary_period'   AS period,
        coalesce(
            (nullif(enrichment->>'salary_min', '')::numeric + nullif(enrichment->>'salary_max', '')::numeric) / 2,
            nullif(enrichment->>'salary_min', '')::numeric,
            nullif(enrichment->>'salary_max', '')::numeric
        ) AS sal
    FROM jobs, unnest(countries) AS country
    WHERE closed_at IS NULL
      AND category <> '' AND seniority <> ''
      AND coalesce(enrichment->>'salary_currency', '') <> ''
      AND coalesce(enrichment->>'salary_period', '') <> ''
) s
WHERE sal IS NOT NULL
GROUP BY country, currency, period, CUBE(category, seniority)
HAVING count(*) >= sqlc.arg('min_sample')::int;

-- name: ListInsightsSalary :many
-- Salary bands for one role × country scope, one row per (currency, period),
-- richest samples first. Currencies are never combined.
SELECT currency, period, sample_size, p25, p50, p75
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
SELECT seniority, currency, period, sample_size, p25, p50, p75
FROM insights_salary_stats
WHERE category = sqlc.arg('category')
  AND country = ''
ORDER BY seniority, sample_size DESC;

-- ---------------------------------------------------------------------------
-- Hiring velocity (faceted)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsVelocityDaily :exec
DELETE FROM insights_velocity_daily;

-- name: RebuildInsightsVelocityDaily :execrows
-- Faceted added/removed-per-day rollup from a SINGLE scan of jobs: a CROSS JOIN
-- LATERAL expands each job into its events instead of the previous 8 separate
-- full-table scans (one per facet axis × added/removed), which at catalogue scale
-- dominated the recompute. Per job the lateral emits: added events on the
-- created_at day and (if closed) removed events on the closed_at day, across the
-- 'all', category, seniority, and country (unnest) axes. Days are UTC calendar
-- dates; the outer GROUP BY aggregates per (day, facet_kind, facet_value).
INSERT INTO insights_velocity_daily (day, facet_kind, facet_value, added, removed)
SELECT day, facet_kind, facet_value, sum(added)::int, sum(removed)::int
FROM jobs j
CROSS JOIN LATERAL (
    -- Added events (created_at day), scalar facet axes.
    SELECT (j.created_at AT TIME ZONE 'UTC')::date AS day, f.fk AS facet_kind, f.fv AS facet_value, 1 AS added, 0 AS removed
    FROM (VALUES ('all', ''), ('category', j.category), ('seniority', j.seniority)) AS f(fk, fv)
    WHERE f.fk = 'all' OR f.fv <> ''
    UNION ALL
    -- Added events, per country.
    SELECT (j.created_at AT TIME ZONE 'UTC')::date, 'country', c, 1, 0
    FROM unnest(j.countries) AS c
    UNION ALL
    -- Removed events (closed_at day), scalar facet axes — only for closed jobs.
    SELECT (j.closed_at AT TIME ZONE 'UTC')::date, f.fk, f.fv, 0, 1
    FROM (VALUES ('all', ''), ('category', j.category), ('seniority', j.seniority)) AS f(fk, fv)
    WHERE j.closed_at IS NOT NULL AND (f.fk = 'all' OR f.fv <> '')
    UNION ALL
    -- Removed events, per country.
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

-- ---------------------------------------------------------------------------
-- Per-company hiring signal
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsCompanyStats :exec
DELETE FROM insights_company_stats;

-- name: RebuildInsightsCompanyStats :execrows
-- Per-(company, day) hiring velocity with a running open count, from the retained
-- jobs lifecycle. Each canonical, attributable job (company_slug <> '' AND
-- duplicate_of IS NULL) emits an added event on its created_at (UTC) day and, if
-- closed, a removed event on its closed_at (UTC) day; the inner aggregate collapses
-- those to one row per (company_slug, day), and the window SUM over (added - removed)
-- ordered by day yields open = cumulative(added) - cumulative(removed) as of that day
-- (a job is created no later than it closes, so this equals the point-in-time open
-- count). Only a company's activity days get a row.
INSERT INTO insights_company_stats (company_slug, day, added, removed, open)
SELECT
    company_slug,
    day,
    added,
    removed,
    sum(added - removed) OVER (PARTITION BY company_slug ORDER BY day)::int AS open
FROM (
    SELECT j.company_slug, day, sum(added)::int AS added, sum(removed)::int AS removed
    FROM jobs j
    CROSS JOIN LATERAL (
        -- Added event on the created_at day; removed event on the closed_at day.
        SELECT (j.created_at AT TIME ZONE 'UTC')::date AS day, 1 AS added, 0 AS removed
        UNION ALL
        SELECT (j.closed_at AT TIME ZONE 'UTC')::date, 0, 1
        WHERE j.closed_at IS NOT NULL
    ) e
    WHERE j.company_slug <> '' AND j.duplicate_of IS NULL
    GROUP BY j.company_slug, day
) daily;

-- ---------------------------------------------------------------------------
-- Per-company open/growth scalar (backs the /insights/companies leaderboard)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsCompanyGrowth :exec
DELETE FROM insights_company_growth;

-- name: RebuildInsightsCompanyGrowth :execrows
-- One row per company with its current open-count and the open-count as of @prev_ts,
-- from a single scan of jobs over canonical rows only (same count(*) FILTER idiom as
-- insights_role_stats). open_count uses closed_at IS NULL (open now); open_count_prev
-- uses open-as-of @prev_ts. Companies open at neither point are dropped (HAVING) to
-- keep the table lean.
INSERT INTO insights_company_growth (company_slug, open_count, open_count_prev)
SELECT
    company_slug,
    count(*) FILTER (WHERE closed_at IS NULL)::int AS open_count,
    count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int AS open_count_prev
FROM jobs
WHERE company_slug <> '' AND duplicate_of IS NULL
GROUP BY company_slug
HAVING count(*) FILTER (WHERE closed_at IS NULL) > 0
    OR count(*) FILTER (WHERE created_at <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts'))) > 0;

-- name: ListInsightsCompanies :many
-- The leaderboard read: companies ranked by growth (open_count - open_count_prev),
-- '-growth' (freezing) reverses it, 'open' ranks by raw size; open_count is the
-- tiebreak. @min_open floors the current open-count (blunts ingest-artifact spikes).
-- company_name falls back to the slug when no companies row exists.
SELECT
    g.company_slug,
    coalesce(c.name, g.company_slug) AS company_name,
    g.open_count,
    g.open_count_prev,
    (g.open_count - g.open_count_prev)::int AS growth
FROM insights_company_growth g
LEFT JOIN companies c ON c.slug = g.company_slug
WHERE g.open_count >= sqlc.arg('min_open')::int
ORDER BY
    (CASE
        WHEN sqlc.arg('sort')::text = 'open'     THEN g.open_count
        WHEN sqlc.arg('sort')::text = '-growth'  THEN -(g.open_count - g.open_count_prev)
        ELSE (g.open_count - g.open_count_prev)
    END) DESC,
    g.open_count DESC
LIMIT sqlc.arg('lim')::int;
