-- Fast approximate count of hiring companies (job_count > 0) for the UNFILTERED
-- /companies list's meta.total, mirroring estimate_open_jobs() for /jobs.
--
-- An exact count(*) FROM companies WHERE job_count > 0 must visit every one of the
-- ~227k matching rows. The companies table is upserted constantly by the ingest and
-- backfill workers, so its visibility map is rarely all-visible and an index-only
-- count falls to the 2.3 GB heap — measured at ~17 s cold on prod, which is what
-- made the /about stats strip (and the first, unfiltered /companies page) crawl.
--
-- The planner's row estimate is O(1) and tracks the job_count > 0 filter. Only the
-- unfiltered catalogue total uses it (see handler.ListCompanies): every facet/search
-- filter rides a narrowing index, so its exact count stays cheap and accurate.
--
-- Applied to a fresh volume by initdb after 0033. CREATE FUNCTION takes no lock on
-- companies, so on the live prod volume this runs as a plain statement out of band.
CREATE FUNCTION public.estimate_hiring_companies() RETURNS bigint
    LANGUAGE plpgsql
    AS $$
DECLARE
    plan json;
BEGIN
    EXECUTE 'EXPLAIN (FORMAT json) SELECT 1 FROM companies WHERE job_count > 0'
        INTO plan;
    RETURN (plan -> 0 -> 'Plan' ->> 'Plan Rows')::bigint;
END;
$$;
