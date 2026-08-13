-- pgvector-backed columns supplementing the existing real[] semantic_embedding:
-- semantic_embedding_vec is the same 768-dim e5 vector in a form pgvector can run
-- nearest-neighbour SQL over (ORDER BY ... <=>), which real[] cannot. A new
-- column rather than an in-place ALTER COLUMN TYPE on semantic_embedding — the
-- latter forces a full-table rewrite under ACCESS EXCLUSIVE on a ~2M-row,
-- write-heavy table; adding a nullable column with no default is instant on
-- Postgres 11+. semantic_embedding stays as-is for now and is backfilled into
-- the new column by a later one-off script; it is dropped in a separate change
-- once nothing reads it. similar_job_ids + similar_computed_at back the
-- precomputed "similar jobs" lookup (cmd/similar-backfill), mirroring the
-- semantic_embedded_model/semantic_embedded_hash stamp idiom already used for
-- semantic_embedding itself. See openspec/changes/drop-hybrid-search-pgvector-similar.
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE public.jobs
    ADD COLUMN semantic_embedding_vec vector(768),
    ADD COLUMN similar_job_ids bigint[],
    ADD COLUMN similar_computed_at timestamptz;
