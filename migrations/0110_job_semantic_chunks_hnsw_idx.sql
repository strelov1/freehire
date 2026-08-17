-- migrate: no-transaction
--
-- HNSW index on job_semantic_chunks.embedding (design.md Decision 2 / tasks.md 3.1),
-- what makes NearestJobsToJob's LATERAL rewrite (see that query's comment) actually
-- index-servable: pgvector's HNSW accelerates exactly the `ORDER BY embedding <=>
-- const LIMIT k` shape each source chunk's LATERAL probe now uses.
--
-- CONCURRENTLY needs its own no-transaction file, same reasoning as every other
-- CONCURRENTLY migration in this repo (0078/0081/0097/0107 etc): Postgres forbids it
-- inside a transaction block, and a multi-statement migration file runs as one implicit
-- transaction regardless of the no-transaction marker.
--
-- Applied to a fresh volume by initdb after 0092 (job_semantic_chunks' own migration);
-- on an existing prod volume, build it by hand, detached from the SSH session
-- (systemd-run or nohup) — a CONCURRENTLY build dies with its ssh session and leaves an
-- INVALID index behind. Raise maintenance_work_mem for the session running this: the
-- default (64MB) spills the build graph to disk past roughly half a million tuples
-- (this session's own earlier local spike, see design.md Risks).
CREATE INDEX CONCURRENTLY IF NOT EXISTS job_semantic_chunks_embedding_hnsw_idx
    ON public.job_semantic_chunks USING hnsw (embedding vector_cosine_ops);
