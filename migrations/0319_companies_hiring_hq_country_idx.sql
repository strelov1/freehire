-- migrate: no-transaction
-- The /companies hq_country facet on the Postgres path (sort=rating, or a Meili error)
-- matches lower(hq_country) over the hiring set; without this it is a seq scan of ~1M rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS companies_hiring_hq_country_idx
    ON public.companies (lower(hq_country))
    WHERE job_count > 0;
