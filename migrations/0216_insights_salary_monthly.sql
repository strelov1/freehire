-- The effective salary every salary rollup reads, and the monthly salary history
-- behind the chart on /insights/salary/<category>.
--
-- jobs_effective_salary resolves a posting's salary ONCE: the first tier that states a
-- bound — manual (moderator, 0031), then source (an ATS's structured field, 0093), then
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
    duplicate_of, is_private, currency, period, amount
FROM (
    SELECT
        id AS job_id, category, seniority, countries, posted_at, created_at, closed_at,
        duplicate_of, is_private,
        CASE
            WHEN salary_min_manual IS NOT NULL OR salary_max_manual IS NOT NULL THEN 'manual'
            WHEN salary_min_source IS NOT NULL OR salary_max_source IS NOT NULL THEN 'source'
            ELSE 'enrichment'
        END AS tier,
        salary_min_manual, salary_max_manual, salary_currency_manual, salary_period_manual,
        salary_min_source, salary_max_source, salary_currency_source, salary_period_source,
        nullif(enrichment->>'salary_min', '')::numeric AS e_min,
        nullif(enrichment->>'salary_max', '')::numeric AS e_max,
        enrichment->>'salary_currency' AS e_currency,
        enrichment->>'salary_period' AS e_period
    FROM public.jobs
) j,
LATERAL (
    SELECT
        upper(CASE tier WHEN 'manual' THEN salary_currency_manual WHEN 'source' THEN salary_currency_source ELSE e_currency END) AS currency,
        CASE tier WHEN 'manual' THEN salary_period_manual WHEN 'source' THEN salary_period_source ELSE e_period END AS period,
        CASE tier
            WHEN 'manual' THEN coalesce((salary_min_manual + salary_max_manual) / 2.0, salary_min_manual, salary_max_manual)
            WHEN 'source' THEN coalesce((salary_min_source + salary_max_source) / 2.0, salary_min_source, salary_max_source)
            ELSE coalesce((e_min + e_max) / 2, e_min, e_max)
        END AS amount
) s
WHERE s.amount IS NOT NULL AND coalesce(s.currency, '') <> '' AND coalesce(s.period, '') <> '';

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
