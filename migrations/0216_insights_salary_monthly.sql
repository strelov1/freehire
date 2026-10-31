-- The effective salary every salary rollup reads, and the monthly salary history
-- behind the chart on /insights/salary/<category>.
--
-- jobs_effective_salary resolves a posting's salary ONCE: the first COMPLETE tier —
-- manual (moderator, 0031), then source (an ATS's structured field, 0093), then
-- enrichment (the model's extraction) — with currency and period taken from that same
-- tier, and the amount the min/max midpoint or whichever bound exists. Until this view
-- the bands read enrichment alone; SetJobEnrichment overlays the first two tiers into
-- it, but enrichment runs for technical postings only, so a non-technical posting's
-- structured salary reached no insights figure (397,884 such postings published in
-- October 2026 alone). Postgres inlines a simple view, so a reader pays nothing for the
-- name; what it buys is one definition instead of a CASE copied into every rollup.
-- It carries the posting columns the salary rollups filter and group on, so a reader
-- scans it alone: joining it back to jobs would read the table twice.
--
-- insights_salary_monthly is a pure function of jobs like every insights_* table,
-- rebuilt by cmd/rollup-stats as an atomic delete-and-reinsert: p25/p50/p75 of the
-- effective salary per (category, currency, period, month of publication), canonical
-- public postings, open or closed. Rebuilding every month from the rows each run is
-- what backfills it — postings are never hard-deleted.

CREATE OR REPLACE VIEW public.jobs_effective_salary AS
SELECT
    job_id, category, seniority, countries, posted_at, created_at, closed_at,
    duplicate_of, is_private,
    CASE t.tier WHEN 'manual' THEN m_cur WHEN 'source' THEN s_cur ELSE e_cur END AS currency,
    CASE t.tier WHEN 'manual' THEN m_per WHEN 'source' THEN s_per ELSE e_per END AS period,
    CASE t.tier WHEN 'manual' THEN m_amt WHEN 'source' THEN s_amt ELSE e_amt END AS amount
FROM (
    SELECT
        id AS job_id, category, seniority, countries, posted_at, created_at, closed_at,
        duplicate_of, is_private,
        upper(btrim(salary_currency_manual)) AS m_cur,
        salary_period_manual AS m_per,
        coalesce((salary_min_manual + salary_max_manual) / 2.0, salary_min_manual, salary_max_manual) AS m_amt,
        upper(btrim(salary_currency_source)) AS s_cur,
        salary_period_source AS s_per,
        coalesce((salary_min_source + salary_max_source) / 2.0, salary_min_source, salary_max_source) AS s_amt,
        upper(btrim(enrichment->>'salary_currency')) AS e_cur,
        enrichment->>'salary_period' AS e_per,
        coalesce(
            (nullif(enrichment->>'salary_min', '')::numeric + nullif(enrichment->>'salary_max', '')::numeric) / 2,
            nullif(enrichment->>'salary_min', '')::numeric,
            nullif(enrichment->>'salary_max', '')::numeric
        ) AS e_amt
    FROM public.jobs
) j,
-- A tier counts only when it is COMPLETE — a positive amount, a currency and a period.
-- One with amounts but no currency falls through to the next, rather than taking the
-- posting out of every figure: a NULL comparison is not true, so it simply loses.
LATERAL (
    SELECT CASE
        WHEN m_amt > 0 AND m_cur <> '' AND m_per <> '' THEN 'manual'
        WHEN s_amt > 0 AND s_cur <> '' AND s_per <> '' THEN 'source'
        WHEN e_amt > 0 AND e_cur <> '' AND e_per <> '' THEN 'enrichment'
    END AS tier
) t
WHERE t.tier IS NOT NULL;

-- integer counts and percentiles, like insights_salary_stats (0022) and the int32 the
-- readers bind: a salary or a monthly sample is far under the 32-bit limit.
CREATE TABLE IF NOT EXISTS public.insights_salary_monthly (
    category    text    NOT NULL,
    currency    text    NOT NULL,
    period      text    NOT NULL,
    month       date    NOT NULL,
    -- squawk-ignore prefer-bigint-over-int
    sample_size integer NOT NULL,
    -- squawk-ignore prefer-bigint-over-int
    p25         integer NOT NULL,
    -- squawk-ignore prefer-bigint-over-int
    p50         integer NOT NULL,
    -- squawk-ignore prefer-bigint-over-int
    p75         integer NOT NULL,
    PRIMARY KEY (category, currency, period, month)
);
