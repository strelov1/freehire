-- Role demand per country without the seniority axis, behind the role × country grid
-- on /insights (GET /api/v1/insights/role-countries). Like every insights_* table it
-- is a pure function of current `jobs`, rebuilt by cmd/rollup-stats as an atomic
-- delete-and-reinsert, and country '' is the all-countries bucket.
--
-- Why not insights_role_stats (0022), keyed (category, seniority, country): it counts
-- only postings with a resolved seniority — about a quarter of the categorized
-- catalogue on 2026-10-30 — and ranks its rows as roles, so all-seniority rows added
-- there would surface as roles in every existing reader. This table also counts a
-- different population: canonical, public postings (duplicate_of IS NULL, NOT
-- is_private), the ones a search for the cell can actually show.
--
-- "Open as of" the window start follows the rule in insights.sql's header
-- (LEAST(posted_at, created_at)), not the created_at rule 0022's comment describes.
--
-- ~50 categories × ~180 countries: the primary key is the only index it needs.

-- integer, like every sibling insights_* count and the int32 the readers bind: one
-- cell is at most the open catalogue (~5M), three orders of magnitude under the limit.
CREATE TABLE IF NOT EXISTS public.insights_role_country_stats (
    category        text    NOT NULL,
    country         text    NOT NULL DEFAULT '',
    -- squawk-ignore prefer-bigint-over-int
    open_count      integer NOT NULL DEFAULT 0,
    -- squawk-ignore prefer-bigint-over-int
    open_count_prev integer NOT NULL DEFAULT 0,
    PRIMARY KEY (category, country)
);
