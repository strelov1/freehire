-- Drop the remote_unspecified column (added in 0027). The "remote, region not
-- specified" bucket it materialized is now a query-time predicate: the search
-- filter selects jobs with no resolved geography via `regions IS EMPTY` (the
-- reserved `regions=none` facet value), which composes with the existing
-- work_mode=remote filter instead of duplicating it in a stored boolean. The
-- column is therefore dead — nothing derives, persists, serves, or filters on it.
--
-- This is also the schema source sqlc reads: applied after 0027, it leaves the
-- column absent from the generated models. On a persistent DB (initdb runs
-- migrations only on first volume init) this must be applied by hand:
--   ALTER TABLE jobs DROP COLUMN IF EXISTS remote_unspecified;
ALTER TABLE jobs
    DROP COLUMN IF EXISTS remote_unspecified;
