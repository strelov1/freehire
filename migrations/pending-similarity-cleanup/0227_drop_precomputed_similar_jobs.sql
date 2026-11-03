-- Apply only AFTER the API and embed workers run the Meilisearch similarity code
-- and the retired similar-backfill service/timer has been stopped and removed.
-- Old binaries still select the deleted columns. New sqlc queries name their
-- columns explicitly and run both before and after this contraction.
-- Chunk embeddings stay for hybrid search. Dependent partial indexes drop with
-- similar_computed_at; this does not rewrite the jobs table.
ALTER TABLE jobs
    -- squawk-ignore ban-drop-column -- derived list is replaced by live search; retire old binaries first.
    DROP COLUMN IF EXISTS similar_job_ids,
    -- squawk-ignore ban-drop-column -- the retired backfill's scheduling stamp has no remaining reader.
    DROP COLUMN IF EXISTS similar_computed_at;
