-- Per-company hiring-signal rollup: the company-grained sibling of the insights_*
-- rollups (0022). Like them it is a pure function of the current `jobs` table, fully
-- recomputed by cmd/rollup-company as an atomic delete-and-reinsert inside one
-- transaction (never an upsert), so a reader never sees a partially rebuilt table.
--
-- One row per (company_slug, day) for each of a company's ACTIVITY days — a UTC day
-- on which it opened (`added`) or closed (`removed`) at least one job. `open` is the
-- company's open-job count as of the end of that day, i.e. the running total
-- cumulative(added) - cumulative(removed) up to and including it. Because a job is
-- always created no later than it closes, open_at(D) = (created<=D) - (closed<=D),
-- so the running difference equals the point-in-time open count. Growth over any
-- window is a carry-forward read of `open` (open now minus open as-of the window
-- start), so no separate "previous window" column is stored.
--
-- Only canonical, attributable rows are counted (company_slug <> '' AND
-- duplicate_of IS NULL), matching companies.job_count / RefreshCompanyFacets, so the
-- per-company numbers reconcile with the rest of the app. Repost copies and
-- company-less rows contribute nothing.
--
-- Applied to a fresh volume by initdb after 0028; on an existing prod volume this
-- CREATE statement must be run manually BEFORE deploying code that reads it (per the
-- migrations gotcha).

CREATE TABLE public.insights_company_stats (
    company_slug text    NOT NULL,
    day          date    NOT NULL,
    added        integer NOT NULL DEFAULT 0,
    removed      integer NOT NULL DEFAULT 0,
    open         integer NOT NULL DEFAULT 0,
    PRIMARY KEY (company_slug, day)
);

-- Cross-company "as of a day" reads (e.g. top movers on a date) scan by day; this
-- index serves them without a full-table scan. Per-company time-series reads are
-- served by the (company_slug, day) primary key.
CREATE INDEX insights_company_stats_day_idx
    ON public.insights_company_stats (day);
