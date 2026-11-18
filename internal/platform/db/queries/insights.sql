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
