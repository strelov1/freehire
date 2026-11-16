-- Salary Trends & Insights rollups (insights_salary_*) and their reads, split out of
-- insights.sql for size. Its header states the rules every rollup here keeps too.

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
