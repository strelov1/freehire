-- Per-company open/growth scalar backing the /api/v1/insights/companies leaderboard.
-- The company-grained sibling of insights_role_stats (0022): one row per company with
-- its current open-job count and the open count as of the growth window ago, so a
-- ranked "who is ramping / freezing" read is a trivial ORDER BY … LIMIT rather than a
-- per-request aggregate over the whole catalogue.
--
-- Like the other rollups it is a pure function of the current `jobs` table, rebuilt by
-- cmd/rollup-company as an atomic delete-and-reinsert (never an upsert) inside the SAME
-- transaction as insights_company_stats, so the two never diverge and a reader never
-- sees a partially rebuilt table. open_count = open canonical jobs now; open_count_prev
-- = open canonical jobs as of now − growth_window (growth = open_count − open_count_prev).
-- Only canonical, attributable rows are counted (company_slug <> '' AND
-- duplicate_of IS NULL), matching companies.job_count and insights_company_stats.
--
-- Applied to a fresh volume by initdb after 0029; on an existing prod volume this
-- CREATE statement must be run manually BEFORE deploying code that reads it (per the
-- migrations gotcha).

CREATE TABLE public.insights_company_growth (
    company_slug    text    NOT NULL PRIMARY KEY,
    open_count      integer NOT NULL DEFAULT 0,
    open_count_prev integer NOT NULL DEFAULT 0
);

-- The leaderboard's `open` sort and its `min_open` floor both key off open_count;
-- this index serves that ranked read. The `growth` sort orders by
-- (open_count - open_count_prev) and sorts the (one-row-per-company) table in memory.
CREATE INDEX insights_company_growth_open_idx
    ON public.insights_company_growth (open_count DESC);
