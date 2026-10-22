-- Per-company posting views backing the company page's Hiring tab: how much attention
-- a company's postings drew over the last 30 days and the 30 before. A nightly scalar
-- pair rather than a per-request sum, because job_daily_views holds ~8M rows per 30
-- days (measured 2026-10-22) and the read would join them to jobs on every page view.
--
-- Rebuilt by cmd/rollup-company in the same transaction as its siblings. Reads
-- page_uniques, never uniques — see the view-count rule in AGENTS.md.

CREATE TABLE public.insights_company_views (
    company_slug text   NOT NULL PRIMARY KEY,
    views        bigint NOT NULL,
    views_prev   bigint NOT NULL
);
