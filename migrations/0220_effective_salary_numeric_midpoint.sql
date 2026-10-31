-- Sum a salary range's bounds as numeric, not integer, in jobs_effective_salary (0216).
--
-- The view took a range's midpoint as (min + max) / 2.0 over two integer columns, and
-- Postgres adds two integers as an integer: a source range of 1.5-1.6 billion (a yearly
-- salary in VND or IRR) overflowed 32 bits before it was ever halved. Every salary
-- rollup reads the view inside cmd/rollup-stats' one insights transaction, so from the
-- first run on 2026-10-31 07:02 UTC the whole rebuild failed with "integer out of range"
-- and every insights_* table stayed on its last good snapshot. Casting one bound to
-- numeric before the sum keeps the midpoint exact; the result type (numeric) is the
-- same as before, which is what lets CREATE OR REPLACE swap the definition in place.

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
        coalesce((salary_min_manual::numeric + salary_max_manual) / 2, salary_min_manual, salary_max_manual) AS m_amt,
        upper(btrim(salary_currency_source)) AS s_cur,
        salary_period_source AS s_per,
        coalesce((salary_min_source::numeric + salary_max_source) / 2, salary_min_source, salary_max_source) AS s_amt,
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
