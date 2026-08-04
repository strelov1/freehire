-- Widen estimate_open_jobs() (0001_init.sql) to also exclude private jobs (0074) from the
-- DB-backed /jobs list's meta.total: it already answers "how many open jobs", and a
-- jd-tailor-intake private job (visible only to its creator) is not one of them, exactly
-- like ListJobs's own WHERE clause. Still O(1) — the function only changes which query the
-- planner is asked to EXPLAIN, not how the estimate is obtained.
CREATE OR REPLACE FUNCTION public.estimate_open_jobs() RETURNS bigint
    LANGUAGE plpgsql
    AS $$
DECLARE
    plan json;
BEGIN
    EXECUTE 'EXPLAIN (FORMAT json) SELECT 1 FROM jobs WHERE closed_at IS NULL AND NOT is_private'
        INTO plan;
    RETURN (plan -> 0 -> 'Plan' ->> 'Plan Rows')::bigint;
END;
$$;
