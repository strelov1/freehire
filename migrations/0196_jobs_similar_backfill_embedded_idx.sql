-- migrate: no-transaction
--
-- Partial index for SelectJobsNeedingSimilarBackfill once it also requires
-- semantic_embedded_model IS NOT NULL (internal/platform/db/queries/semantic.sql).
--
-- jobs_similar_backfill_idx (0093) is partial only on similar_computed_at IS NULL.
-- That predicate does not shrink: a job is born with the column NULL and leaves the
-- index only after similar-backfill stamps it, which requires chunks. Measured
-- 2026-10-25, the index held ~17.2M rows (null_frac 0.92) and each LIMIT 50 walked
-- ~1.5M of them doing a chunk-pkey probe that missed. The embed stamp
-- (semantic_embedded_model) is set in the same transaction as the chunk replace, so
-- it is the column that means "this row can have chunks". The hash column is not:
-- it copies content_hash and stays NULL when that hash is NULL.
--
-- Keyed the same way as 0093, COALESCE(posted_at, created_at) DESC, id DESC, because
-- the query's ORDER BY must match or Postgres sorts the whole partial set.
--
-- The 0093 index is left in place. The binary this migration runs ahead of still
-- issues the old predicate, which cannot use this narrower index; dropping 0093
-- first would send that query through a sequential scan of jobs. Drop
-- jobs_similar_backfill_idx only after this query is what production runs.
--
-- CONCURRENTLY: jobs is under continuous write traffic. IF NOT EXISTS so a retry
-- after a partial failure is a no-op once the index exists.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_similar_backfill_embedded_idx
    ON public.jobs (COALESCE(posted_at, created_at) DESC, id DESC)
    WHERE similar_computed_at IS NULL
      AND semantic_embedded_model IS NOT NULL;
