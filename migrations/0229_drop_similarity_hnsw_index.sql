-- migrate: no-transaction
-- The retired similarity backfill was the only nearest-neighbour SQL consumer.
-- Stored chunk vectors remain, read by job_id for Meilisearch indexing and queries.
DROP INDEX CONCURRENTLY IF EXISTS job_semantic_chunks_embedding_hnsw_idx;
