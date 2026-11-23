-- migrate: no-transaction
-- A prefix of jobs_source_id_idx (source, id) and of jobs_source_extid_pattern_idx
-- (source, external_id), either of which answers a filter on source alone. 1.25 GB.
DROP INDEX CONCURRENTLY IF EXISTS jobs_source_idx;
