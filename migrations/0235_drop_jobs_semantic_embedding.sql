-- jobs.semantic_embedding (real[], migration 0021) was the single truncated vector per job
-- from before job_semantic_chunks (0092). Nothing has written it on the open path since
-- then, and nothing reads it; the close path stopped nulling it in #4150, and every binary
-- that still selected it is retired. Dropping it removes a wide column from every SELECT * projection.
ALTER TABLE jobs
    -- squawk-ignore ban-drop-column -- no reader since 0092; every binary selecting it retired after #4150.
    DROP COLUMN IF EXISTS semantic_embedding;
