-- estimate_open_jobs() backs meta.total on the DB-backed /jobs list. It estimated
-- `closed_at IS NULL` alone, but the list it labels also applies
-- `duplicate_of IS NULL AND NOT is_private` — so every suppressed repost and every
-- private posting was counted in the total and absent from the results. On production
-- that gap, compounded by an inflated reltuples, published 5,226,661 against 3,300,658
-- actual rows.
--
-- The total stays an estimate: the planner answers from statistics, so this is still
-- O(1) and still not an exact count. It now estimates the right set.
CREATE OR REPLACE FUNCTION public.estimate_open_jobs() RETURNS bigint
    LANGUAGE plpgsql
    AS $$
DECLARE
    plan json;
BEGIN
    EXECUTE 'EXPLAIN (FORMAT json) SELECT 1 FROM jobs WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private'
        INTO plan;
    RETURN (plan -> 0 -> 'Plan' ->> 'Plan Rows')::bigint;
END;
$$;
