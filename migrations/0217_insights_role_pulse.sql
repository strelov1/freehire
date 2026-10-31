-- Per-category market pulse behind /insights/roles/<category> (GET
-- /api/v1/insights/role-pulse): competition, time to close and inflow, each for the last
-- 30 days and (for the first two) the 30 before. Like every insights_* table it is a pure function of
-- current `jobs` (and `job_daily_views`), rebuilt by cmd/rollup-stats as an atomic
-- delete-and-reinsert. The rules behind each figure are argued beside
-- RebuildInsightsRolePulse in internal/platform/db/queries/insights.sql.
--
-- One row per category (~50): the primary key is the only index it needs. Counts are
-- bigint — page-view sums are the largest figures in the insights family. Medians are
-- NULL when the window holds no qualifying close: "no figure" is not "zero days".
CREATE TABLE IF NOT EXISTS public.insights_role_pulse (
    category                text             PRIMARY KEY,
    -- The freshest job_daily_views day: both view windows end on it (see the query).
    views_through           date,
    views                   bigint           NOT NULL DEFAULT 0,
    views_prev              bigint           NOT NULL DEFAULT 0,
    exposure_days           double precision NOT NULL DEFAULT 0,
    exposure_days_prev      double precision NOT NULL DEFAULT 0,
    closed                  bigint           NOT NULL DEFAULT 0,
    closed_prev             bigint           NOT NULL DEFAULT 0,
    median_close_days       real,
    median_close_days_prev  real,
    -- The last 30 days only, with no earlier window to compare against: see the
    -- INFLOW note beside RebuildInsightsRolePulse.
    inflow                  bigint           NOT NULL DEFAULT 0
);
