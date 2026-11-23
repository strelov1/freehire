-- name: DeleteAllInsightsRoleDemand :exec
DELETE FROM insights_role_demand_weekly;

-- name: ComputeInsightsRoleDemand :many
-- Count public canonical vacancies by publication date, including closed ones.
-- A missing publication date falls back to first appearance in the catalogue.
-- The same Monday UTC bound drives the aggregation and the dense output grid.
WITH bounds AS (
    SELECT sqlc.arg('week_start')::date AS cutoff
), counts AS (
    SELECT
        j.category,
        date_trunc('week', COALESCE(j.posted_at, j.created_at) AT TIME ZONE 'UTC')::date AS week,
        count(*) AS new_jobs
    FROM jobs j
    CROSS JOIN bounds b
    WHERE j.category = ANY(sqlc.arg('categories')::text[])
      AND j.duplicate_of IS NULL
      AND NOT j.is_private
      AND COALESCE(j.posted_at, j.created_at) >= (b.cutoff - 84)::timestamp AT TIME ZONE 'UTC'
      AND COALESCE(j.posted_at, j.created_at) < b.cutoff::timestamp AT TIME ZONE 'UTC'
    GROUP BY j.category, 2
)
SELECT c.category::text AS category, weeks.start::date AS week, COALESCE(counts.new_jobs, 0)::bigint AS new_jobs
FROM bounds b
CROSS JOIN unnest(sqlc.arg('categories')::text[]) AS c(category)
CROSS JOIN LATERAL generate_series((b.cutoff - 84)::timestamp, (b.cutoff - 7)::timestamp, interval '1 week') AS weeks(start)
LEFT JOIN counts ON counts.category = c.category AND counts.week = weeks.start::date;

-- name: CopyInsightsRoleDemand :copyfrom
INSERT INTO insights_role_demand_weekly (category, week, new_jobs) VALUES ($1, $2, $3);

-- name: ListInsightsRoleDemand :many
SELECT category, week, new_jobs
FROM insights_role_demand_weekly
ORDER BY category, week;
