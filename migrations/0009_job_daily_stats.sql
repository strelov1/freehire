-- Materialized daily rollup of catalogue flow (see the job-activity-stats change).
-- One row per calendar day (UTC): `added` = jobs whose created_at falls on that
-- day, `removed` = jobs whose CURRENT closed_at falls on that day. It is a pure
-- function of the current `jobs` state, fully recomputed by cmd/rollup-stats as an
-- atomic delete-and-reinsert (not an upsert); reopen (closed_at cleared) drops a job
-- from its old removed day on the next recompute. The public GET /api/v1/stats/jobs-activity endpoint
-- reads it (aggregated to day/week/month via date_trunc).
--
-- No supporting jobs index ships here: the recompute is a full-table batch
-- aggregate (GROUP BY date(...)), so a seq scan + hash aggregate is the plan an
-- index would not improve. If the recompute ever gets heavy at scale, the levers
-- are a bounded recompute window (day >= now() - interval 'N days') or a partial
-- jobs(closed_at) WHERE closed_at IS NOT NULL index — deferred until measured.
--
-- Applied to a fresh volume by initdb after 0008; on an existing prod volume
-- these statements must be run manually BEFORE deploying code that reads the
-- table (per the migrations gotcha).

CREATE TABLE public.job_daily_stats (
    day         date        PRIMARY KEY,
    added       integer     NOT NULL DEFAULT 0,
    removed     integer     NOT NULL DEFAULT 0,
    computed_at timestamptz NOT NULL DEFAULT now()
);
