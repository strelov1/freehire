-- Precomputed rollups for the public Trends & Insights API (/api/v1/insights/*).
-- Like job_daily_stats, each table is a pure function of the current `jobs` state,
-- fully recomputed by cmd/rollup-stats as an atomic delete-and-reinsert inside one
-- transaction (never an upsert), so a reader never sees a partially rebuilt table.
--
-- "Open as of a date" is derived from created_at/closed_at:
--   open_at(D) := created_at <= D AND (closed_at IS NULL OR closed_at > D)
-- open_count uses D = now (equivalently closed_at IS NULL); open_count_prev uses
-- D = now - growth_window, so role/skill growth is derivable from current state
-- alone — no historical snapshot table is needed.
--
-- The '' (empty string) value of a facet column is the aggregate ("all") bucket,
-- so a single table serves both the global and the facet-scoped reads. Salary
-- percentiles are computed per (currency, period) and never across them; a band
-- below the recompute's minimum sample size is not emitted at all.
--
-- Applied to a fresh volume by initdb after 0021; on an existing prod volume these
-- CREATE statements must be run manually BEFORE deploying code that reads them
-- (per the migrations gotcha).

-- Role demand: category × seniority, per country ('' = all countries).
CREATE TABLE public.insights_role_stats (
    category        text    NOT NULL,
    seniority       text    NOT NULL,
    country         text    NOT NULL DEFAULT '',
    open_count      integer NOT NULL DEFAULT 0,
    open_count_prev integer NOT NULL DEFAULT 0,
    PRIMARY KEY (category, seniority, country)
);

-- Ranked reads select a country slice and order by open_count (or by growth,
-- open_count - open_count_prev) descending; this index serves the volume sort.
CREATE INDEX insights_role_stats_country_open_idx
    ON public.insights_role_stats (country, open_count DESC);

-- Skill demand: canonical skill, optionally scoped by category or country
-- ('' = all). Category and country are not crossed in one row (see the recompute):
-- rows are (skill,'',''), (skill,category,''), or (skill,'',country).
CREATE TABLE public.insights_skill_stats (
    skill           text    NOT NULL,
    category        text    NOT NULL DEFAULT '',
    country         text    NOT NULL DEFAULT '',
    open_count      integer NOT NULL DEFAULT 0,
    open_count_prev integer NOT NULL DEFAULT 0,
    PRIMARY KEY (skill, category, country)
);

CREATE INDEX insights_skill_stats_scope_open_idx
    ON public.insights_skill_stats (category, country, open_count DESC);

-- Salary bands: percentiles per role × geography × (currency, period). Figures are
-- integers in the currency's units (the enrichment salary_min/max scale). Only
-- bands at/above the recompute's minimum sample size are stored.
CREATE TABLE public.insights_salary_stats (
    category    text    NOT NULL DEFAULT '',
    seniority   text    NOT NULL DEFAULT '',
    country     text    NOT NULL DEFAULT '',
    currency    text    NOT NULL,
    period      text    NOT NULL,
    sample_size integer NOT NULL,
    p25         integer NOT NULL,
    p50         integer NOT NULL,
    p75         integer NOT NULL,
    PRIMARY KEY (category, seniority, country, currency, period)
);

-- Faceted hiring velocity: added vs removed per UTC day, per single facet slice.
-- facet_kind is 'all' | 'category' | 'seniority' | 'country'; facet_value is '' for
-- the 'all' slice, else the facet value (a job contributes to each of its
-- countries). The global /stats/jobs-activity keeps reading job_daily_stats; this
-- table backs the facet-scoped /insights/velocity read.
CREATE TABLE public.insights_velocity_daily (
    day         date    NOT NULL,
    facet_kind  text    NOT NULL,
    facet_value text    NOT NULL DEFAULT '',
    added       integer NOT NULL DEFAULT 0,
    removed     integer NOT NULL DEFAULT 0,
    PRIMARY KEY (day, facet_kind, facet_value)
);

CREATE INDEX insights_velocity_daily_facet_day_idx
    ON public.insights_velocity_daily (facet_kind, facet_value, day);
