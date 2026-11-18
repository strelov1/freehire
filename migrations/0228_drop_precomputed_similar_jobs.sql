-- Drops the offline similar-jobs list and its scheduling stamp: related jobs now come from
-- live Meilisearch search, and every binary that still selected these columns is retired.
-- Chunk embeddings stay for hybrid search. Dependent partial indexes drop with
-- similar_computed_at; this does not rewrite the jobs table.
ALTER TABLE jobs
    -- squawk-ignore ban-drop-column -- derived list is replaced by live search; retire old binaries first.
    DROP COLUMN IF EXISTS similar_job_ids,
    -- squawk-ignore ban-drop-column -- the retired backfill's scheduling stamp has no remaining reader.
    DROP COLUMN IF EXISTS similar_computed_at;
