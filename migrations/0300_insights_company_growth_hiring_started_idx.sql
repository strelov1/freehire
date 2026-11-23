-- migrate: no-transaction
-- The /jobs/search "just started hiring" filter (new_hiring=true) reads the slugs of the
-- few thousand companies rollup-company marked, out of ~1M rows, on every such search.
CREATE INDEX CONCURRENTLY IF NOT EXISTS insights_company_growth_hiring_started_idx
    ON public.insights_company_growth (company_slug)
    WHERE hiring_started_at IS NOT NULL;
