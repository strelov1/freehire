-- migrate: no-transaction
-- jobs_normalized_url_open_idx (0051, WHERE closed_at IS NULL) covers every lookup
-- this narrower one (closed_at IS NULL AND duplicate_of IS NULL) served: the planner
-- proves the open predicate from the stricter one. Measured on prod 2026-11-22: 360
-- scans in 18 days against 26.5k on the open index, 1.1 GB that autovacuum re-read on
-- every pass over jobs.
DROP INDEX CONCURRENTLY IF EXISTS jobs_normalized_url_idx;
