-- Speed up the DB-backed GET /api/v1/jobs list at catalogue scale (it was 17-30s
-- over ~2.5M open jobs). Two costs are removed: a full-table sort and a full
-- count(*), both linear in the open-job set on every request.
--
-- (1) Partial index matching the list's filter + order exactly
--     (WHERE closed_at IS NULL, ORDER BY created_at DESC, id DESC), so ListJobs
--     becomes an index scan of only LIMIT+OFFSET rows with no sort.
--
--     This file uses a plain CREATE INDEX, which is instant on a fresh/empty
--     initdb volume and is the sqlc schema source. On the LIVE prod DB apply it
--     manually and non-locking instead (CONCURRENTLY cannot run inside a
--     transaction, so it is not put here):
--
--       CREATE INDEX CONCURRENTLY jobs_open_created_idx
--         ON jobs (created_at DESC, id DESC) WHERE closed_at IS NULL;
--
--     If a concurrent build is interrupted it leaves an INVALID index; drop it
--     (DROP INDEX jobs_open_created_idx) and retry.
CREATE INDEX IF NOT EXISTS jobs_open_created_idx
    ON jobs (created_at DESC, id DESC)
    WHERE closed_at IS NULL;

-- (2) O(1) approximate open-job total for the list's meta.total, replacing the
--     exact count(*) over ~millions of rows. Returns the Postgres planner's
--     estimated row count for the open-jobs filter -- unlike pg_class.reltuples
--     (whole table, includes closed jobs), the planner estimate tracks
--     `closed_at IS NULL` via column statistics. Accuracy follows ANALYZE, which
--     is sufficient for an approximate list total. The EXECUTEd query is a
--     constant, so there is no dynamic-SQL injection surface.
--     The function is intentionally VOLATILE (the default): its result changes as
--     table statistics change, so do NOT mark it STABLE/IMMUTABLE (that would let
--     the planner cache a stale estimate within a statement).
CREATE OR REPLACE FUNCTION estimate_open_jobs() RETURNS bigint AS $$
DECLARE
    plan json;
BEGIN
    EXECUTE 'EXPLAIN (FORMAT json) SELECT 1 FROM jobs WHERE closed_at IS NULL'
        INTO plan;
    RETURN (plan -> 0 -> 'Plan' ->> 'Plan Rows')::bigint;
END;
$$ LANGUAGE plpgsql;
