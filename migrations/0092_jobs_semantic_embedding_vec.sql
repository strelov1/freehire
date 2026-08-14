-- pgvector-backed storage for job embeddings, replacing the real[] semantic_embedding
-- column's role as the queryable vector representation. A job's embedding is not one
-- vector but several: the source text is HTML-stripped and split into chunks so the
-- FULL description is represented (today's single real[] vector only ever captures the
-- first ~15-20% of an average description, truncated twice — once by the facet-index
-- rune cap, again silently by TEI's own token limit). pgvector's vector type is
-- fixed-arity per row, so "many vectors for one job" needs many rows in a dedicated
-- table, not a wider column on jobs (unlike Meili's native multi-vector document
-- support, which has no pgvector equivalent) — hence job_semantic_chunks below rather
-- than a single semantic_embedding_vec vector(768) column on jobs, which this migration
-- originally added and this revision removes before it was ever populated with real
-- data. jobs.semantic_embedding (real[]) stays as-is for now and is dropped in a
-- separate later change once nothing reads it. similar_job_ids + similar_computed_at
-- back the precomputed "similar jobs" lookup (cmd/similar-backfill), mirroring the
-- semantic_embedded_model/semantic_embedded_hash stamp idiom already used for
-- semantic_embedding itself. See openspec/changes/drop-hybrid-search-pgvector-similar.
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE public.jobs
    ADD COLUMN similar_job_ids bigint[],
    ADD COLUMN similar_computed_at timestamptz;

CREATE TABLE job_semantic_chunks (
    job_id      bigint NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    chunk_index smallint NOT NULL,
    embedding   vector(768) NOT NULL,
    PRIMARY KEY (job_id, chunk_index)
);
