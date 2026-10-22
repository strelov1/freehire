-- Per-company posting lifetime backing the company page's Hiring tab: how many days a
-- company's postings typically stay open before the EMPLOYER closes them. One row per
-- company with the number of qualifying closures and their median duration.
--
-- Like its siblings (insights_company_stats, _growth, _response) it is a pure function
-- of the current `jobs` table, rebuilt by cmd/rollup-company as an atomic
-- delete-and-reinsert inside the same transaction, so a reader never sees it diverge
-- from them. A separate table rather than a column on insights_company_growth: that
-- table drops companies with nothing open, which are exactly the ones whose closure
-- history says the most.

CREATE TABLE public.insights_company_lifetime (
    company_slug text NOT NULL PRIMARY KEY,
    -- squawk-ignore prefer-bigint-over-int -- closures of one company in 180 days; never nears 2^31
    closed       integer NOT NULL,
    median_days  real    NOT NULL
);
